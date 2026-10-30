package engine

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

func waitProjectWaiters(t *testing.T, p *projectIndex, root string, n int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		p.mu.Lock()
		f := p.active[root]
		ready := f != nil && f.waiters == n
		p.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline:
			t.Fatal("waiters did not arrive")
		default:
			runtime.Gosched()
		}
	}
}

func TestProjectFlightCoalescingAndCancellation(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	started, release := make(chan struct{}, 2), make(chan struct{})
	p.load = func(ctx context.Context, root string) (files.Listing, error) {
		started <- struct{}{}
		select {
		case <-release:
			return files.Listing{Root: root, Files: []string{"file"}, Status: map[string]files.Status{"file": files.Modified}}, nil
		case <-ctx.Done():
			return files.Listing{}, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { _, err := p.listing(ctx, "root"); first <- err }()
	<-started
	second := make(chan files.Listing, 1)
	go func() {
		out, err := p.listing(context.Background(), "root")
		if err != nil {
			t.Error(err)
		}
		second <- out
	}()
	waitProjectWaiters(t, p, "root", 2)
	cancel()
	if err := <-first; err != context.Canceled {
		t.Fatal(err)
	}
	close(release)
	out := <-second
	if len(out.Files) != 1 {
		t.Fatal("leader canceled shared work")
	}
	select {
	case <-started:
		t.Fatal("duplicate load")
	default:
	}
	// Completed flights are not cached, and each result owns its mutable data.
	next, err := p.listing(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	out.Files[0] = "changed"
	out.Status["file"] = files.Deleted
	if next.Files[0] != "file" || next.Status["file"] != files.Modified {
		t.Fatal("shared mutable result")
	}
}

func TestProjectFlightAdmissionAndShutdown(t *testing.T) {
	p := newProjectIndex()
	started := make(chan struct{}, projectActiveLimit)
	p.load = func(ctx context.Context, _ string) (files.Listing, error) {
		select {
		case started <- struct{}{}:
		case <-ctx.Done():
			return files.Listing{}, ctx.Err()
		}
		<-ctx.Done()
		return files.Listing{}, ctx.Err()
	}
	results := make(chan error, projectActiveLimit+projectQueueLimit)
	for i := range projectActiveLimit + projectQueueLimit {
		root := fmt.Sprint(i)
		go func() { _, err := p.listing(context.Background(), root); results <- err }()
		waitProjectWaiters(t, p, root, 1)
	}
	for range projectActiveLimit {
		<-started
	}
	if _, err := p.listing(context.Background(), "overflow"); err != ErrProjectBusy {
		t.Fatalf("admission: %v", err)
	}
	select {
	case <-started:
		t.Fatal("too many active loaders")
	default:
	}
	p.stop()
	for range projectActiveLimit + projectQueueLimit {
		if err := <-results; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	p.start(context.Background())
	p.load = func(context.Context, string) (files.Listing, error) { return files.Listing{}, nil }
	if _, err := p.listing(context.Background(), "restart"); err != nil {
		t.Fatal(err)
	}
	p.stop()
}

func TestProjectFlightSupersededRetry(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	started, release := make(chan struct{}, 2), make(chan struct{})
	p.load = func(context.Context, string) (files.Listing, error) {
		started <- struct{}{}
		<-release
		return files.Listing{Files: []string{"file"}}, nil
	}
	done := make(chan error, 1)
	go func() { _, err := p.listing(context.Background(), "root"); done <- err }()
	<-started
	p.mu.Lock()
	p.epoch++
	p.active = make(map[string]*projectFlight)
	p.mu.Unlock()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	<-started // Superseded result was retried, not returned.
}

func TestProjectFlightRetryBound(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	calls := 0
	p.load = func(context.Context, string) (files.Listing, error) {
		calls++
		p.mu.Lock()
		p.epoch++
		p.mu.Unlock()
		return files.Listing{}, nil
	}
	// Two shared attempts are superseded; the private fallback still answers.
	if _, err := p.listing(context.Background(), "root"); err != nil || calls != 3 {
		t.Fatalf("retry bound: calls=%d err=%v", calls, err)
	}
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("private fallback retained")
	}
}

func TestProjectInvalidateCancelsSupersededLoader(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	started, canceled := make(chan struct{}, 3), make(chan struct{}, 3)
	var calls atomic.Int32
	p.load = func(ctx context.Context, root string) (files.Listing, error) {
		if calls.Add(1) > 1 {
			return files.Listing{Root: root, Files: []string{"fresh"}}, nil
		}
		started <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return files.Listing{}, ctx.Err()
	}
	done := make(chan files.Listing, 1)
	go func() {
		out, err := p.listing(context.Background(), "root")
		if err != nil {
			t.Error(err)
		}
		done <- out
	}()
	<-started
	p.invalidate("other")
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("superseded loader kept running")
	}
	if out := <-done; len(out.Files) != 1 || out.Files[0] != "fresh" {
		t.Fatalf("retry result %+v", out)
	}
}

// Save and Replace use this wrapper; it is the injection point for outcomes
// that real files.Save can only reach through a failing disk.
func TestProjectMutationUncertainOutcomes(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	p.load = func(_ context.Context, root string) (files.Listing, error) {
		return files.Listing{Root: root, Files: []string{"old"}}, nil
	}
	retain := func() {
		if _, err := p.listing(context.Background(), "/repo"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"incomplete", "panic"} {
		retain()
		before := p.epoch
		func() {
			defer func() {
				if r := recover(); (r != nil) != (name == "panic") {
					t.Fatalf("%s: recovered %v", name, r)
				}
			}()
			_, err := withProjectMutation(p, "/repo/sub", func() (files.SaveResult, error) {
				if p.epoch != before+1 {
					t.Fatal("write ran before invalidation")
				}
				retain() // A listing taken mid-write must not survive.
				if name == "panic" {
					panic("disk")
				}
				return files.SaveResult{}, files.ErrSaveIncomplete
			})
			if !errors.Is(err, files.ErrSaveIncomplete) {
				t.Fatal(err)
			}
		}()
		if p.epoch != before+2 {
			t.Fatalf("%s: epoch %d, want %d", name, p.epoch, before+2)
		}
		if _, _, ok := p.snapshot("/repo"); ok {
			t.Fatalf("%s: mid-write snapshot survived", name)
		}
	}
}

func TestProjectStopBoundsUncooperativeLoader(t *testing.T) {
	p := newProjectIndex()
	started, release := make(chan struct{}), make(chan struct{})
	p.load = func(context.Context, string) (files.Listing, error) {
		close(started)
		<-release // Models a hung mount that ignores cancellation.
		return files.Listing{}, nil
	}
	done := make(chan error, 1)
	go func() { _, err := p.listing(context.Background(), "root"); done <- err }()
	<-started
	begin := time.Now()
	p.stop()
	if elapsed := time.Since(begin); elapsed > projectStopGrace+time.Second {
		t.Fatalf("stop blocked %v", elapsed)
	}
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestProjectListingAuthorizationRevoked(t *testing.T) {
	s := NewService(nil, nil)
	defer s.projects.stop()
	root := t.TempDir()
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "synthetic"}
	s.catalog.Apply(provider.ScanResult{Changed: []model.SessionMeta{{Ref: ref, CWD: root}}})
	started, release := make(chan struct{}), make(chan struct{})
	s.projects.load = func(context.Context, string) (files.Listing, error) {
		close(started)
		<-release
		return files.Listing{Files: []string{"secret"}}, nil
	}
	if _, err := s.ListFiles(context.Background(), root+"/unknown"); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.ListFiles(context.Background(), root); done <- err }()
	<-started
	s.catalog.Remove([]model.SessionRef{ref})
	close(release)
	if err := <-done; !errors.Is(err, ErrUnknownDirectory) {
		t.Fatal(err)
	}
}

func TestProjectFlightLastWaiterAndError(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	started, canceled := make(chan struct{}), make(chan struct{})
	p.load = func(ctx context.Context, _ string) (files.Listing, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return files.Listing{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.listing(ctx, "root"); done <- err }()
	<-started
	cancel()
	if err := <-done; err != context.Canceled {
		t.Fatal(err)
	}
	<-canceled
	p.stop()
	p.start(context.Background())
	want := errors.New("load failed")
	p.load = func(context.Context, string) (files.Listing, error) { return files.Listing{}, want }
	for range 2 {
		if _, err := p.listing(context.Background(), "root"); err != want {
			t.Fatal(err)
		}
	}
}

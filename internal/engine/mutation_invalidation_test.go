package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// Each fixture is generated under TempDir, including upload staging and sources.
func TestMutationRouteInvalidation(t *testing.T) {
	type route struct {
		name string
		all  bool
		run  func(context.Context, *Engine, string, string) error
	}
	routes := []route{
		{"create-directory", false, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.CreateDirectory(ctx, root, "", "new")
		}},
		{"create-file", false, func(ctx context.Context, e *Engine, root, _ string) error { return e.CreateFile(ctx, root, "", "new") }},
		{"add", false, func(ctx context.Context, e *Engine, root, source string) error {
			return e.AddFile(ctx, root, "new", source)
		}},
		{"upload", false, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.CommitFileUpload(ctx, root, "new", "upload-fixture")
		}},
		{"rename", false, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.RenameFile(ctx, root, "old", "new")
		}},
		{"delete", false, func(ctx context.Context, e *Engine, root, _ string) error { return e.DeleteFile(ctx, root, "old") }},
		{"stage", true, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.GitStage(ctx, root, "", []string{"old"})
		}},
		{"unstage", true, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.GitUnstage(ctx, root, "", []string{"old"})
		}},
		{"discard", true, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.GitDiscard(ctx, root, "", files.GitDiscardRequest{})
		}},
		{"commit", true, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.GitCommit(ctx, root, "", files.GitCommitRequest{})
		}},
		{"switch", true, func(ctx context.Context, e *Engine, root, _ string) error {
			return e.GitSwitch(ctx, root, "", files.GitSwitchRequest{})
		}},
		{"sync", true, func(ctx context.Context, e *Engine, root, _ string) error {
			_, err := e.GitSync(ctx, root, "", files.GitSyncRequest{})
			return err
		}},
	}
	for _, route := range routes {
		for _, outcome := range []string{"success", "failure", "unauthorized"} {
			// Git fixtures intentionally have no repository: errors still invalidate all roots.
			if route.all && outcome == "success" {
				continue
			}
			t.Run(route.name+"/"+outcome, func(t *testing.T) {
				parent := t.TempDir()
				root, sibling := filepath.Join(parent, "root"), filepath.Join(parent, "sibling")
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
				cache := t.TempDir()
				staging := filepath.Join(cache, "staging")
				if err := os.MkdirAll(staging, 0700); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{filepath.Join(root, "old"), filepath.Join(staging, "upload-fixture")} {
					if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				source := filepath.Join(root, "old")
				catalog := scan.NewCatalog()
				catalog.Reset([]model.SessionMeta{{CWD: root}})
				e := &Engine{svc: NewService(catalog, nil), cacheDirOverride: cache}
				p := e.svc.projects
				t.Cleanup(p.stop)
				p.mu.Lock()
				for _, scope := range []string{parent, root, filepath.Join(root, "child"), sibling} {
					p.retainLocked(files.Listing{Root: scope, Files: []string{"old"}})
				}
				before := p.epoch
				p.mu.Unlock()
				ctx := context.Background()
				if outcome == "failure" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				dir := root
				if outcome == "unauthorized" {
					dir = sibling
				}
				err := route.run(ctx, e, dir, source)
				if outcome == "success" && err != nil {
					t.Fatal(err)
				}
				if outcome != "success" && err == nil {
					t.Fatal("expected refusal")
				}
				if outcome == "unauthorized" && !errors.Is(err, ErrUnknownDirectory) {
					t.Fatalf("authorization: %v", err)
				}
				p.mu.Lock()
				delta := p.epoch - before
				p.mu.Unlock()
				wantDelta := uint64(2)
				if outcome == "unauthorized" {
					wantDelta = 0
				}
				if delta != wantDelta {
					t.Fatalf("generation bumps = %d, want %d", delta, wantDelta)
				}
				for _, scope := range []string{parent, root, filepath.Join(root, "child"), sibling} {
					_, _, retained := p.snapshot(scope)
					want := outcome == "unauthorized" || (!route.all && scope == sibling)
					if retained != want {
						t.Errorf("retained(%q) = %v, want %v", scope, retained, want)
					}
				}
			})
		}
	}
}

func TestMutationRouteSupersedesInFlightListing(t *testing.T) {
	root := t.TempDir()
	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{CWD: root}})
	e := &Engine{svc: NewService(catalog, nil)}
	p := e.svc.projects
	entered, release, written := make(chan struct{}), make(chan struct{}, 1), make(chan struct{})
	defer p.stop()
	defer close(release)
	var calls atomic.Int32
	p.load = func(ctx context.Context, dir string) (files.Listing, error) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
				return files.Listing{Root: dir, Files: []string{"stale"}}, nil
			case <-ctx.Done():
				return files.Listing{}, ctx.Err()
			}
		}
		// The pre-write invalidation already supersedes the first loader, so a
		// retry can otherwise list the directory before the file exists. That
		// listing overlaps the write and may legitimately miss it; hold retries
		// until the write has returned so the result is deterministic.
		select {
		case <-written:
		case <-ctx.Done():
			return files.Listing{}, ctx.Err()
		}
		return files.List(ctx, dir)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type result struct {
		listing files.Listing
		err     error
	}
	done := make(chan result, 1)
	go func() { listing, err := e.svc.ListFiles(ctx, root); done <- result{listing, err} }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := e.CreateFile(ctx, root, "", "new"); err != nil {
		t.Fatal(err)
	}
	close(written)
	// Leave release open for deferred cleanup while unblocking the first loader.
	release <- struct{}{}
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if len(got.listing.Files) != 1 || got.listing.Files[0] != "new" {
			t.Fatalf("stale listing: %+v", got.listing)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Both pre- and post-write invalidations can supersede a shared attempt.
	if n := calls.Load(); n < 2 || n > 3 {
		t.Fatalf("loads = %d, want bounded retry", n)
	}
}

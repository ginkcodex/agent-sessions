package watch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// fakeFS is a filesystem watcher the test drives: it records watches and
// can refuse new ones.
type fakeFS struct {
	events chan fsnotify.Event
	errors chan error

	mu      sync.Mutex
	added   []string
	removed []string
	addErr  error
}

func newFakeFS() *fakeFS {
	return &fakeFS{events: make(chan fsnotify.Event), errors: make(chan error)}
}

func (f *fakeFS) Add(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, path)
	return f.addErr
}

func (f *fakeFS) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, path)
	return nil
}

func (f *fakeFS) Close() error                      { return nil }
func (f *fakeFS) WatchList() []string               { return nil }
func (f *fakeFS) EventsChan() <-chan fsnotify.Event { return f.events }
func (f *fakeFS) ErrorsChan() <-chan error          { return f.errors }

func (f *fakeFS) setAddErr(err error) {
	f.mu.Lock()
	f.addErr = err
	f.mu.Unlock()
}

func (f *fakeFS) counts() (added, removed int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.added), len(f.removed)
}

type recoverHarness struct {
	t     *testing.T
	w     *Watcher
	fs    *fakeFS
	root  string
	agent model.AgentID

	mu      sync.Mutex
	changes []Change
}

// startFake starts a watcher for one provider watching a temporary folder,
// with the fake filesystem watcher, no periodic polls outside fallback and
// the given fallback poll interval.
func startFake(t *testing.T, fallbackPoll time.Duration) *recoverHarness {
	t.Helper()
	h := &recoverHarness{t: t, fs: newFakeFS(), root: filepath.Join(t.TempDir(), "sessions"), agent: model.AgentCodex}
	mkdir(t, h.root)
	fake := providertest.NewFake(h.agent, "Codex")
	fake.Watch = []string{h.root}
	w, err := New(provider.Set{fake}, func(c Change) {
		h.mu.Lock()
		h.changes = append(h.changes, c)
		h.mu.Unlock()
	}, WithIntervals(10*time.Millisecond, 50*time.Millisecond, time.Hour, time.Hour, fallbackPoll))
	if err != nil {
		t.Fatal(err)
	}
	w.options.newFSWatcher = func() (fsWatcher, error) { return h.fs, nil }
	h.w = w
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() {
		cancel()
		_ = w.Close()
	})
	if added, _ := h.fs.counts(); added != 1 {
		t.Fatalf("start: %d watches, want 1", added)
	}
	return h
}

// state reports the provider's fallback flag and retry delay.
func (h *recoverHarness) state() (fallback bool, retryDelay time.Duration, retrying bool) {
	h.w.mu.Lock()
	defer h.w.mu.Unlock()
	s := h.w.states[h.agent]
	return s.fallback, s.retryDelay, !s.retryAt.IsZero()
}

func (h *recoverHarness) changeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.changes)
}

func (h *recoverHarness) sendError(err error) {
	h.t.Helper()
	select {
	case h.fs.errors <- err:
	case <-time.After(time.Second):
		h.t.Fatal("the watcher did not read its error channel")
	}
}

// eventually waits up to 3 s for cond.
func (h *recoverHarness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// An overflow lost events but no watches: the provider is rescanned in full
// and keeps its watches.
func TestOverflowRescansAndKeepsWatching(t *testing.T) {
	h := startFake(t, time.Hour)
	h.sendError(fmt.Errorf("inotify: %w", fsnotify.ErrEventOverflow))
	h.eventually("a rescan after the overflow", func() bool { return h.changeCount() > 0 })
	h.mu.Lock()
	c := h.changes[0]
	h.mu.Unlock()
	if c.Agent != h.agent || c.Paths != nil {
		t.Fatalf("change %+v, want a full rescan of %s", c, h.agent)
	}
	if fallback, _, _ := h.state(); fallback {
		t.Fatal("the overflow moved the provider to polling")
	}
	if _, removed := h.fs.counts(); removed != 0 {
		t.Fatalf("the overflow removed %d watches", removed)
	}
}

// Any other error moves the provider to polling, and it tries its watches
// again with a growing delay until they work.
func TestErrorFallsBackThenWatchesAgain(t *testing.T) {
	const poll = 40 * time.Millisecond
	h := startFake(t, poll)
	h.fs.setAddErr(errors.New("add refused"))
	h.sendError(errors.New("inotify read failed"))
	h.eventually("fallback", func() bool { f, _, _ := h.state(); return f })
	if _, removed := h.fs.counts(); removed != 1 {
		t.Fatalf("fallback removed %d watches, want 1", removed)
	}
	if _, delay, retrying := h.state(); !retrying || delay != poll {
		t.Fatalf("first retry: scheduled %v after %v, want after %v", retrying, delay, poll)
	}
	// The first try fails: the next waits twice as long.
	h.eventually("a failed retry", func() bool { added, _ := h.fs.counts(); return added >= 2 })
	h.eventually("the delay doubles", func() bool { _, d, _ := h.state(); return d == 2*poll })
	if fallback, _, retrying := h.state(); !fallback || !retrying {
		t.Fatalf("after a failed retry: fallback %v, retry scheduled %v", fallback, retrying)
	}
	// Polling goes on meanwhile.
	h.eventually("a fallback poll", func() bool { return h.changeCount() > 0 })

	h.fs.setAddErr(nil)
	h.eventually("watching again", func() bool { f, _, _ := h.state(); return !f })
	h.fs.mu.Lock()
	last := h.fs.added[len(h.fs.added)-1]
	h.fs.mu.Unlock()
	if last != h.root {
		t.Fatalf("watching again: last watch %s, want %s", last, h.root)
	}
	h.w.mu.Lock()
	_, watched := h.w.states[h.agent].watched[h.root]
	h.w.mu.Unlock()
	if !watched {
		t.Fatal("watching again: the root is not in the watched set")
	}
}

// A full watch table waits the longest before trying again.
func TestWatchLimitWaitsLongest(t *testing.T) {
	const poll = 30 * time.Millisecond
	h := startFake(t, poll)
	h.fs.setAddErr(fmt.Errorf("add: %w", syscall.ENOSPC))
	h.sendError(errors.New("inotify read failed"))
	h.eventually("a retry that hits the limit", func() bool {
		_, d, _ := h.state()
		return d == retryBackoffLimit*poll
	})
	if fallback, _, retrying := h.state(); !fallback || !retrying {
		t.Fatalf("at the limit: fallback %v, retry scheduled %v", fallback, retrying)
	}
}

// A filesystem watcher whose channels closed is gone: the provider polls and
// never tries to watch again.
func TestClosedWatcherPollsWithoutRetry(t *testing.T) {
	h := startFake(t, 20*time.Millisecond)
	close(h.fs.errors)
	h.eventually("fallback", func() bool { f, _, _ := h.state(); return f })
	if _, _, retrying := h.state(); retrying {
		t.Fatal("a closed watcher scheduled a retry")
	}
	h.eventually("a fallback poll", func() bool { return h.changeCount() > 0 })
	time.Sleep(100 * time.Millisecond)
	h.fs.mu.Lock()
	added := slices.Clone(h.fs.added)
	h.fs.mu.Unlock()
	if len(added) != 1 {
		t.Fatalf("a closed watcher added watches again: %q", added)
	}
}

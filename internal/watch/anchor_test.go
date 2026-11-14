package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// recordingWatcher is the real fsnotify watcher, recording each Add.
type recordingWatcher struct {
	*nativeWatcher
	mu    sync.Mutex
	added []string
}

func (r *recordingWatcher) Add(path string) error {
	r.mu.Lock()
	r.added = append(r.added, path)
	r.mu.Unlock()
	return r.nativeWatcher.Add(path)
}

func (r *recordingWatcher) adds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.added)
}

type anchorHarness struct {
	t       *testing.T
	fs      *recordingWatcher
	mu      sync.Mutex
	changes []Change
}

// startRecorded starts a watcher for p with the recording filesystem
// watcher, a short debounce and no periodic polls.
func startRecorded(t *testing.T, p provider.Provider) *anchorHarness {
	t.Helper()
	h := &anchorHarness{t: t}
	w, err := New(provider.Set{p}, func(c Change) {
		h.mu.Lock()
		h.changes = append(h.changes, c)
		h.mu.Unlock()
	}, WithIntervals(20*time.Millisecond, 100*time.Millisecond, time.Hour, time.Hour, time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	w.options.newFSWatcher = func() (fsWatcher, error) {
		native, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, err
		}
		h.fs = &recordingWatcher{nativeWatcher: &nativeWatcher{Watcher: native}}
		return h.fs, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)
	t.Cleanup(func() {
		cancel()
		_ = w.Close()
	})
	return h
}

func (h *anchorHarness) changeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.changes)
}

// settle waits long enough for an event to be handled and dispatched.
func settle() { time.Sleep(400 * time.Millisecond) }

func (h *anchorHarness) waitChange(what string) {
	h.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for h.changeCount() == 0 {
		if time.Now().After(deadline) {
			h.t.Fatalf("%s: no change", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if c := h.changes[len(h.changes)-1]; c.Paths != nil {
		h.t.Fatalf("%s: change %+v, want a full rescan", what, c)
	}
	h.changes = nil
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// Without ~/.codex/sessions the watcher used to watch $HOME, then watched
// every new directory in it recursively and rescanned Codex for each change.
func TestMissingRootWatchesOnlyForItsCreation(t *testing.T) {
	home := t.TempDir()
	sessions := filepath.Join(home, ".codex", "sessions")
	fake := providertest.NewFake(model.AgentCodex, "Codex")
	fake.Watch = []string{sessions}
	h := startRecorded(t, fake)
	if got := h.fs.adds(); !slices.Equal(got, []string{home}) {
		t.Fatalf("start: watches %q, want only %s", got, home)
	}

	// Unrelated work in $HOME: no watches, no rescans.
	mkdir(t, filepath.Join(home, "proj", "a", "b"))
	if err := os.WriteFile(filepath.Join(home, "proj", "a", "b", "rollout-x.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	settle()
	if got := h.fs.adds(); len(got) != 1 {
		t.Fatalf("a new project in $HOME added watches: %q", got)
	}
	if n := h.changeCount(); n != 0 {
		t.Fatalf("a new project in $HOME queued %d rescans", n)
	}

	// ~/.codex appears: it becomes the anchor; still nothing to rescan.
	mkdir(t, filepath.Join(home, ".codex"))
	settle()
	if got := h.fs.adds(); !slices.Equal(got, []string{home, filepath.Join(home, ".codex")}) {
		t.Fatalf("after ~/.codex: watches %q", got)
	}
	mkdir(t, filepath.Join(home, ".codex", "log", "deep"))
	settle()
	if got, n := h.fs.adds(), h.changeCount(); len(got) != 2 || n != 0 {
		t.Fatalf("a new ~/.codex/log: watches %q, %d rescans", got, n)
	}

	// The root appears: it is watched recursively and rescanned.
	mkdir(t, filepath.Join(sessions, "2026", "11"))
	h.waitChange("root created")
	settle()
	got := h.fs.adds()
	if !slices.Contains(got, sessions) || !slices.Contains(got, filepath.Join(sessions, "2026", "11")) {
		t.Fatalf("root created: watches %q", got)
	}
	h.mu.Lock()
	h.changes = nil
	h.mu.Unlock()
	if err := os.WriteFile(filepath.Join(sessions, "2026", "11", "rollout-a.jsonl"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for h.changeCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.changeCount() == 0 {
		t.Fatal("a rollout in the new root: no change")
	}

	// The root is removed and comes back: the anchor sees it again.
	if err := os.RemoveAll(sessions); err != nil {
		t.Fatal(err)
	}
	settle()
	h.mu.Lock()
	h.changes = nil
	h.mu.Unlock()
	mkdir(t, sessions)
	h.waitChange("root created again")
}

// A non-recursive root, the folder holding a database, does not watch its
// new subdirectories or rescan for them.
func TestNonRecursiveRootIgnoresNewDirectories(t *testing.T) {
	data := t.TempDir()
	db := filepath.Join(data, "opencode.db")
	if err := os.WriteFile(db, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fake := providertest.NewFake(model.AgentOpenCode, "OpenCode")
	fake.Watch = []string{db}
	h := startRecorded(t, fake)
	mkdir(t, filepath.Join(data, "snapshot", "a", "b"))
	settle()
	if got, n := h.fs.adds(), h.changeCount(); !slices.Equal(got, []string{data}) || n != 0 {
		t.Fatalf("a new snapshot directory: watches %q, %d rescans", got, n)
	}
	if err := os.WriteFile(db, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.waitChange("database write")
}

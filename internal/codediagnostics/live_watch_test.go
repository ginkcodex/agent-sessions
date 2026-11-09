package codediagnostics

// SYNTHETIC: fake fsnotify events and fake language servers (RL-10). They
// prove which saved-file transitions the project watcher reports and to whom,
// not how ruby-lsp re-indexes (RL-07 checks the real server).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchEnv is an approved attachment with live servers behind LiveDocuments,
// the fake servers recording what they receive.
type watchEnv struct {
	t    *testing.T
	m    *Manager
	d    *LiveDocuments
	rec  *freshnessServers
	root AuthorizedRoot
	dir  string
	s    Snapshot

	mu      sync.Mutex
	batches [][]FileChange
}

// newWatchEnv registers RuboCop and ruby-lsp, plus gopls and go-syntax when
// withGo, and approves them. Nothing is opened yet.
func newWatchEnv(t *testing.T, withGo bool, files map[string]string) *watchEnv {
	t.Helper()
	e := newEnv(t, SessionOptions{})
	root, tools, _ := fixture(t)
	_ = os.Remove(filepath.Join(root.Project().Root, "source.go"))
	writeTree(t, root.Project().Root, files)
	rec := &freshnessServers{got: map[string][]map[string]any{}, seen: make(chan struct{}, 1)}
	e.script = rec.script
	state := func(id string) ProviderState {
		return ProviderState{ProviderID: id, Availability: "available", Phase: "idle", Freshness: "uncertain", Coverage: Coverage{Kind: "openedDocuments", Roots: []string{""}}, Notice: id + " notice."}
	}
	server := func(id string) ServerConfig {
		return ServerConfig{ProviderID: id, Executable: tools[0].Executable, Args: []string{id}, AdapterPolicy: "fake-" + id}
	}
	ruby := func(file string) (string, bool) { return RubyLanguageID(file) }
	regs := []Registration{
		liveRegistration(state(rubocopProviderID), server(rubocopProviderID), ruby),
		liveRegistration(state(rubyLspProviderID), server(rubyLspProviderID), ruby),
	}
	if withGo {
		regs = append(regs,
			liveRegistration(state(goplsProviderID), server(goplsProviderID), func(file string) (string, bool) { return "go", strings.HasSuffix(file, ".go") }),
			Registration{ProviderID: GoSyntaxProviderID, Availability: "available", InProcess: true})
	}
	m, err := NewManager("owner", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	d := NewLiveDocuments(e.trust, m, e.mgr.options)
	t.Cleanup(func() { m.Close(); d.Close() })
	if err := d.Register(root, regs); err != nil {
		t.Fatal(err)
	}
	m.SetDocumentProvider(d)
	s, err := m.Attach(root, 1, regs)
	if err != nil {
		t.Fatal(err)
	}
	return &watchEnv{t: t, m: m, d: d, rec: rec, root: root, dir: root.Project().Root, s: s}
}

func (w *watchEnv) approve() {
	w.t.Helper()
	s, err := w.m.Approve(w.s.Context, w.s.Proposal.Fingerprint, "session")
	if err != nil {
		w.t.Fatal(err)
	}
	w.s = s
}

func (w *watchEnv) open(path string) {
	w.t.Helper()
	if _, err := w.m.OpenDocument(context.Background(), w.s.Context, path, path, 1, "x", ""); err != nil {
		w.t.Fatal(err)
	}
}

// options forward every reported batch to LiveDocuments, as the engine does,
// and keep a copy.
func (w *watchEnv) options(fs FSWatcher, timer chan time.Time) ProjectWatchOptions {
	return ProjectWatchOptions{
		NewFS: func() (FSWatcher, error) { return fs, nil },
		After: func(time.Duration) <-chan time.Time { return timer },
		OnChanges: func(ctx context.Context, changes []FileChange) {
			w.mu.Lock()
			w.batches = append(w.batches, changes)
			w.mu.Unlock()
			_ = w.d.WatchedFiles(ctx, w.root, changes)
		},
	}
}

func (w *watchEnv) take() [][]FileChange {
	w.mu.Lock()
	defer w.mu.Unlock()
	b := w.batches
	w.batches = nil
	return b
}

// watched waits for provider's didChangeWatchedFiles, if want is not empty.
func (w *watchEnv) watched(provider string, want ...string) {
	w.t.Helper()
	var got []string
	done := func() bool {
		for _, m := range w.rec.take(provider) {
			if strings.HasPrefix(m, "workspace/didChangeWatchedFiles") {
				got = append(got, m)
			}
		}
		return len(got) >= len(want)
	}
	if len(want) == 0 {
		time.Sleep(50 * time.Millisecond)
		done()
	} else {
		eventually(w.t, provider+" watched files", done)
	}
	if !slices.Equal(got, want) {
		w.t.Fatalf("%s got %v, want %v", provider, got, want)
	}
}

func (w *watchEnv) event(fs *fakeFS, rel string, op fsnotify.Op) {
	fs.events <- fsnotify.Event{Name: filepath.Join(w.dir, filepath.FromSlash(rel)), Op: op}
}

// fireTimer sends the debounce tick once every queued event was taken.
func fireTimer(fs *fakeFS, timer chan time.Time) {
	for len(fs.events) > 0 || len(fs.errors) > 0 {
		time.Sleep(time.Millisecond)
	}
	timer <- time.Now()
}

// fireTimerOrDone is fireTimer for a watcher that may end first: after a
// revoke it can stop before it reads the queued events, or the tick.
func fireTimerOrDone(fs *fakeFS, timer chan time.Time, done <-chan struct{}) {
	for len(fs.events) > 0 || len(fs.errors) > 0 {
		select {
		case <-done:
			return
		case <-time.After(time.Millisecond):
		}
	}
	select {
	case timer <- time.Now():
	case <-done:
	}
}

// awaitWatcherReady waits for the event loop: adding the root watch is too
// early, since full performs a second scan before the initial publication.
func awaitWatcherReady(t *testing.T, fs *fakeFS) {
	t.Helper()
	select {
	case <-fs.ready:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not finish its initial scans and publication")
	}
}

func providerState(s Snapshot, id string) ProviderState {
	for _, p := range s.Providers {
		if p.ProviderID == id {
			return p
		}
	}
	return ProviderState{}
}

// startupBlockedFS pauses after installing the first watch, before the
// startup re-scan. This makes the old root-watch readiness race deterministic.
type startupBlockedFS struct {
	*fakeFS
	installed chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (f *startupBlockedFS) Add(path string) error {
	if err := f.fakeFS.Add(path); err != nil {
		return err
	}
	f.once.Do(func() {
		close(f.installed)
		<-f.release
	})
	return nil
}

func TestProjectWatcherReadyAfterStartupScans(t *testing.T) {
	w := newWatchEnv(t, false, map[string]string{"lib/b.rb": "b"})
	w.approve()
	w.open("lib/b.rb")
	w.rec.take(rubyLspProviderID)
	fs, timer := newFakeFS(), make(chan time.Time)
	blocked := &startupBlockedFS{fakeFS: fs, installed: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(blocked.release) }) }
	pw := StartProjectWatcher(w.m, w.s.Context, w.root, w.options(blocked, timer))
	defer pw.Stop()
	defer unblock()
	select {
	case <-blocked.installed:
	case <-time.After(5 * time.Second):
		t.Fatal("root watch was not installed")
	}
	if !fs.watching(w.dir) {
		t.Fatal("root is not watched")
	}
	select {
	case <-fs.ready:
		t.Fatal("watcher ready before startup re-scan")
	default:
	}
	writeTree(t, w.dir, map[string]string{"lib/b.rb": "during startup"})
	unblock()
	awaitWatcherReady(t, fs)
	if got := w.take(); len(got) != 0 {
		t.Fatalf("startup reported changes: %v", got)
	}
	writeTree(t, w.dir, map[string]string{"lib/b.rb": "after startup, a saved change"})
	w.event(fs, "lib/b.rb", fsnotify.Write)
	fireTimer(fs, timer)
	w.watched(rubyLspProviderID, "workspace/didChangeWatchedFiles b.rb")
	if got := w.take(); len(got) != 1 || !slices.Equal(got[0], []FileChange{{"lib/b.rb", 2}}) {
		t.Fatalf("saved change after startup: %v", got)
	}
}

// A Ruby-only approved project: saved Ruby transitions reach ruby-lsp and
// nothing reaches RuboCop; other files, skipped trees and metadata-only events
// report nothing; the initial scan is not reported; nothing is published.
func TestProjectWatcherRubyOnly(t *testing.T) {
	w := newWatchEnv(t, false, map[string]string{"app.rb": "x", "lib/b.rb": "b", "gone.rb": "g", "Gemfile": "", "notes.txt": ""})
	w.approve()
	w.open("app.rb")
	w.rec.take(rubyLspProviderID)
	w.rec.take(rubocopProviderID)
	before, _ := w.m.GetSnapshot(w.s.Context)
	fs, timer := newFakeFS(), make(chan time.Time)
	pw := StartProjectWatcher(w.m, w.s.Context, w.root, w.options(fs, timer))
	defer pw.Stop()
	awaitWatcherReady(t, fs)
	if !fs.watching(filepath.Join(w.dir, "lib")) || !slices.Equal(pw.live, []string{rubyLspProviderID}) || pw.goSyntax {
		t.Fatalf("watching %v, live %v, go-syntax %v", fs.added, pw.live, pw.goSyntax)
	}

	writeTree(t, w.dir, map[string]string{"lib/b.rb": "changed", "c.rake": "new", "Gemfile": "changed", "notes.txt": "changed", "vendor/bundle/v.rb": "", ".hidden.rb": ""})
	if err := os.Remove(filepath.Join(w.dir, "gone.rb")); err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		rel string
		op  fsnotify.Op
	}{{"lib/b.rb", fsnotify.Write}, {"lib/b.rb", fsnotify.Write}, {"c.rake", fsnotify.Create}, {"gone.rb", fsnotify.Remove}, {"Gemfile", fsnotify.Write}, {"notes.txt", fsnotify.Write}, {"vendor/bundle/v.rb", fsnotify.Create}, {".hidden.rb", fsnotify.Create}, {"app.rb", fsnotify.Chmod}} {
		w.event(fs, e.rel, e.op)
	}
	fireTimer(fs, timer)
	w.watched(rubyLspProviderID, "workspace/didChangeWatchedFiles c.rake gone.rb b.rb")
	w.watched(rubocopProviderID)
	if got := w.take(); len(got) != 1 || !slices.Equal(got[0], []FileChange{{"c.rake", 1}, {"gone.rb", 3}, {"lib/b.rb", 2}}) {
		t.Fatalf("batches %v", got)
	}
	// A metadata-only event on an owned file reports nothing.
	w.event(fs, "app.rb", fsnotify.Chmod)
	fireTimer(fs, timer)
	// A new directory is a rescan: its Ruby file is created, and watched.
	writeTree(t, w.dir, map[string]string{"app/models/user.rb": "u"})
	w.event(fs, "app", fsnotify.Create)
	fireTimer(fs, timer)
	w.watched(rubyLspProviderID, "workspace/didChangeWatchedFiles user.rb")
	if got := w.take(); len(got) != 1 || !slices.Equal(got[0], []FileChange{{"app/models/user.rb", 1}}) || !fs.watching(filepath.Join(w.dir, "app", "models")) {
		t.Fatalf("rescan batches %v", got)
	}
	after, _ := w.m.GetSnapshot(w.s.Context)
	p := providerState(after, rubyLspProviderID)
	if len(after.ResultSets) != len(before.ResultSets) || p.Coverage.Partial || slices.ContainsFunc([]string{liveWatchUnavailable, liveWatchLimited, liveWatchStopped}, func(n string) bool { return strings.Contains(p.Notice, n) }) {
		t.Fatalf("watcher published: %+v", after)
	}
}

// With Go sources, one watcher serves go-syntax, gopls and ruby-lsp: Go
// transitions come from go-syntax's analysis, Ruby ones from tracking.
func TestProjectWatcherGoAndRuby(t *testing.T) {
	w := newWatchEnv(t, true, map[string]string{"x.go": "package x\n", "a.rb": "a"})
	w.approve()
	w.open("a.rb")
	w.open("x.go")
	fs, timer := newFakeFS(), make(chan time.Time)
	pw := StartProjectWatcher(w.m, w.s.Context, w.root, w.options(fs, timer))
	defer pw.Stop()
	awaitSnapshot(t, w.m, w.s.Context, func(s Snapshot) bool {
		return providerState(s, GoSyntaxProviderID).Coverage.ExternalChanges == "watching"
	})
	if !pw.goSyntax || !slices.Equal(pw.live, []string{rubyLspProviderID}) {
		t.Fatalf("go-syntax %v, live %v", pw.goSyntax, pw.live)
	}
	w.rec.take(goplsProviderID)
	w.rec.take(rubyLspProviderID)
	writeTree(t, w.dir, map[string]string{"x.go": "package x\nfunc {", "a.rb": "changed"})
	w.event(fs, "x.go", fsnotify.Write)
	w.event(fs, "a.rb", fsnotify.Write)
	s, _ := w.m.GetSnapshot(w.s.Context)
	fireTimer(fs, timer)
	awaitSnapshot(t, w.m, w.s.Context, func(n Snapshot) bool {
		set, _ := syntaxSet(n)
		return n.Sequence > s.Sequence && slices.Equal(paths(set), []string{"x.go"})
	})
	w.watched(goplsProviderID, "workspace/didChangeWatchedFiles x.go")
	w.watched(rubyLspProviderID, "workspace/didChangeWatchedFiles a.rb")
	w.watched(rubocopProviderID)
	if got := w.take(); len(got) != 1 || !slices.Equal(got[0], []FileChange{{"a.rb", 2}, {"x.go", 2}}) {
		t.Fatalf("batches %v", got)
	}
}

// No watcher scans before approval or without a consumer, and after revoke it
// reports nothing and ends.
func TestProjectWatcherLiveApproval(t *testing.T) {
	w := newWatchEnv(t, false, map[string]string{"a.rb": "a"})
	never := func() (FSWatcher, error) { t.Error("fsnotify created"); return nil, errors.New("x") }
	if w.m.LiveWatched(w.s.Context) {
		t.Fatal("watched before approval")
	}
	pw := StartProjectWatcher(w.m, w.s.Context, w.root, ProjectWatchOptions{NewFS: never, OnChanges: func(context.Context, []FileChange) {}})
	<-pw.Done()
	w.approve()
	if !w.m.LiveWatched(w.s.Context) {
		t.Fatal("approved ruby-lsp not watched")
	}
	// Without OnChanges, nobody takes the transitions.
	pw = StartProjectWatcher(w.m, w.s.Context, w.root, ProjectWatchOptions{NewFS: never})
	<-pw.Done()

	fs, timer := newFakeFS(), make(chan time.Time)
	pw = StartProjectWatcher(w.m, w.s.Context, w.root, w.options(fs, timer))
	awaitWatcherReady(t, fs)
	revoked, err := w.m.Revoke(w.s.Context)
	if err != nil {
		t.Fatal(err)
	}
	if w.m.LiveWatched(revoked.Context) || w.m.LiveWatched(w.s.Context) {
		t.Fatal("watched after revoke")
	}
	writeTree(t, w.dir, map[string]string{"a.rb": "changed"})
	w.event(fs, "a.rb", fsnotify.Write)
	// The watcher may stop on revoke before reading the queued events or tick.
	fireTimerOrDone(fs, timer, pw.Done())
	<-pw.Done()
	if got := w.take(); len(got) != 0 {
		t.Fatalf("reported after revoke: %v", got)
	}
	if w.m.OwnedWorkers() != 0 {
		t.Fatal("owned work leaked")
	}
}

// Limits, unavailable fsnotify and a stopped watcher mark ruby-lsp's coverage
// partial and say so in its notice; RuboCop's is untouched.
func TestProjectWatcherLiveDegrades(t *testing.T) {
	check := func(w *watchEnv, note string) {
		t.Helper()
		s := awaitSnapshot(t, w.m, w.s.Context, func(s Snapshot) bool {
			return strings.Contains(providerState(s, rubyLspProviderID).Notice, note)
		})
		p := providerState(s, rubyLspProviderID)
		if p.Notice != "ruby-lsp notice. "+note || !p.Coverage.Partial {
			t.Fatalf("%+v", p)
		}
		if r := providerState(s, rubocopProviderID); r.Notice != "rubocop notice." || r.Coverage.Partial {
			t.Fatalf("RuboCop changed: %+v", r)
		}
	}
	files := map[string]string{"a.rb": "a", "lib/b.rb": "b"}
	forward := func(context.Context, []FileChange) {}

	w := newWatchEnv(t, false, files)
	w.approve()
	pw := StartProjectWatcher(w.m, w.s.Context, w.root, ProjectWatchOptions{NewFS: func() (FSWatcher, error) { return nil, errors.New("unsupported") }, OnChanges: forward})
	<-pw.Done()
	check(w, liveWatchUnavailable)

	w = newWatchEnv(t, false, files)
	w.approve()
	fs := newFakeFS()
	timer := make(chan time.Time)
	batches := make(chan []FileChange, 1)
	pw = StartProjectWatcher(w.m, w.s.Context, w.root, ProjectWatchOptions{NewFS: func() (FSWatcher, error) { return fs, nil }, After: func(time.Duration) <-chan time.Time { return timer }, MaxDirs: 1, OnChanges: func(_ context.Context, c []FileChange) { batches <- c }})
	check(w, liveWatchLimited)
	if fs.watching(filepath.Join(w.dir, "lib")) {
		t.Fatal("watched beyond MaxDirs")
	}
	// A later batch, still limited, does not repeat the note.
	writeTree(t, w.dir, map[string]string{"a.rb": "changed"})
	w.event(fs, "a.rb", fsnotify.Write)
	fireTimer(fs, timer)
	if got := <-batches; !slices.Equal(got, []FileChange{{"a.rb", 2}}) {
		t.Fatalf("limited batch %v", got)
	}
	close(fs.events)
	<-pw.Done()
	s, _ := w.m.GetSnapshot(w.s.Context)
	if n := providerState(s, rubyLspProviderID).Notice; n != "ruby-lsp notice. "+liveWatchLimited+" "+liveWatchStopped {
		t.Fatalf("stopped: %q", n)
	}

	w = newWatchEnv(t, false, files)
	w.approve()
	fs = newFakeFS()
	pw = StartProjectWatcher(w.m, w.s.Context, w.root, ProjectWatchOptions{NewFS: func() (FSWatcher, error) { return fs, nil }, Limits: GoSyntaxLimits{Entries: 2}, OnChanges: forward})
	defer pw.Stop()
	check(w, liveWatchLimited)
}

// Tracked files skip what the watcher skips ("." and "_" names, vendor). A
// walk cut short by the entry limit does not report the files it did not
// reach as deleted; a complete walk does.
func TestProjectScanTracksWithoutContents(t *testing.T) {
	root, dir := syntaxRoot(t, map[string]string{"a.rb": "a", "b.rb": "b", "z/c.rb": "c", "x.go": "package x\nfunc {", ".hidden.rb": "", "_tmp.rb": "", "vendor/bundle/g.rb": ""})
	g := newProjectScan(root, GoSyntaxLimits{}, false, watchedBy([]string{rubyLspProviderID}))
	result, _, changes, err := g.Full()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changes, []FileChange{{"a.rb", 1}, {"b.rb", 1}, {"z/c.rb", 1}}) || len(result.Diagnostics) != 0 || len(g.cache) != 0 {
		t.Fatalf("initial %v %+v", changes, result)
	}
	g.limits.Entries = 4
	if _, _, changes, _ = g.Full(); len(changes) != 0 || !g.truncated || len(g.saved) != 3 {
		t.Fatalf("truncated walk: %v %v", changes, g.saved)
	}
	g.limits.Entries = 50000
	if err := os.Remove(filepath.Join(dir, "z", "c.rb")); err != nil {
		t.Fatal(err)
	}
	if _, _, changes, _ = g.Full(); !slices.Equal(changes, []FileChange{{"z/c.rb", 3}}) || g.truncated {
		t.Fatalf("complete walk: %v", changes)
	}
	// Update compares size and time; an unchanged file reports nothing.
	writeTree(t, dir, map[string]string{"a.rb": "longer"})
	if _, changes, _ = g.Update([]string{"a.rb", "b.rb", "x.go", "nope.rb"}); !slices.Equal(changes, []FileChange{{"a.rb", 2}}) {
		t.Fatalf("update: %v", changes)
	}
}

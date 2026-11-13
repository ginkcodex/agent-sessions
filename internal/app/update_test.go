package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/update"
)

const updateBody = "installer bytes"

// readyUpdater is a manager with 1.6.0 downloaded and checked. applied
// records what Apply was asked to install.
type readyUpdater struct {
	M       *update.Manager
	mu      sync.Mutex
	applied []bool // relaunch flag of each Apply
}

func (r *readyUpdater) calls() []bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.applied...)
}

func newUpdater(t *testing.T, supported bool, auto func() bool) *readyUpdater {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(updateBody))
	}))
	t.Cleanup(srv.Close)
	r := &readyUpdater{}
	r.M = &update.Manager{
		Store:     &update.Store{Dir: filepath.Join(t.TempDir(), "updates"), HTTP: http.DefaultClient},
		Supported: func() bool { return supported },
		Auto:      auto,
		Apply: func(_ update.Ready, relaunch bool) error {
			r.mu.Lock()
			r.applied = append(r.applied, relaunch)
			r.mu.Unlock()
			return nil
		},
	}
	return r
}

func offer(m *update.Manager, srvURL string, signed bool) {
	sum := sha256.Sum256([]byte(updateBody))
	info := update.Info{Current: "1.5.5", Latest: "1.6.0", URL: update.ReleasesPage + "/tag/v1.6.0", Available: true}
	if signed {
		info.Release = &update.Release{
			Version: "1.6.0",
			File:    "agent-sessions_1.6.0_macos_universal.dmg",
			URL:     srvURL + "/agent-sessions_1.6.0_macos_universal.dmg",
			SHA256:  hex.EncodeToString(sum[:]),
		}
	}
	m.Offer(info)
}

func readyNow(t *testing.T) *readyUpdater {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(updateBody))
	}))
	t.Cleanup(srv.Close)
	r := newUpdater(t, true, nil)
	offer(r.M, srv.URL, true)
	waitFor(t, "update ready", func() bool { return r.M.Status().State == update.StateReady })
	return r
}

func TestAutoUpdateSettingIsOnByDefaultAndSaved(t *testing.T) {
	a := &App{roots: paths.Roots{Config: t.TempDir()}}
	if on, err := a.AutoUpdate(); err != nil || !on {
		t.Fatalf("AutoUpdate = %v, %v; want on by default", on, err)
	}
	if err := a.SetAutoUpdate(false); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.AutoUpdate(); on {
		t.Fatal("the setting was not saved")
	}
	data, _ := os.ReadFile(filepath.Join(a.roots.Config, "config.toml"))
	if autoUpdateFrom(data) {
		t.Fatalf("config = %q still has updates on", data)
	}
	if !autoUpdateAt(t.TempDir()) {
		t.Fatal("a missing config turned updates off")
	}
	if autoUpdateAt(a.roots.Config) {
		t.Fatal("the manager does not see the saved setting")
	}
	if err := a.SetAutoUpdate(true); err != nil {
		t.Fatal(err)
	}
	if on, _ := a.AutoUpdate(); !on {
		t.Fatal("turning it back on was not saved")
	}
	// Other settings in the file survive.
	if err := os.WriteFile(filepath.Join(a.roots.Config, "config.toml"), []byte("[terminal]\napp = \"kitty\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAutoUpdate(false); err != nil {
		t.Fatal(err)
	}
	if id, _ := a.savedTerminal(); id != "kitty" {
		t.Fatalf("terminal setting = %q after saving the update setting", id)
	}
	// A value that is not a boolean leaves updates on.
	if !autoUpdateFrom([]byte("[update]\nauto = maybe\n")) {
		t.Fatal("garbage turned updates off")
	}
}

func TestSettingTurnedOffStopsTheSharedDownload(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	var auto = true
	var mu sync.Mutex
	r := newUpdater(t, true, func() bool { mu.Lock(); defer mu.Unlock(); return auto })
	a := &App{roots: paths.Roots{Config: t.TempDir()}, updater: r.M}

	offer(r.M, srv.URL, true)
	<-started
	if a.UpdateState().State != update.StateDownloading {
		t.Fatalf("state = %+v", a.UpdateState())
	}
	mu.Lock()
	auto = false
	mu.Unlock()
	if err := a.SetAutoUpdate(false); err != nil {
		t.Fatal(err)
	}
	if s := a.UpdateState(); s.State != update.StateManual || s.Version != "1.6.0" || s.URL == "" {
		t.Fatalf("state = %+v", s)
	}
}

func TestUpdateStateWithoutAnUpdater(t *testing.T) {
	a := &App{}
	if s := a.UpdateState(); s.State != update.StateIdle {
		t.Fatalf("state = %+v", s)
	}
	if err := a.RestartToUpdate(); !errors.Is(err, errNoUpdate) {
		t.Fatalf("RestartToUpdate = %v", err)
	}
	a.watchUpdates()() // nothing to watch, nothing to stop
}

func TestUnsupportedInstallKeepsTheReleasePage(t *testing.T) {
	r := newUpdater(t, false, nil)
	a := &App{updater: r.M}
	offer(r.M, "http://unused.invalid", true)
	if s := a.UpdateState(); s.State != update.StateUnsupported || s.Version != "1.6.0" || s.URL != update.ReleasesPage+"/tag/v1.6.0" {
		t.Fatalf("state = %+v", s)
	}
	if err := a.RestartToUpdate(); !errors.Is(err, errNoUpdate) {
		t.Fatalf("RestartToUpdate = %v", err)
	}
}

func TestDefaultUpdaterCannotUpdateYet(t *testing.T) {
	// Until a platform's installer is wired in, a newer release only shows
	// its page, as before.
	m := newUpdateManager()
	if m.Supported() {
		t.Fatal("the default updater claims it can install")
	}
}

func TestUpdateStateChangesReachTheWindow(t *testing.T) {
	r := newUpdater(t, false, nil)
	events := make(chan update.Status, 4)
	a := &App{updater: r.M}
	a.desktopEvents = emitFunc(func(name string, payload any) {
		if name == UpdateStateEvent {
			events <- payload.(update.Status)
		}
	})
	stop := a.watchUpdates()
	offer(r.M, "http://unused.invalid", true)
	select {
	case s := <-events:
		if s.State != update.StateUnsupported {
			t.Fatalf("event = %+v", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no update:state event")
	}
	stop()
	r.M.Offer(update.Info{Available: false})
	select {
	case s := <-events:
		t.Fatalf("event after stop: %+v", s)
	case <-time.After(50 * time.Millisecond):
	}
}

type emitFunc func(name string, payload any)

func (f emitFunc) Emit(name string, payload any) { f(name, payload) }

func TestQuittingInstallsTheReadyUpdate(t *testing.T) {
	stubRestart(t, func() error { t.Error("started another copy"); return nil })
	w := testWindows(t)
	quits := quitCounter(w)
	r := readyNow(t)
	w.updater = r.M
	a := openQuitWindow(t, w, false)

	w.ShouldQuit()
	receive(t, quits, "quit")
	w.Shutdown()
	if got := r.calls(); len(got) != 1 || got[0] {
		t.Fatalf("Apply calls = %v, want one without relaunch", got)
	}
	_ = a
}

func TestRestartToUpdateInstallsAndRelaunchesWithoutAnotherCopy(t *testing.T) {
	stubRestart(t, func() error { t.Error("started another copy next to the installer"); return nil })
	w := testWindows(t)
	quits := quitCounter(w)
	r := readyNow(t)
	w.updater = r.M
	a := openQuitWindow(t, w, true)

	if err := a.app.RestartToUpdate(); err != nil {
		t.Fatal(err)
	}
	// The usual question about unsaved files still comes first.
	a.app.CompleteFileClose(receive(t, a.requests, "close request"), true)
	receive(t, quits, "quit")
	w.Shutdown()
	if got := r.calls(); len(got) != 1 || !got[0] {
		t.Fatalf("Apply calls = %v, want one with relaunch", got)
	}
}

func TestRestartToUpdateRefusedByAWindowInstallsNothing(t *testing.T) {
	stubRestart(t, func() error { t.Error("started another copy"); return nil })
	w := testWindows(t)
	quits := quitCounter(w)
	r := readyNow(t)
	w.updater = r.M
	a := openQuitWindow(t, w, true)

	if err := a.app.RestartToUpdate(); err != nil {
		t.Fatal(err)
	}
	a.app.CompleteFileClose(receive(t, a.requests, "close request"), false)
	none(t, quits, "quit")
	if got := r.calls(); len(got) != 0 {
		t.Fatalf("Apply calls = %v after a refusal", got)
	}
	// A later plain quit installs, without relaunching.
	waitFor(t, "quit round to end", func() bool {
		w.quitMu.Lock()
		defer w.quitMu.Unlock()
		return !w.quitting.running
	})
	w.ShouldQuit()
	a.app.CompleteFileClose(receive(t, a.requests, "quit request"), true)
	receive(t, quits, "quit")
	w.Shutdown()
	if got := r.calls(); len(got) != 1 || got[0] {
		t.Fatalf("Apply calls = %v, want one without relaunch", got)
	}
}

func TestFailedInstallFallsBackToTheNormalRestart(t *testing.T) {
	starts := make(chan bool, 2)
	stubRestart(t, func() error { starts <- true; return nil })
	w := testWindows(t)
	quits := quitCounter(w)
	r := readyNow(t)
	r.M.Apply = func(update.Ready, bool) error { return errors.New("no") }
	w.updater = r.M
	a := openQuitWindow(t, w, true)

	if err := a.app.RestartToUpdate(); err != nil {
		t.Fatal(err)
	}
	a.app.CompleteFileClose(receive(t, a.requests, "close request"), true)
	receive(t, quits, "quit")
	w.Shutdown()
	receive(t, starts, "restart")
}

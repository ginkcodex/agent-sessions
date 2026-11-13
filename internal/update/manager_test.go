package update

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recorder struct {
	mu   sync.Mutex
	list []Status
}

func (r *recorder) add(s Status) {
	r.mu.Lock()
	r.list = append(r.list, s)
	r.mu.Unlock()
}

func (r *recorder) states() []State {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []State
	for _, s := range r.list {
		out = append(out, s.State)
	}
	return out
}

func waitFor(t *testing.T, m *Manager, want State) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Status(); s.State == want {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("status = %+v, want state %s", m.Status(), want)
	return Status{}
}

type mgrEnv struct {
	M         *Manager
	Rec       *recorder
	Srv       *installerServer
	Body      string
	Auto      atomic.Bool
	Supported atomic.Bool
	Applied   []Ready
	Relaunch  []bool
}

func newMgr(t *testing.T, handler http.HandlerFunc) *mgrEnv {
	t.Helper()
	e := &mgrEnv{Body: "installer bytes", Rec: &recorder{}}
	e.Auto.Store(true)
	e.Supported.Store(true)
	e.Srv = serveInstaller(t, e.Body, handler)
	e.M = &Manager{
		Store:     &Store{Dir: filepath.Join(t.TempDir(), "updates"), HTTP: http.DefaultClient},
		Supported: e.Supported.Load,
		Auto:      e.Auto.Load,
		Apply: func(r Ready, relaunch bool) error {
			e.Applied = append(e.Applied, r)
			e.Relaunch = append(e.Relaunch, relaunch)
			return nil
		},
	}
	e.M.Subscribe(e.Rec.add)
	return e
}

func (e *mgrEnv) info(version string) Info {
	rel := releaseFor(e.Srv.URL, version, e.Body)
	return Info{Current: "1.5.5", Latest: version, URL: "https://example.test/tag/v" + version, Available: true, Release: &rel}
}

func TestOfferDownloadsAndBecomesReady(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	s := waitFor(t, e.M, StateReady)
	if s.Version != "1.6.0" || s.URL != "https://example.test/tag/v1.6.0" {
		t.Fatalf("status = %+v", s)
	}
	got := e.Rec.states()
	if len(got) != 2 || got[0] != StateDownloading || got[1] != StateReady {
		t.Fatalf("events = %v, want downloading then ready", got)
	}
	// Offering the same release again changes nothing and downloads nothing.
	e.M.Offer(e.info("1.6.0"))
	if e.Srv.Hits.Load() != 1 || e.M.Status() != s {
		t.Fatalf("second offer: %d downloads, status %+v", e.Srv.Hits.Load(), e.M.Status())
	}
}

func TestOfferWithoutAnUpdateIsIdleAndClearsDownloads(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)
	// The app was updated by other means, or the release was pulled so the
	// latest is the running version again.
	e.M.Offer(Info{Current: "1.5.5", Latest: "1.5.5", Available: false})
	if s := e.M.Status(); s.State != StateIdle {
		t.Fatalf("status = %+v", s)
	}
	if _, ok := e.M.Store.Ready("1.6.0"); ok {
		t.Fatal("the download stayed")
	}
}

func TestPulledReleaseIsDroppedAndNewerOneReplacesIt(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)
	e.M.Offer(e.info("1.6.1"))
	waitFor(t, e.M, StateReady)
	if _, ok := e.M.Store.Ready("1.6.0"); ok {
		t.Fatal("the older download stayed")
	}
	if s := e.M.Status(); s.Version != "1.6.1" {
		t.Fatalf("status = %+v", s)
	}
}

func TestUnsupportedInstallShowsThePage(t *testing.T) {
	e := newMgr(t, nil)
	e.Supported.Store(false)
	e.M.Offer(e.info("1.6.0"))
	if s := e.M.Status(); s.State != StateUnsupported || s.Version != "1.6.0" || s.URL == "" {
		t.Fatalf("status = %+v", s)
	}
	if e.Srv.Hits.Load() != 0 {
		t.Fatal("downloaded on an install that cannot update")
	}
	// No signed release (Release nil, from the API fallback): same.
	e = newMgr(t, nil)
	info := e.info("1.6.0")
	info.Release = nil
	e.M.Offer(info)
	if s := e.M.Status(); s.State != StateUnsupported {
		t.Fatalf("status = %+v", s)
	}
}

func TestSettingOffShowsThePageAndOnDownloads(t *testing.T) {
	e := newMgr(t, nil)
	e.Auto.Store(false)
	e.M.Offer(e.info("1.6.0"))
	if s := e.M.Status(); s.State != StateManual || s.Version != "1.6.0" {
		t.Fatalf("status = %+v", s)
	}
	if e.Srv.Hits.Load() != 0 {
		t.Fatal("downloaded with the setting off")
	}
	e.Auto.Store(true)
	e.M.SettingsChanged()
	waitFor(t, e.M, StateReady)
}

func TestSettingTurnedOffStopsADownload(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	e := newMgr(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		<-r.Context().Done()
	})
	e.M.Offer(e.info("1.6.0"))
	<-started
	if s := e.M.Status(); s.State != StateDownloading {
		t.Fatalf("status = %+v", s)
	}
	e.Auto.Store(false)
	e.M.SettingsChanged()
	if s := e.M.Status(); s.State != StateManual || s.Version != "1.6.0" {
		t.Fatalf("status = %+v", s)
	}
	if e.M.Store.Failures("1.6.0") != 0 {
		t.Fatal("stopping a download counted as a failure")
	}
	// Nothing flips it back to downloading or ready afterwards.
	time.Sleep(50 * time.Millisecond)
	if s := e.M.Status(); s.State != StateManual {
		t.Fatalf("status = %+v", s)
	}
}

func TestFailedDownloadRetriesThenGivesUp(t *testing.T) {
	e := newMgr(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) })
	for i := 1; i <= 3; i++ {
		e.M.Offer(e.info("1.6.0"))
		if i < 3 {
			waitForFailures(t, e, i)
			if s := e.M.Status(); s.State != StateIdle {
				t.Fatalf("after failure %d: %+v", i, s)
			}
		}
	}
	s := waitFor(t, e.M, StateManual)
	if s.Version != "1.6.0" || s.URL == "" {
		t.Fatalf("status = %+v", s)
	}
	hits := e.Srv.Hits.Load()
	e.M.Offer(e.info("1.6.0"))
	if e.Srv.Hits.Load() != hits || e.M.Status().State != StateManual {
		t.Fatal("kept trying after giving up")
	}
}

func waitForFailures(t *testing.T, e *mgrEnv, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if e.M.Store.Failures("1.6.0") >= n && e.M.Status().State != StateDownloading {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("failures = %d, want %d", e.M.Store.Failures("1.6.0"), n)
}

func TestFinishInstallsTheReadyVersion(t *testing.T) {
	e := newMgr(t, nil)
	if e.M.Finish(false) {
		t.Fatal("Finish claimed an install with nothing ready")
	}
	e = newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)
	if !e.M.Finish(false) || len(e.Applied) != 1 || e.Applied[0].Version != "1.6.0" || e.Relaunch[0] {
		t.Fatalf("plain quit: applied %v relaunch %v", e.Applied, e.Relaunch)
	}

	e = newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)
	if !e.M.Finish(true) || !e.Relaunch[0] {
		t.Fatalf("restart: relaunch %v", e.Relaunch)
	}
}

func TestFinishFailingToApplyLeavesTheAppToRestartItself(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Apply = func(Ready, bool) error { return os.ErrPermission }
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)
	if e.M.Finish(true) {
		t.Fatal("Finish claimed an install that failed")
	}
}

func TestFinishRefusesAChangedInstaller(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	r := waitFor(t, e.M, StateReady)
	ready, _ := e.M.Store.Ready(r.Version)
	// Same size, other bytes: Pending compares sizes only, so Finish hashes.
	if err := os.WriteFile(ready.Path(), []byte("installer byteX"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.M.Pending(); !ok {
		t.Fatal("same-size change made it not pending")
	}
	if e.M.Finish(false) || len(e.Applied) != 0 {
		t.Fatal("installed a changed installer")
	}
}

func TestFinishStopsADownloadAndInstallsNothing(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	e := newMgr(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		<-r.Context().Done()
	})
	e.M.Offer(e.info("1.6.0"))
	<-started
	done := make(chan bool)
	go func() { done <- e.M.Finish(false) }()
	select {
	case installed := <-done:
		if installed || len(e.Applied) != 0 {
			t.Fatal("installed a download that was not finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Finish waited for the download")
	}
	// After Finish the manager takes no more offers.
	e.M.Offer(e.info("1.6.1"))
	if s := e.M.Status(); s.State == StateDownloading && s.Version == "1.6.1" {
		t.Fatal("started a download after Finish")
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	e := newMgr(t, nil)
	var n atomic.Int32
	unsub := e.M.Subscribe(func(Status) { n.Add(1) })
	e.Supported.Store(false)
	e.M.Offer(e.info("1.6.0"))
	if n.Load() != 1 {
		t.Fatalf("subscriber saw %d events", n.Load())
	}
	e.M.Offer(e.info("1.6.0")) // no change, no event
	if n.Load() != 1 {
		t.Fatalf("an unchanged status was announced (%d)", n.Load())
	}
	unsub()
	e.M.Offer(Info{Available: false})
	if n.Load() != 1 {
		t.Fatalf("an unsubscribed callback was called (%d)", n.Load())
	}
}

func TestResumePicksUpAReadyDownloadFromAnEarlierRun(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)

	// A new launch: a fresh manager on the same folder.
	m := &Manager{Store: e.M.Store, Supported: e.Supported.Load, Apply: e.M.Apply}
	m.Resume("1.5.5")
	if s := m.Status(); s.State != StateReady || s.Version != "1.6.0" {
		t.Fatalf("status = %+v", s)
	}
	if !m.Finish(false) || len(e.Applied) != 1 {
		t.Fatal("the resumed download was not installed at quit")
	}
}

func TestResumeRemovesWhatTheRunningVersionHasPassed(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)

	for name, tc := range map[string]struct {
		current   string
		supported bool
	}{"already updated": {"1.6.0", true}, "newer build": {"1.7.0", true}} {
		m := &Manager{Store: e.M.Store, Supported: func() bool { return tc.supported }}
		m.Resume(tc.current)
		if m.Status().State != StateIdle {
			t.Errorf("%s: status = %+v", name, m.Status())
		}
		if _, ok := m.Store.Ready("1.6.0"); ok {
			t.Errorf("%s: the download stayed", name)
		}
		e.M.Store.Fetch(t.Context(), releaseFor(e.Srv.URL, "1.6.0", e.Body))
	}

	// A development build must never clear a developer's downloads.
	m := &Manager{Store: e.M.Store, Supported: e.Supported.Load}
	m.Resume("dev")
	if _, ok := m.Store.Ready("1.6.0"); !ok || m.Status().State != StateIdle {
		t.Fatal("a dev build touched the downloads")
	}
	// An install that cannot update does not install what it finds.
	m = &Manager{Store: e.M.Store, Supported: func() bool { return false }}
	m.Resume("1.5.5")
	if m.Status().State != StateIdle {
		t.Fatalf("status = %+v", m.Status())
	}
}

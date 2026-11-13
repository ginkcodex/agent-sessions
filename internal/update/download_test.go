package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type installerServer struct {
	URL  string
	Hits atomic.Int32
}

// serveInstaller serves body at /<file>. handler, if set, replaces the body.
func serveInstaller(t *testing.T, body string, handler http.HandlerFunc) *installerServer {
	t.Helper()
	is := &installerServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		is.Hits.Add(1)
		if handler != nil {
			handler(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	is.URL = srv.URL
	return is
}

func releaseFor(srvURL, version, body string) Release {
	file := "agent-sessions_" + version + "_macos_universal.dmg"
	return Release{Version: version, File: file, URL: srvURL + "/" + file, SHA256: hexSum(body)}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Dir: filepath.Join(t.TempDir(), "updates"), HTTP: http.DefaultClient}
}

func TestFetchDownloadsChecksAndMarksReady(t *testing.T) {
	const body = "installer bytes"
	srv := serveInstaller(t, body, nil)
	s := newStore(t)
	rel := releaseFor(srv.URL, "1.6.0", body)

	r, err := s.Fetch(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "1.6.0" || r.File != rel.File || r.SHA256 != rel.SHA256 || r.Size != int64(len(body)) {
		t.Fatalf("Ready = %+v", r)
	}
	if got, _ := os.ReadFile(r.Path()); string(got) != body {
		t.Fatalf("installer = %q", got)
	}
	if err := r.Verify(); err != nil {
		t.Fatal(err)
	}
	if again, ok := s.Ready("1.6.0"); !ok || again != r {
		t.Fatalf("Ready() = %+v, %v", again, ok)
	}
	if n, ok := s.Newest(); !ok || n.Version != "1.6.0" {
		t.Fatalf("Newest = %+v, %v", n, ok)
	}
	// Ready already: no second download.
	if _, err := s.Fetch(context.Background(), rel); err != nil || srv.Hits.Load() != 1 {
		t.Fatalf("second Fetch = %v after %d downloads", err, srv.Hits.Load())
	}
	// Corrupting the file after the fact is caught by Verify, and a size
	// change makes it not ready.
	if err := os.WriteFile(r.Path(), []byte("installer byteX"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r.Verify() == nil {
		t.Fatal("Verify accepted a changed installer")
	}
	if err := os.WriteFile(r.Path(), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Ready("1.6.0"); ok {
		t.Fatal("a resized installer is still ready")
	}
}

func TestFetchNeverMarksAWrongDownloadReady(t *testing.T) {
	for name, tc := range map[string]struct {
		handler http.HandlerFunc
	}{
		"altered bytes": {func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("tampered bytes")) }},
		"truncated": {func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("only a little"))
		}},
		"not found": {func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }},
	} {
		t.Run(name, func(t *testing.T) {
			srv := serveInstaller(t, "", tc.handler)
			s := newStore(t)
			rel := releaseFor(srv.URL, "1.6.0", "installer bytes")
			if _, err := s.Fetch(context.Background(), rel); err == nil {
				t.Fatal("Fetch succeeded")
			}
			if _, ok := s.Ready("1.6.0"); ok {
				t.Fatal("marked ready")
			}
			entries, _ := os.ReadDir(filepath.Join(s.Dir, "1.6.0"))
			for _, e := range entries {
				t.Errorf("left behind: %s", e.Name())
			}
			if s.Failures("1.6.0") != 1 {
				t.Fatalf("failures = %d, want 1", s.Failures("1.6.0"))
			}
		})
	}
}

func TestFetchReplacesAnEarlierBadDownload(t *testing.T) {
	const body = "installer bytes"
	srv := serveInstaller(t, body, nil)
	s := newStore(t)
	rel := releaseFor(srv.URL, "1.6.0", body)
	// A stale folder with a marker for other bytes, and a partial file.
	dir := filepath.Join(s.Dir, "1.6.0")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, rel.File), []byte("old"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, rel.File+".part"), []byte("half"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "ready.json"), []byte(`{"version":"1.6.0","file":"`+rel.File+`","sha256":"`+hexSum("old")+`","size":3}`), 0o600)

	r, err := s.Fetch(context.Background(), rel)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(r.Path()); string(got) != body {
		t.Fatalf("installer = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, rel.File+".part")); err == nil {
		t.Fatal("partial file kept")
	}
}

func TestFetchGivesUpAfterThreeFailures(t *testing.T) {
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) })
	s := newStore(t)
	rel := releaseFor(srv.URL, "1.6.0", "installer bytes")
	for i := 1; i <= 3; i++ {
		if s.GaveUp("1.6.0") {
			t.Fatalf("gave up after %d failures", i-1)
		}
		if _, err := s.Fetch(context.Background(), rel); err == nil || errors.Is(err, ErrGaveUp) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if !s.GaveUp("1.6.0") {
		t.Fatal("did not give up after 3 failures")
	}
	before := srv.Hits.Load()
	if _, err := s.Fetch(context.Background(), rel); !errors.Is(err, ErrGaveUp) {
		t.Fatalf("Fetch = %v, want ErrGaveUp", err)
	}
	if srv.Hits.Load() != before {
		t.Fatal("downloaded again after giving up")
	}
	// A newer version starts fresh.
	if s.GaveUp("1.6.1") {
		t.Fatal("failures carried over to another version")
	}
	// The count persists in a new Store on the same folder (a new launch).
	if !(&Store{Dir: s.Dir}).GaveUp("1.6.0") {
		t.Fatal("failure count was not saved")
	}
}

func TestFetchSuccessClearsFailures(t *testing.T) {
	const body = "installer bytes"
	var fail atomic.Bool
	fail.Store(true)
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "no", 500)
			return
		}
		_, _ = w.Write([]byte(body))
	})
	s := newStore(t)
	rel := releaseFor(srv.URL, "1.6.0", body)
	_, _ = s.Fetch(context.Background(), rel)
	_, _ = s.Fetch(context.Background(), rel)
	fail.Store(false)
	if _, err := s.Fetch(context.Background(), rel); err != nil {
		t.Fatal(err)
	}
	if s.Failures("1.6.0") != 0 {
		t.Fatalf("failures = %d after success", s.Failures("1.6.0"))
	}
}

func TestCancelledFetchIsNotAFailure(t *testing.T) {
	started := make(chan struct{})
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	})
	s := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err := s.Fetch(ctx, releaseFor(srv.URL, "1.6.0", "installer bytes"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch = %v, want context.Canceled", err)
	}
	if s.Failures("1.6.0") != 0 {
		t.Fatal("a cancelled download counted as a failure")
	}
}

func TestStalledDownloadIsAbandoned(t *testing.T) {
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	s := newStore(t)
	s.IdleTimeout = 150 * time.Millisecond
	start := time.Now()
	_, err := s.Fetch(context.Background(), releaseFor(srv.URL, "1.6.0", "installer bytes"))
	if err == nil || !strings.Contains(err.Error(), "no data") {
		t.Fatalf("Fetch = %v, want a stall error", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("took too long to notice the stall")
	}
	if s.Failures("1.6.0") != 1 {
		t.Fatal("a stalled download is a failure")
	}
}

func TestSlowButSteadyDownloadIsNotAbandoned(t *testing.T) {
	const body = "0123456789"
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		for i := range body {
			_, _ = w.Write([]byte{body[i]})
			w.(http.Flusher).Flush()
			time.Sleep(60 * time.Millisecond)
		}
	})
	s := newStore(t)
	s.IdleTimeout = 250 * time.Millisecond // shorter than the whole download
	if _, err := s.Fetch(context.Background(), releaseFor(srv.URL, "1.6.0", body)); err != nil {
		t.Fatal(err)
	}
}

func TestTwoProcessesDownloadOnce(t *testing.T) {
	const body = "installer bytes"
	srv := serveInstaller(t, "", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond) // long enough for the other to arrive
		_, _ = w.Write([]byte(body))
	})
	dir := filepath.Join(t.TempDir(), "updates")
	rel := releaseFor(srv.URL, "1.6.0", body)
	// Separate Stores share nothing but the folder, as two processes do.
	var wg sync.WaitGroup
	results := make([]Ready, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := &Store{Dir: dir, HTTP: http.DefaultClient}
			results[i], errs[i] = s.Fetch(context.Background(), rel)
		}()
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("Fetch %d: %v", i, errs[i])
		}
	}
	if results[0] != results[1] {
		t.Fatalf("results differ: %+v vs %+v", results[0], results[1])
	}
	if n := srv.Hits.Load(); n != 1 {
		t.Fatalf("installer downloaded %d times, want 1", n)
	}
}

func TestFetchWaitsForTheLockHolderAndHonoursCancel(t *testing.T) {
	s := newStore(t)
	held, err := s.lock(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	srv := serveInstaller(t, "installer bytes", nil)
	_, err = s.Fetch(ctx, releaseFor(srv.URL, "1.6.0", "installer bytes"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Fetch = %v, want the context's deadline", err)
	}
	if srv.Hits.Load() != 0 {
		t.Fatal("downloaded without the lock")
	}
	// Cleanup does not wait: it reports busy and removes nothing.
	if err := s.Reconcile(""); !errors.Is(err, ErrBusy) {
		t.Fatalf("Reconcile = %v, want ErrBusy", err)
	}
}

func TestFetchRefusesUnusableReleases(t *testing.T) {
	s := newStore(t)
	good := releaseFor("http://x", "1.6.0", "b")
	for name, mutate := range map[string]func(*Release){
		"path in version": func(r *Release) { r.Version = "../1.6.0" },
		"path in file":    func(r *Release) { r.File = "../evil" },
		"empty file":      func(r *Release) { r.File = "" },
		"no hash":         func(r *Release) { r.SHA256 = "" },
		"no url":          func(r *Release) { r.URL = "" },
	} {
		rel := good
		mutate(&rel)
		if _, err := s.Fetch(context.Background(), rel); err == nil {
			t.Errorf("%s: Fetch succeeded", name)
		}
	}
	if _, err := os.Stat(s.Dir); err == nil {
		t.Error("an unusable release created the folder")
	}
}

func TestReadyIgnoresAForgedMarker(t *testing.T) {
	s := newStore(t)
	dir := filepath.Join(s.Dir, "1.6.0")
	_ = os.MkdirAll(dir, 0o700)
	for name, marker := range map[string]string{
		"other version": `{"version":"9.9.9","file":"a.dmg","sha256":"` + hexSum("x") + `","size":1}`,
		"path in file":  `{"version":"1.6.0","file":"../a.dmg","sha256":"` + hexSum("x") + `","size":1}`,
		"bad hash":      `{"version":"1.6.0","file":"a.dmg","sha256":"zz","size":1}`,
		"not json":      `ready`,
	} {
		_ = os.WriteFile(filepath.Join(dir, "a.dmg"), []byte("x"), 0o600)
		_ = os.WriteFile(filepath.Join(dir, "ready.json"), []byte(marker), 0o600)
		if _, ok := s.Ready("1.6.0"); ok {
			t.Errorf("%s: marker accepted", name)
		}
	}
	if _, ok := s.Ready("../1.6.0"); ok {
		t.Error("a path was accepted as a version")
	}
}

func TestReconcileKeepsOnlyTheLatest(t *testing.T) {
	const body = "installer bytes"
	srv := serveInstaller(t, body, nil)
	s := newStore(t)
	for _, v := range []string{"1.5.0", "1.6.0", "1.7.0"} {
		if _, err := s.Fetch(context.Background(), releaseFor(srv.URL, v, body)); err != nil {
			t.Fatal(err)
		}
	}
	// A partial file in the kept folder, a log, and a failure count.
	_ = os.WriteFile(filepath.Join(s.Dir, "1.6.0", "x.dmg.part"), []byte("p"), 0o600)
	_ = os.WriteFile(filepath.Join(s.Dir, "apply.log"), []byte("log"), 0o600)
	if err := s.writeFailures(map[string]int{"1.5.0": 2, "1.6.0": 1}); err != nil {
		t.Fatal(err)
	}

	if err := s.Reconcile("1.6.0"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"1.5.0", "1.7.0"} {
		if _, err := os.Stat(filepath.Join(s.Dir, v)); err == nil {
			t.Errorf("%s was kept", v)
		}
	}
	if _, ok := s.Ready("1.6.0"); !ok {
		t.Error("the latest was removed")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "1.6.0", "x.dmg.part")); err == nil {
		t.Error("partial file kept")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, "apply.log")); err != nil {
		t.Error("the log was removed")
	}
	if s.Failures("1.5.0") != 0 || s.Failures("1.6.0") != 1 {
		t.Errorf("failures = %v", s.readFailures())
	}

	// The latest is now an older, pulled version: ready 1.6.0 goes.
	if err := s.Reconcile("1.5.5"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Ready("1.6.0"); ok {
		t.Error("a pulled version stayed ready")
	}
	if err := s.Reconcile(""); err != nil {
		t.Fatal(err)
	}
	if err := (&Store{Dir: filepath.Join(t.TempDir(), "none")}).Reconcile("1.0.0"); err != nil {
		t.Fatalf("Reconcile on a missing folder = %v", err)
	}
}

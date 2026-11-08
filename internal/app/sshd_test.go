package app

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/remote"
	"github.com/ginkcode/agent-sessions/internal/remote/sshtest"
	"github.com/ginkcode/agent-sessions/internal/version"
)

// These drive the real connection loop (probe, deploy, serve) against a
// Docker sshd; they are skipped unless AGENT_SESSIONS_SSH_DOCKER=1. Run them
// with `make test-ssh`.

// phaseLog records every ConnectionState the connection emits.
type phaseLog struct {
	mu     sync.Mutex
	states []ConnectionState
}

func (l *phaseLog) Emit(name string, payload any) {
	if name != connectionEvent {
		return
	}
	l.mu.Lock()
	l.states = append(l.states, payload.(ConnectionState))
	l.mu.Unlock()
}

// waitFor returns the first state from index from on that matches ok.
func (l *phaseLog) waitFor(t *testing.T, from int, timeout time.Duration, ok func(ConnectionState) bool) (int, ConnectionState) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		for i := from; i < len(l.states); i++ {
			if ok(l.states[i]) {
				st := l.states[i]
				l.mu.Unlock()
				return i, st
			}
		}
		l.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	t.Fatalf("no matching state within %v; saw %+v", timeout, l.states)
	return 0, ConnectionState{}
}

func sshdConnection(t *testing.T, h *sshtest.Host) (*connection, *phaseLog) {
	t.Helper()
	prev := reconnectBackoff
	reconnectBackoff = []time.Duration{200 * time.Millisecond}
	t.Cleanup(func() { reconnectBackoff = prev })
	return sshdConnectionProduction(t, h)
}

// sshdConnectionProduction keeps the production backoff and timeouts, for
// tests that measure how long recovery takes.
func sshdConnectionProduction(t *testing.T, h *sshtest.Host) (*connection, *phaseLog) {
	t.Helper()
	log := &phaseLog{}
	c := newConnection(log)
	t.Cleanup(c.Shutdown)
	c.opts.ConfigFile = h.ConfigFile
	c.opts.ControlPath = h.ControlPath
	c.stopMaster = stopControlMaster
	return c, log
}

func phaseIs(phase string) func(ConnectionState) bool {
	return func(st ConnectionState) bool { return st.Phase == phase }
}

func TestSSHD_ConnectionReconnectsAfterDrop(t *testing.T) {
	h := sshtest.Start(t)
	t.Setenv("AGENT_SESSIONS_REMOTE_SERVERS_DIR", sshtest.ServerBundleDir(t, version.Current()))
	c, log := sshdConnection(t, h)
	alias := h.Alias(sshtest.UserZsh)

	if err := c.Connect(context.Background(), alias); err != nil {
		t.Fatal(err)
	}
	i, st := log.waitFor(t, 0, 60*time.Second, func(st ConnectionState) bool {
		return st.Phase == ConnConnected || st.Phase == ConnDisconnected
	})
	if st.Phase != ConnConnected || st.Host != alias || st.AppVersion != version.Current() {
		t.Fatalf("first connect: %+v", st)
	}
	backend := c.backend(nil)
	if backend == nil {
		t.Fatal("connected without a remote backend")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := backend.Scan(ctx); err != nil {
		t.Fatalf("remote scan: %v", err)
	}

	// Kill the remote server: the loop reports the drop, stays on the host,
	// and reconnects to a new session on its own.
	h.Exec(t, `pkill -f 'agent-sessions-cli serve'`)
	i, st = log.waitFor(t, i+1, 15*time.Second, phaseIs(ConnDisconnected))
	if st.Host != alias || st.Error != "connection lost" {
		t.Fatalf("drop state: %+v", st)
	}
	i, _ = log.waitFor(t, i+1, 5*time.Second, phaseIs(ConnReconnecting))
	_, st = log.waitFor(t, i+1, 60*time.Second, phaseIs(ConnConnected))
	if st.Host != alias {
		t.Fatalf("reconnected to %q", st.Host)
	}
	again := c.backend(nil)
	if again == nil || again == backend {
		t.Fatal("reconnect did not install a new session")
	}
	if _, err := again.AgentCounts(ctx, engine.FilterOpts{}); err != nil {
		t.Fatalf("call after reconnect: %v", err)
	}
	masterCheck := func() error {
		return exec.Command("ssh", "-F", h.ConfigFile, "-o", "ControlPath="+h.ControlPath, "-O", "check", "--", alias).Run()
	}
	if err := masterCheck(); err != nil {
		t.Fatalf("no ssh master while connected: %v", err)
	}

	c.Disconnect()
	if st := c.snapshot(); st.Phase != ConnLocal || st.Host != "" {
		t.Fatalf("after disconnect: %+v", st)
	}

	// Disconnect leaves nothing running: no server on the host and no ssh
	// master on this side.
	deadline := time.Now().Add(10 * time.Second)
	for {
		out := strings.TrimSpace(h.Exec(t, `pgrep -f '[a]gent-sessions-cli.* serve --stdio' | wc -l`))
		if out == "0" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s remote servers still running after disconnect", out)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if masterCheck() == nil {
		t.Fatal("ssh master still running after disconnect")
	}
}

func TestSSHD_VersionMismatchStopsRetrying(t *testing.T) {
	h := sshtest.Start(t)
	// A server stamped with another version deploys, then fails the version
	// check. Redeploying the same bundle cannot help, so the loop must stop.
	t.Setenv("AGENT_SESSIONS_REMOTE_SERVERS_DIR", sshtest.ServerBundleDir(t, "0.0.0-mismatch"))
	c, log := sshdConnection(t, h)
	alias := h.Alias(sshtest.UserSh)

	var mu sync.Mutex
	dials := 0
	c.dial = func(ctx context.Context, alias string, opts remote.SSHOptions, env map[string]string, emitter engine.Emitter) (*remote.Session, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return remote.StartSession(ctx, alias, opts, env, emitter)
	}
	if err := c.Connect(context.Background(), alias); err != nil {
		t.Fatal(err)
	}
	_, st := log.waitFor(t, 0, 60*time.Second, phaseIs(ConnDisconnected))
	if !strings.Contains(st.Error, "server version mismatch") {
		t.Fatalf("state: %+v", st)
	}
	time.Sleep(time.Second) // several backoff periods
	mu.Lock()
	defer mu.Unlock()
	if dials != 1 {
		t.Fatalf("dial attempts = %d, want 1", dials)
	}
	if got := c.snapshot(); got.Phase != ConnDisconnected {
		t.Fatalf("phase after wait = %q", got.Phase)
	}
}

// silentDrop measures recovery from a fault that leaves the link open but
// silent, at production timings: how long until the app reports the drop,
// and how long after the fault is undone until it is connected again. The
// fault lasts at least outage, and at least a few seconds past detection
// so that some retries fail first, as on a real outage.
type silentDrop struct {
	name   string
	fault  func(t *testing.T, h *sshtest.Host)
	undo   func(t *testing.T, h *sshtest.Host)
	outage time.Duration
	// Upper bounds; past them the test fails with what it saw.
	detectWithin, recoverWithin time.Duration
}

func (s silentDrop) run(t *testing.T) {
	h := sshtest.Start(t)
	t.Setenv("AGENT_SESSIONS_REMOTE_SERVERS_DIR", sshtest.ServerBundleDir(t, version.Current()))
	c, log := sshdConnectionProduction(t, h)
	alias := h.Alias(sshtest.UserSh)

	if err := c.Connect(context.Background(), alias); err != nil {
		t.Fatal(err)
	}
	i, st := log.waitFor(t, 0, 60*time.Second, func(st ConnectionState) bool {
		return st.Phase == ConnConnected || st.Phase == ConnDisconnected
	})
	if st.Phase != ConnConnected {
		t.Fatalf("first connect: %+v", st)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := c.backend(nil).AgentCounts(ctx, engine.FilterOpts{}); err != nil {
		t.Fatalf("call before the fault: %v", err)
	}

	faultAt := time.Now()
	s.fault(t, h)
	undone := false
	defer func() {
		if !undone {
			s.undo(t, h)
		}
	}()
	i, st = log.waitFor(t, i+1, s.detectWithin, phaseIs(ConnDisconnected))
	detect := time.Since(faultAt)
	t.Logf("%s: drop reported after %s (%q)", s.name, detect.Round(100*time.Millisecond), st.Error)

	undoAt := faultAt.Add(s.outage)
	if earliest := time.Now().Add(5 * time.Second); undoAt.Before(earliest) {
		undoAt = earliest
	}
	time.Sleep(time.Until(undoAt))
	undoAt = time.Now()
	s.undo(t, h)
	undone = true
	_, st = log.waitFor(t, i+1, s.recoverWithin, phaseIs(ConnConnected))
	recovered := time.Since(undoAt)
	if st.Host != alias {
		t.Fatalf("reconnected to %q", st.Host)
	}
	if _, err := c.backend(nil).AgentCounts(ctx, engine.FilterOpts{}); err != nil {
		t.Fatalf("call after reconnect: %v", err)
	}
	t.Logf("RESULT %s: detect=%s recover=%s (outage %s, total %s)", s.name,
		detect.Round(100*time.Millisecond), recovered.Round(100*time.Millisecond),
		undoAt.Sub(faultAt).Round(100*time.Millisecond), time.Since(faultAt).Round(100*time.Millisecond))
}

// The network to the host is cut: nothing gets through, nothing fails.
func TestSSHD_RecoversFromNetworkCut(t *testing.T) {
	silentDrop{
		name:          "network cut",
		fault:         func(t *testing.T, h *sshtest.Host) { h.CutNetwork(t) },
		undo:          func(t *testing.T, h *sshtest.Host) { h.RestoreNetwork(t) },
		outage:        60 * time.Second,
		detectWithin:  3 * time.Minute,
		recoverWithin: 4 * time.Minute,
	}.run(t)
}

// The host freezes: its kernel accepts packets but sshd and the server
// never answer.
func TestSSHD_RecoversFromFrozenHost(t *testing.T) {
	silentDrop{
		name:          "frozen host",
		fault:         func(t *testing.T, h *sshtest.Host) { h.Pause(t) },
		undo:          func(t *testing.T, h *sshtest.Host) { h.Unpause(t) },
		outage:        60 * time.Second,
		detectWithin:  3 * time.Minute,
		recoverWithin: 4 * time.Minute,
	}.run(t)
}

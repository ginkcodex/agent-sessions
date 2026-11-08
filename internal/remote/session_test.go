package remote

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/rpc"
)

func TestDialSession(t *testing.T) {
	bin := fakeSSHPath(t, "serve", nil)
	obs := engine.NewStartupObservation()
	ctx, cancel := context.WithTimeout(engine.WithStartupObservation(context.Background(), obs), 10*time.Second)
	defer cancel()

	s, err := DialSession(ctx, "box", SSHOptions{Binary: bin, ControlMaster: "no"}, "/opt/agent-sessions-cli", nil, nil)
	if err != nil {
		t.Fatalf("DialSession: %v", err)
	}
	defer s.Close()

	seen := map[string]bool{}
	for _, sample := range obs.Samples() {
		seen[sample.Phase] = sample.Success
	}
	if !seen["remote_preface"] || !seen["remote_initialize"] {
		t.Fatalf("missing successful handshake observations: %+v", obs.Samples())
	}
	if s.Client() == nil || s.InitializeResult() == nil {
		t.Fatal("handshake did not complete")
	}
	if s.InitializeResult().ProtocolVersion != 1 {
		t.Fatalf("protocol = %d", s.InitializeResult().ProtocolVersion)
	}
	if !s.InitializeResult().Capabilities.Search {
		t.Fatal("expected search capability")
	}
	// The fake writes stderr before serving, but stderr is copied apart from
	// the stdout handshake, so it can arrive after Initialize returns.
	for deadline := time.Now().Add(5 * time.Second); !strings.Contains(s.Stderr(), "server ready"); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("stderr = %q", s.Stderr())
		}
	}

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session did not exit after Close")
	}
}

func TestDialSession_RetainedStderrDoesNotDelaySilence(t *testing.T) {
	if !SupportsSSHAskpass() {
		t.Skip("Unix proxy child fixture")
	}
	bin := fakeSSHPath(t, "serve", map[string]string{"AGENT_SESSIONS_FAKE_SILENT": "1", "AGENT_SESSIONS_FAKE_RETAIN_STDERR": "1"})
	prevProbe, prevDeadline := livenessProbeAfter, livenessDeadline
	livenessProbeAfter, livenessDeadline = 30*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { livenessProbeAfter, livenessDeadline = prevProbe, prevDeadline })
	s, err := DialSession(t.Context(), "box", SSHOptions{Binary: bin, ControlMaster: "no"}, "/opt/agent-sessions-cli", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	select {
	case <-s.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("retained stderr delayed process completion")
	}
	if !errors.Is(s.Err(), rpc.ErrLinkSilent) {
		t.Fatalf("reason = %v", s.Err())
	}
}

func TestDialSession_SilentServerEndsTransport(t *testing.T) {
	bin := fakeSSHPath(t, "serve", map[string]string{"AGENT_SESSIONS_FAKE_SILENT": "1"})
	prevProbe, prevDeadline := livenessProbeAfter, livenessDeadline
	livenessProbeAfter, livenessDeadline = 30*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { livenessProbeAfter, livenessDeadline = prevProbe, prevDeadline })
	s, err := DialSession(t.Context(), "box", SSHOptions{Binary: bin, ControlMaster: "no"}, "/opt/agent-sessions-cli", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("silent transport still running")
	}
	if !errors.Is(s.Client().Err(), rpc.ErrLinkSilent) {
		t.Fatalf("reason = %v", s.Client().Err())
	}
}

func TestDialSession_RejectsBadAlias(t *testing.T) {
	for _, alias := range []string{"-oProxyCommand=x", "wsl:-d", "wsl:bad name"} {
		_, err := DialSession(context.Background(), alias, SSHOptions{}, "/bin/true", nil, nil)
		if !errors.Is(err, ErrInvalidHostAlias) {
			t.Errorf("DialSession(%q) = %v, want ErrInvalidHostAlias", alias, err)
		}
	}
}

// The session heartbeats from before initialize, so a server with an idle
// timeout stays up for as long as the session does.
func TestHeartbeatKeepsIdleServerAlive(t *testing.T) {
	sInR, sInW := io.Pipe()
	defer sInW.Close()
	sOutR, sOutW := io.Pipe()
	srv := rpc.NewServer(nil, sInR, sOutW)
	srv.SetIdleTimeout(200 * time.Millisecond)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(context.Background()) }()

	client := rpc.NewClient(sOutR, sInW)
	ctx, cancel := context.WithCancel(context.Background())
	go heartbeat(ctx, client, 40*time.Millisecond)

	select {
	case err := <-done:
		cancel()
		t.Fatalf("server exited while heartbeating: %v", err)
	case <-time.After(700 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, rpc.ErrIdleTimeout) {
			t.Fatalf("Serve = %v, want ErrIdleTimeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server kept running after heartbeats stopped")
	}
}

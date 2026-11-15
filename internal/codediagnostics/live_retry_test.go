package codediagnostics

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func liveRetryFixture(t *testing.T, cap int) (*sessionEnv, *LiveDocuments, *Manager, Snapshot, AuthorizedRoot, ServerConfig, <-chan map[string]any) {
	t.Helper()
	e := newEnv(t, SessionOptions{MaxRestarts: cap})
	root, tools, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(root.Project().Root, "a.go"), []byte("disk"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := ServerConfig{ProviderID: "gopls", Executable: tools[0].Executable, Args: []string{"--fake"}, AdapterPolicy: "fake-only"}
	regs := []Registration{liveRegistration(ProviderState{ProviderID: "gopls", Availability: "available", Phase: "idle", Freshness: "uncertain", Coverage: Coverage{Kind: "openedDocuments", Roots: []string{""}, Partial: true, ExternalChanges: "unwatched"}}, cfg, func(file string) (string, bool) { return "go", file == "a.go" })}
	m, err := NewManager("retry-owner", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
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
	s, err = m.Approve(s.Context, s.Proposal.Fingerprint, "session")
	if err != nil {
		t.Fatal(err)
	}
	steps := make(chan map[string]any, 16)
	e.script = func(f *fakeSrv) {
		for {
			msg, ok := f.read()
			if !ok {
				return
			}
			switch msg["method"] {
			case "initialize":
				f.send(`{"jsonrpc":"2.0","id":1,"result":` + defaultInit + `}`)
			case "textDocument/didOpen":
				steps <- msg
			case "shutdown":
				f.send(`{"jsonrpc":"2.0","id":2,"result":null}`)
			case "exit":
				_ = f.Terminate()
				return
			}
		}
	}
	if _, err := m.OpenDocument(context.Background(), s.Context, "a.go", "a", 1, "draft", ""); err != nil {
		t.Fatal(err)
	}
	<-steps
	return e, d, m, s, root, cfg, steps
}

func TestLiveAutomaticRestartBackoffCapAndManualRetry(t *testing.T) {
	e, d, m, s, root, cfg, steps := liveRetryFixture(t, 2)
	old := d.Sessions.Info(root, cfg).Generation
	for attempt := 0; attempt < 2; attempt++ {
		server := e.next()
		if attempt == 0 {
			_, _ = io.WriteString(server.toTr, "Content-Length: 8388609\r\n\r\n")
		} else {
			_ = server.Terminate()
		}
		eventually(t, "crash", func() bool { return d.Sessions.Info(root, cfg).Phase == phaseFailed })
		d.Refresh()
		if e.launches() != attempt+1 {
			t.Fatal("retry skipped backoff")
		}
		e.clock.Advance(restartBackoff << attempt)
		d.Refresh()
		eventually(t, "automatic recovery", func() bool { return d.Sessions.Info(root, cfg).Phase == phaseInitialized })
		step := <-steps
		doc := step["params"].(map[string]any)["textDocument"].(map[string]any)
		if doc["text"] != "draft" || doc["version"] != float64(1) {
			t.Fatalf("replay: %+v", doc)
		}
		if d.Sessions.CurrentGeneration(root, cfg, old) {
			t.Fatal("old generation current")
		}
	}
	_ = e.next().Terminate()
	eventually(t, "cap crash", func() bool { return d.Sessions.Info(root, cfg).Phase == phaseFailed })
	d.Refresh()
	e.clock.Advance(restartBackoff << 2)
	d.Refresh()
	if e.launches() != 3 {
		t.Fatal("cap ignored")
	}
	if live, err := m.RetryLive(s.Context, "gopls"); !live || err != nil {
		t.Fatalf("manual retry: %v %v", live, err)
	}
	<-steps
	if e.launches() != 4 || d.Sessions.Info(root, cfg).Restarts != 1 {
		t.Fatal("manual retry did not reset bounded series")
	}
	v := int64(1)
	current := d.Sessions.Info(root, cfg).Generation
	if err := m.PublishPathDiagnostics(root.Project(), "gopls", current, "a.go", &v, nil); err != nil {
		t.Fatal("recovered publication", err)
	}
}

func TestLiveRetryHonorsLifecycleAndTrust(t *testing.T) {
	for _, action := range []string{"close document", "detach", "revoke", "manager close", "trust change"} {
		t.Run(action, func(t *testing.T) {
			e, d, m, s, root, cfg, _ := liveRetryFixture(t, 2)
			_ = e.next().Terminate()
			eventually(t, "crash", func() bool { return d.Sessions.Info(root, cfg).Phase == phaseFailed })
			d.Refresh()
			switch action {
			case "close document":
				_, _ = m.CloseDocument(context.Background(), s.Context, "a")
			case "detach":
				_ = m.Detach(s.Context)
			case "revoke":
				_, _ = m.Revoke(s.Context)
			case "manager close":
				m.Close()
			case "trust change":
				_ = e.trust.Revoke(root)
			}
			e.clock.Advance(restartBackoff)
			d.Refresh()
			m.workers.Wait()
			if e.launches() != 1 || d.Sessions.Info(root, cfg).Restarts != 0 {
				t.Fatal("lifecycle/trust bypassed")
			}
		})
	}
}

func TestSessionRetryPreflightDoesNotConsumeAttempt(t *testing.T) {
	e := newEnv(t, SessionOptions{MaxRestarts: 1})
	root, cfg := e.project()
	if _, err := e.mgr.Attach(context.Background(), root, cfg); err != nil {
		t.Fatal(err)
	}
	_ = e.next().Terminate()
	eventually(t, "crash", func() bool { return e.mgr.Info(root, cfg).Phase == phaseFailed })
	if err := e.trust.Revoke(root); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Retry(context.Background(), root, cfg); err == nil {
		t.Fatal("retry without approval")
	}
	e.approve(root, cfg)
	if info, err := e.mgr.Retry(context.Background(), root, cfg); err != nil || info.Restarts != 1 {
		t.Fatalf("preflight consumed attempt: %+v %v", info, err)
	}
}

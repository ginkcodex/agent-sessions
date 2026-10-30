package codediagnostics

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionInitializeServesConfigurationBeforeResponse(t *testing.T) {
	var notifications atomic.Int32
	e := newEnv(t, SessionOptions{Notifications: func(SessionNotification) { notifications.Add(1) }})
	replies := make(chan map[string]any, 1)
	e.script = func(f *fakeSrv) {
		init, ok := f.read()
		if !ok {
			return
		}
		// A startup notification must not re-enter the document manager while
		// Attach is waiting. The subsequent request is a delivery barrier.
		f.send(`{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"file:///early.rs","diagnostics":[]}}`)
		reply := f.request(`"config"`, "workspace/configuration", `{"items":[{"section":"rust-analyzer"}]}`)
		replies <- reply
		id, _ := json.Marshal(init["id"])
		f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + defaultInit + `}`)
		serveInit(defaultInit)(f)
	}
	root, cfg := e.project()
	cfg.Settings = json.RawMessage(`{"rust-analyzer":{"cargo":{"metadataExtraArgs":["--locked"],"extraArgs":["--locked"]}}}`)
	e.approve(root, cfg)
	const count = 4
	var wg sync.WaitGroup
	infos, errs := make([]SessionInfo, count), make([]error, count)
	for i := range infos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			infos[i], errs[i] = e.mgr.Attach(ctx, root, cfg)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil || infos[i].Phase != phaseInitialized || infos[i].Generation != infos[0].Generation {
			t.Fatalf("attach %d: %+v %v", i, infos[i], err)
		}
	}
	if e.launches() != 1 || notifications.Load() != 0 {
		t.Fatalf("launches=%d premature notifications=%d", e.launches(), notifications.Load())
	}
	reply := <-replies
	want, _ := boundedConfiguration(root, json.RawMessage(`{"items":[{"section":"rust-analyzer"}]}`), cfg.Settings)
	if !reflect.DeepEqual(reply["result"], want) {
		t.Fatalf("configuration reply: %v, want %v", reply, want)
	}
}

func TestSessionInitializeRequestPolicy(t *testing.T) {
	e := newEnv(t, SessionOptions{})
	type request struct {
		method, params string
		code           float64
	}
	requests := []request{
		{"workspace/configuration", `{"items":[]}`, -32602},
		{"workspace/configuration", `{"items":[{"scopeUri":"file:///outside","section":"rust-analyzer"}]}`, -32602},
		{"workspace/applyEdit", `{"edit":{}}`, -32601},
		{"client/registerCapability", `{"registrations":[{"method":"workspace/didChangeWatchedFiles"}]}`, -32601},
		{"unknown/startup", `{}`, -32601},
	}
	replies := make(chan map[string]any, len(requests))
	e.script = func(f *fakeSrv) {
		init, ok := f.read()
		if !ok {
			return
		}
		for _, req := range requests {
			replies <- f.request(`"request"`, req.method, req.params)
		}
		id, _ := json.Marshal(init["id"])
		f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + defaultInit + `}`)
		serveInit(defaultInit)(f)
	}
	root, cfg := e.project()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := e.mgr.Attach(ctx, root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, req := range requests {
		reply := <-replies
		got, ok := reply["error"].(map[string]any)
		if !ok || got["code"] != req.code {
			t.Fatalf("%s: %v", req.method, reply)
		}
	}
	if info.RejectedWatchers != 1 || info.ExternalChanges != CoverageExternalChanges {
		t.Fatalf("request policy changed: %+v", info)
	}
}

func TestDocumentOpenStartupFailureDoesNotRegisterLifecycle(t *testing.T) {
	e := newEnv(t, SessionOptions{})
	root, cfg := e.project()
	writeTree(t, root.Project().Root, map[string]string{"source.go": "package p\n"})
	cfg.ProviderID = goplsProviderID
	regs := []Registration{liveRegistration(ProviderState{
		ProviderID: goplsProviderID, Availability: "available", Phase: "idle", Freshness: "uncertain",
		Coverage: Coverage{Kind: "openedDocuments", Roots: []string{""}, ExternalChanges: CoverageExternalChanges},
	}, cfg, func(path string) (string, bool) { return "go", strings.HasSuffix(path, ".go") })}
	manager, err := NewManager("startup-failure", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	owner := NewLiveDocuments(e.trust, manager, e.mgr.options)
	t.Cleanup(func() { manager.Close(); owner.Close() })
	if err := owner.Register(root, regs); err != nil {
		t.Fatal(err)
	}
	manager.SetDocumentProvider(owner)
	snapshot, err := manager.Attach(root, 1, regs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Approve(snapshot.Context, snapshot.Proposal.Fingerprint, "session"); err != nil {
		t.Fatal(err)
	}
	e.script = func(srv *fakeSrv) {
		if _, ok := srv.read(); ok {
			srv.request(`"config"`, "workspace/configuration", `{"items":[{"section":"x"}]}`)
			_ = srv.Terminate()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := manager.OpenDocument(ctx, snapshot.Context, "source.go", "failed", 1, "package p\n", ""); err == nil {
		t.Fatal("broken initialize acknowledged open")
	}
	manager.mu.Lock()
	a := manager.attachments[snapshot.Context.AttachmentID]
	docs, closedDocs := len(a.docs), len(a.closedDocs)
	manager.mu.Unlock()
	if docs != 0 || closedDocs != 0 {
		t.Fatalf("failed startup registered lifecycle: docs=%d closed=%d", docs, closedDocs)
	}
	first := e.next()
	if methods := first.seen(); slices.Contains(methods, "textDocument/didOpen") {
		t.Fatalf("failed startup sent didOpen: %v", methods)
	}
	e.script = serveInit(defaultInit)
	project, _ := owner.project(root)
	if _, err := owner.Sessions.Retry(ctx, root, project.servers[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.OpenDocument(ctx, snapshot.Context, "source.go", "fresh", 1, "package p\n", ""); err != nil {
		t.Fatal("fresh lifecycle after explicit retry:", err)
	}
}

func TestSessionInitializeRequestFailureCleanup(t *testing.T) {
	for _, action := range []string{"timeout", "revoke", "transport close"} {
		t.Run(action, func(t *testing.T) {
			e := newEnv(t, SessionOptions{})
			replied := make(chan struct{})
			e.script = func(f *fakeSrv) {
				if _, ok := f.read(); !ok {
					return
				}
				f.request(`"config"`, "workspace/configuration", `{"items":[{"section":"rust-analyzer"}]}`)
				close(replied)
				// Deliberately never answer initialize.
			}
			root, cfg := e.project()
			done := make(chan error, 1)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, err := e.mgr.Attach(ctx, root, cfg)
				done <- err
			}()
			srv := e.next()
			select {
			case <-replied:
			case <-time.After(5 * time.Second):
				t.Fatal("startup configuration was not answered")
			}
			switch action {
			case "timeout":
				e.clock.Advance(initTimeout)
			case "revoke":
				if err := e.trust.Revoke(root); err != nil {
					t.Fatal(err)
				}
			case "transport close":
				_ = srv.Terminate()
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("failed startup was acknowledged")
				}
				if action == "timeout" {
					code(t, err, "timeout")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("startup failure did not unblock attach")
			}
			eventually(t, "startup process cleanup", func() bool { return closed(srv.terminated) })
			e.mgr.Close()
			// Close joins session/process work; deadline waiters separately observe
			// their canceled contexts before decrementing the accounting counter.
			eventually(t, "startup timer cleanup", func() bool { return e.mgr.OwnedResources() == (OwnedResources{}) })
			if e.launches() != 1 {
				t.Fatalf("startup retried: launches=%d", e.launches())
			}
		})
	}
}

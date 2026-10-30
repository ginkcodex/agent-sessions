package codediagnostics

// Synthetic protocol tests only: no Rust server, Cargo or toolchain is executed.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

type rustOwnerEnv struct {
	*bridgeEnv
	owner   *LiveDocuments
	context Context
	drift   atomic.Bool
}

func newRustOwnerEnv(t *testing.T) *rustOwnerEnv {
	t.Helper()
	e := newEnv(t, SessionOptions{})
	b := &rustOwnerEnv{bridgeEnv: &bridgeEnv{sessionEnv: e}}
	b.root, b.cfg = e.project()
	b.cfg.ProviderID = rustAnalyzerProviderID
	b.cfg.DeclareDiagnostics = true
	b.cfg.ValidateStart = func() error {
		if b.drift.Load() {
			return failure("approvalChanged", "Synthetic Rust inputs changed")
		}
		return nil
	}
	writeTree(t, b.root.Project().Root, map[string]string{"a.rs": "disk is not the draft", "b.rs": "", "Cargo.toml": ""})
	m, err := NewManager("rust-owner", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	b.owner = NewLiveDocuments(e.trust, m, e.mgr.options)
	t.Cleanup(func() { m.Close(); b.owner.Close() })
	regs := []Registration{liveRegistration(ProviderState{
		ProviderID: rustAnalyzerProviderID, Availability: "available", Phase: "idle", Freshness: "uncertain",
		Coverage: Coverage{Kind: "openedDocuments", Roots: []string{""}, ExternalChanges: CoverageExternalChanges},
	}, b.cfg, func(file string) (string, bool) { return "rust", strings.HasSuffix(file, ".rs") })}
	if err := b.owner.Register(b.root, regs); err != nil {
		t.Fatal(err)
	}
	m.SetDocumentProvider(b.owner)
	s, err := m.Attach(b.root, 1, regs)
	if err != nil {
		t.Fatal(err)
	}
	s, err = m.Approve(s.Context, s.Proposal.Fingerprint, "session")
	if err != nil {
		t.Fatal(err)
	}
	b.context = s.Context
	b.h = &managerHarness{Manager: m, snap: s}
	return b
}

func (b *rustOwnerEnv) open(t *testing.T, id string) string {
	t.Helper()
	if _, err := b.h.OpenDocument(context.Background(), b.context, "a.rs", id, 1, "fn main() { let s = \"😀\"; }\r\n", ""); err != nil {
		t.Fatal(err)
	}
	generation, _, err := b.owner.Session(b.root, "a.rs")
	if err != nil {
		t.Fatal(err)
	}
	return generation
}

func (b *rustOwnerEnv) notification(uri, generation, version, diags string) SessionNotification {
	params := map[string]any{"uri": uri, "diagnostics": json.RawMessage("[" + diags + "]")}
	if version != "" {
		params["version"] = json.RawMessage(version)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return SessionNotification{Project: b.root.Project(), ProviderID: rustAnalyzerProviderID, Generation: generation, Method: "textDocument/publishDiagnostics", Params: raw}
}

func (b *rustOwnerEnv) publish(path, generation, version, diags string) {
	b.owner.notification(b.notification(uriOf(b.root, path), generation, version, diags))
}

func (b *rustOwnerEnv) snapshot(t *testing.T) Snapshot {
	t.Helper()
	s, err := b.h.GetSnapshot(b.context)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const rustFinding = `{"range":{"start":{"line":0,"character":21},"end":{"line":0,"character":23}},"severity":1,"source":"rustc","code":"E0308","message":"type mismatch","data":{"ignored":"server-only"},"relatedInformation":[{"location":{"uri":"file:///outside.rs"},"message":"ignored"}]}`

func TestRustLiveOwnerDiagnosticsFreshnessAndNormalization(t *testing.T) {
	b := newRustOwnerEnv(t)
	generation := b.open(t, "draft")
	b.srv = b.next()
	// Session initialization and didOpen alone do not certify a clean document.
	if s := b.snapshot(t); len(s.ResultSets) != 0 || s.Providers[0].Freshness != "uncertain" {
		t.Fatalf("premature freshness: %+v", s)
	}
	// Exercise the actual session -> live-owner switch, not just a bridge seam.
	n := b.notification(uriOf(b.root, "a.rs"), generation, "1", rustFinding)
	frame, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": n.Method, "params": n.Params})
	if err != nil {
		t.Fatal(err)
	}
	b.srv.send(string(frame))
	b.await(t, "Rust publication routed", func(r []ResultSet) bool { return len(r) == 1 && len(r[0].Diagnostics) == 1 })
	set := b.sets(t)[0]
	d := set.Diagnostics[0]
	if set.ProviderID != rustAnalyzerProviderID || set.Origin != "lsp" || set.Scope.DocumentID != "draft" || d.Source != "rust-analyzer/rustc" || d.Code != "E0308" || d.Severity != "error" || d.Freshness != "current" || d.Basis.Kind != "document" || d.Basis.Version == nil || *d.Basis.Version != 1 || d.Range == nil || d.Range.End.Character != 23 {
		t.Fatalf("not normalized and version-proven: %+v", set)
	}
	if _, err := b.h.ChangeDocument(context.Background(), b.context, "draft", 2, "fn main() {}\n"); err != nil {
		t.Fatal(err)
	}
	before := b.sets(t)
	for _, version := range []string{"1", "3"} {
		b.publish("a.rs", generation, version, "")
		if got := b.sets(t); !reflect.DeepEqual(before, got) {
			t.Fatalf("mismatched version %s replaced results: %+v", version, got)
		}
	}
	b.publish("a.rs", generation, "2", "")
	if got := b.sets(t); len(got) != 1 || len(got[0].Diagnostics) != 0 {
		t.Fatalf("matching empty push did not clear: %+v", got)
	}
	b.publish("a.rs", generation, "2", rustFinding) // range no longer fits this draft
	d = b.sets(t)[0].Diagnostics[0]
	if d.Range != nil || d.LocationIssue != "invalidRange" || !b.snapshot(t).Providers[0].Coverage.Partial {
		t.Fatalf("unverified range kept: %+v", d)
	}
	b.publish("a.rs", generation, "", rustFinding)
	d = b.sets(t)[0].Diagnostics[0]
	if d.Freshness != "uncertain" || d.Range != nil || d.Basis.Version != nil || d.Basis.ObservedVersion != nil {
		t.Fatalf("unversioned Rust push certified freshness/position: %+v", d)
	}
	b.publish("a.rs", generation, "", "")
	if got := b.sets(t); len(got) != 1 || len(got[0].Diagnostics) != 0 {
		t.Fatalf("unversioned clear failed: %+v", got)
	}
}

func TestRustLiveOwnerPublicationGuards(t *testing.T) {
	b := newRustOwnerEnv(t)
	generation := b.open(t, "draft")
	b.publish("a.rs", generation, "1", rustFinding)
	before := b.sets(t)
	for name, n := range map[string]SessionNotification{
		"old generation": b.notification(uriOf(b.root, "a.rs"), "old-generation", "1", ""),
		"outside root":   b.notification(fileURI(filepath.Join(t.TempDir(), "outside.rs")), generation, "1", rustFinding),
		"unopened":       b.notification(uriOf(b.root, "b.rs"), generation, "1", rustFinding),
		"unowned":        b.notification(uriOf(b.root, "Cargo.toml"), generation, "1", rustFinding),
		"malformed URI":  b.notification("https://example.invalid/a.rs", generation, "1", ""),
	} {
		t.Run(name, func(t *testing.T) {
			b.owner.notification(n)
			if got := b.sets(t); !reflect.DeepEqual(before, got) {
				t.Fatalf("rejected publication replaced results: %+v", got)
			}
		})
	}
	if _, err := b.h.CloseDocument(context.Background(), b.context, "draft"); err != nil {
		t.Fatal(err)
	}
	b.publish("a.rs", generation, "1", rustFinding)
	if got := b.sets(t); len(got) != 0 {
		t.Fatalf("closed document resurrected: %+v", got)
	}
	// A reopened wire URI on the same transport is ambiguous, even with version 1.
	b.open(t, "reopened")
	b.publish("a.rs", generation, "1", rustFinding)
	if got := b.sets(t); len(got) != 0 {
		t.Fatalf("reused URI certified a new lifecycle: %+v", got)
	}
}

func TestRustBridgeIgnoresOtherIdentityAndMethod(t *testing.T) {
	b := newRustOwnerEnv(t)
	generation := b.open(t, "draft")
	bridge := NewRustBridge(b.root, b.h.Manager)
	for _, mutate := range []func(*SessionNotification){
		func(n *SessionNotification) { n.Project.HostID = "other-host" },
		func(n *SessionNotification) { n.ProviderID = "gopls" },
		func(n *SessionNotification) { n.Method = "window/showMessage" },
	} {
		n := b.notification(uriOf(b.root, "a.rs"), generation, "1", rustFinding)
		mutate(&n)
		bridge.Handle(n)
	}
	if got := b.sets(t); len(got) != 0 {
		t.Fatalf("unrelated notification published: %+v", got)
	}
}

func TestRustLiveOwnerPublicationInvalidation(t *testing.T) {
	for _, kind := range []string{"input drift", "revocation"} {
		t.Run(kind, func(t *testing.T) {
			b := newRustOwnerEnv(t)
			generation := b.open(t, "draft")
			b.publish("a.rs", generation, "1", rustFinding)
			if kind == "revocation" {
				s, err := b.h.Revoke(b.context)
				if err != nil {
					t.Fatal(err)
				}
				b.context, b.h.snap.Context = s.Context, s.Context
				b.owner.Revoke(b.root)
			} else {
				b.drift.Store(true)
				b.owner.Refresh()
				if s := b.snapshot(t); len(s.ResultSets) != 1 || s.ResultSets[0].Diagnostics[0].Freshness != "stale" || s.Providers[0].Phase != "failed" {
					t.Fatalf("input invalidation did not stale retained findings: %+v", s)
				}
			}
			before := b.sets(t)
			b.publish("a.rs", generation, "1", "")
			if got := b.sets(t); !reflect.DeepEqual(before, got) {
				t.Fatalf("invalidated session replaced results: %+v", got)
			}
			if b.owner.Sessions.CurrentGeneration(b.root, b.cfg, generation) {
				t.Fatal("invalidated session still current")
			}
		})
	}
}

package codediagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func assertPublicationUnchanged(t *testing.T, x *liveLifecycle, before Snapshot) {
	t.Helper()
	after := x.snapshot(t)
	if after.Sequence != before.Sequence || !reflect.DeepEqual(after.ResultSets, before.ResultSets) {
		t.Fatal("ambiguous publication changed sequence or result sets")
	}
}

func TestLiveReusedURIDiscardsFindingsAndEmpties(t *testing.T) {
	for _, text := range []string{"aa", "bb"} {
		t.Run(text, func(t *testing.T) {
			x := newLiveLifecycle(t)
			ctx, c := context.Background(), x.owner.snap.Context
			ds := []any{liveDiagnostic("held A/v1", Range{Position{0, 0}, Position{0, 1}})}
			if _, err := x.owner.OpenDocument(ctx, c, "a.go", "A", 1, "aa", ""); err != nil {
				t.Fatal(err)
			}
			x.push(t, uriOf(x.root, "a.go"), 1, ds)
			if _, err := x.owner.CloseDocument(ctx, c, "A"); err != nil {
				t.Fatal(err)
			}
			// Repeated closes/reopens never reset transport history, even with
			// identical text, changed text, or a matching valid range.
			for i := range 3 {
				id := fmt.Sprintf("B%d", i)
				if _, err := x.owner.OpenDocument(ctx, c, "a.go", id, 1, text, ""); err != nil {
					t.Fatal(err)
				}
				before := x.snapshot(t)
				for _, batch := range []any{ds, []any{liveDiagnostic("B also cannot be proven", Range{})}, []any{}} {
					x.push(t, uriOf(x.root, "a.go"), 1, batch)
					assertPublicationUnchanged(t, x, before)
				}
				x.push(t, uriOf(x.root, "a.go"), nil, ds)
				assertPublicationUnchanged(t, x, before)
				if _, err := x.owner.CloseDocument(ctx, c, id); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// A reopened URI sends versions above everything the transport sent before, so
// publications naming them describe the new lifecycle and nothing earlier.
func TestLiveReopenedURICertifiedByItsWireVersions(t *testing.T) {
	x := newLiveLifecycle(t)
	ctx, c := context.Background(), x.owner.snap.Context
	uri := uriOf(x.root, "a.go")
	if _, err := x.owner.OpenDocument(ctx, c, "a.go", "A", 1, "aa", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := x.owner.ChangeDocument(ctx, c, "A", 2, "ab"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.owner.OpenDocument(ctx, c, "b.go", "B", 1, "bb", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := x.owner.CloseDocument(ctx, c, "A"); err != nil {
		t.Fatal(err)
	}
	// Wire versions so far: a.go 1 and 2, b.go 1. The reopen starts above 2.
	if _, err := x.owner.OpenDocument(ctx, c, "a.go", "A2", 1, "aa", ""); err != nil {
		t.Fatal(err)
	}
	before := x.snapshot(t)
	for _, version := range []any{1, 2, nil} {
		x.push(t, uri, version, []any{liveDiagnostic("earlier lifecycle", Range{})})
		assertPublicationUnchanged(t, x, before)
	}
	current := func(message string, version int64) {
		t.Helper()
		for _, set := range x.snapshot(t).ResultSets {
			if set.Scope.DocumentID != "A2" {
				continue
			}
			if len(set.Diagnostics) != 1 {
				t.Fatalf("%+v", set)
			}
			d := set.Diagnostics[0]
			if d.Message != message || d.Freshness != "current" || d.Basis.Version == nil || *d.Basis.Version != version || d.Range == nil {
				t.Fatalf("%+v", d)
			}
			return
		}
		t.Fatal("reopened lifecycle has no results")
	}
	x.push(t, uri, 3, []any{liveDiagnostic("reopened", Range{Position{0, 0}, Position{0, 1}})})
	current("reopened", 1)
	if _, err := x.owner.ChangeDocument(ctx, c, "A2", 2, "ab"); err != nil {
		t.Fatal(err)
	}
	x.push(t, uri, 4, []any{liveDiagnostic("edited", Range{Position{0, 0}, Position{0, 2}})})
	current("edited", 2)
	// A version the lifecycle has not sent yet is not proof either.
	before = x.snapshot(t)
	x.push(t, uri, 9, []any{})
	assertPublicationUnchanged(t, x, before)
}

func TestLiveIsolationHistorySurvivesReapproval(t *testing.T) {
	x := newLiveLifecycle(t)
	ctx, c := context.Background(), x.owner.snap.Context
	if _, err := x.owner.OpenDocument(ctx, c, "a.go", "A", 1, "aa", ""); err != nil {
		t.Fatal(err)
	}
	gen := x.mgr.Info(x.root, x.cfg).Generation
	s, err := x.owner.Revoke(c) // manager-only revoke: transport remains approved/live
	if err != nil {
		t.Fatal(err)
	}
	x.owner.snap = s
	before := x.snapshot(t)
	x.push(t, uriOf(x.root, "a.go"), 1, []any{})
	assertPublicationUnchanged(t, x, before)
	s, err = x.owner.Approve(s.Context, s.Proposal.Fingerprint, "session")
	if err != nil {
		t.Fatal(err)
	}
	x.owner.snap = s
	if _, err := x.owner.OpenDocument(ctx, s.Context, "a.go", "B", 1, "bb", ""); err != nil {
		t.Fatal(err)
	}
	if x.mgr.Info(x.root, x.cfg).Generation != gen {
		t.Fatal("test did not reuse transport")
	}
	before = x.snapshot(t)
	x.push(t, uriOf(x.root, "a.go"), 1, []any{liveDiagnostic("old", Range{})})
	x.push(t, uriOf(x.root, "a.go"), 1, []any{})
	assertPublicationUnchanged(t, x, before)
}

func TestLiveIsolationHistorySurvivesNewAttachmentOwner(t *testing.T) {
	x := newLiveLifecycle(t)
	ctx := context.Background()
	if _, err := x.owner.OpenDocument(ctx, x.owner.snap.Context, "a.go", "A", 1, "aa", ""); err != nil {
		t.Fatal(err)
	}
	gen := x.mgr.Info(x.root, x.cfg).Generation
	x.owner.Close() // joins best-effort closes, but not the independently owned session
	x.owner = NewManagerForTest(t, x.root)
	x.owner.SetDocumentProvider(SessionDocuments{Sessions: x.mgr, Resolve: func(AuthorizedRoot, string) (DocumentServer, bool) {
		return DocumentServer{Config: x.cfg, LanguageID: "go"}, true
	}})
	x.bridge = NewGoplsBridge(x.root, x.owner.Manager)
	if _, err := x.owner.OpenDocument(ctx, x.owner.snap.Context, "a.go", "B", 1, "bb", ""); err != nil {
		t.Fatal(err)
	}
	if x.mgr.Info(x.root, x.cfg).Generation != gen {
		t.Fatal("test did not reuse transport")
	}
	before := x.snapshot(t)
	x.push(t, uriOf(x.root, "a.go"), 1, []any{liveDiagnostic("held before detach", Range{})})
	x.push(t, uriOf(x.root, "a.go"), 1, []any{})
	assertPublicationUnchanged(t, x, before)
}

func TestLiveIsolationCrashRetrySameURIAndVersion(t *testing.T) {
	x := newLiveLifecycle(t)
	ctx, c := context.Background(), x.owner.snap.Context
	if _, err := x.owner.OpenDocument(ctx, c, "a.go", "A", 1, "aa", ""); err != nil {
		t.Fatal(err)
	}
	x.mgr.mu.Lock()
	old := x.mgr.entries[keyOf(x.root, x.cfg)].session
	x.mgr.mu.Unlock()
	_ = x.server.Terminate()
	select {
	case <-old.disposed:
	case <-time.After(10 * time.Second):
		t.Fatal("disposal did not finish")
	}
	if _, err := x.owner.CloseDocument(ctx, c, "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := x.mgr.Retry(ctx, x.root, x.cfg); err != nil {
		t.Fatal(err)
	}
	x.server = x.next()
	if _, err := x.owner.OpenDocument(ctx, c, "a.go", "B", 1, "bb", ""); err != nil {
		t.Fatal(err)
	}
	x.push(t, uriOf(x.root, "a.go"), 1, []any{liveDiagnostic("new transport", Range{Position{0, 0}, Position{0, 1}})})
	before := x.snapshot(t)
	d := before.ResultSets[0].Diagnostics[0]
	if d.Freshness != "current" || d.Basis.DocumentID != "B" || d.Basis.Version == nil || *d.Basis.Version != 1 || d.Range == nil || before.ResultSets[0].ProviderID != goplsProviderID {
		t.Fatal("isolated synchronized new transport was not eligible")
	}
	for _, ds := range []any{[]any{liveDiagnostic("old callback", Range{})}, []any{}} {
		raw, _ := json.Marshal(map[string]any{"uri": uriOf(x.root, "a.go"), "version": 1, "diagnostics": ds})
		x.bridge.Handle(SessionNotification{Project: x.root.Project(), ProviderID: goplsProviderID, Generation: old.generation, Method: "textDocument/publishDiagnostics", Params: raw})
		assertPublicationUnchanged(t, x, before)
	}
}

func TestLiveIsolationBudgetFailsClosedAndCanonicalURI(t *testing.T) {
	x := newLiveLifecycle(t)
	ctx, c := context.Background(), x.owner.snap.Context
	// Alias resolves to the same wire URI and cannot acquire a fresh lifecycle.
	if err := os.Symlink("a.go", filepath.Join(x.root.Project().Root, "alias.go")); err != nil {
		t.Fatal(err)
	}
	for i, path := range []string{"a.go", "alias.go"} {
		id := fmt.Sprintf("alias%d", i)
		if _, err := x.owner.OpenDocument(ctx, c, path, id, 1, "aa", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := x.owner.CloseDocument(ctx, c, id); err != nil {
			t.Fatal(err)
		}
	}
	provider := x.owner.provider.(SessionDocuments)
	gen := x.mgr.Info(x.root, x.cfg).Generation
	if provider.PublicationIsolated(x.root, "a.go", gen) || provider.PublicationIsolated(x.root, "alias.go", gen) {
		t.Fatal("canonical URI alias escaped ambiguity")
	}
	// Keep a certified set on a distinct URI: after exhaustion even an empty
	// unversioned push must not erase that set or manufacture an all-clean
	// result. Versioned pushes stay provable by their lifecycle's wire versions.
	if _, err := x.owner.OpenDocument(ctx, c, "b.go", "certified", 1, "bb", ""); err != nil {
		t.Fatal(err)
	}
	x.push(t, uriOf(x.root, "b.go"), 1, []any{liveDiagnostic("certified", Range{})})
	// Cross both history budgets. No FIFO eviction may restore certification.
	for i := range maxPublicationURIs + 1 {
		path := fmt.Sprintf("p%d.go", i)
		if err := os.WriteFile(filepath.Join(x.root.Project().Root, path), []byte("aa"), 0o600); err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("p%d", i)
		if _, err := x.owner.OpenDocument(ctx, c, path, id, 1, "aa", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := x.owner.CloseDocument(ctx, c, id); err != nil {
			t.Fatal(err)
		}
	}
	before := x.snapshot(t)
	x.push(t, uriOf(x.root, "b.go"), nil, []any{})
	assertPublicationUnchanged(t, x, before)
	x.push(t, uriOf(x.root, "b.go"), 1, []any{liveDiagnostic("still certified", Range{})})
	if s := x.snapshot(t); s.Sequence == before.Sequence {
		t.Fatal("the open lifecycle lost its versioned proof")
	}
	for _, path := range []string{"a.go", "p256.go"} {
		if _, err := x.owner.OpenDocument(ctx, c, path, path, 1, "bb", ""); err != nil {
			t.Fatal(err)
		}
		before := x.snapshot(t)
		x.push(t, uriOf(x.root, path), 1, []any{liveDiagnostic("late", Range{})})
		x.push(t, uriOf(x.root, path), 1, []any{})
		assertPublicationUnchanged(t, x, before)
	}
	x.mgr.mu.Lock()
	s := x.mgr.entries[keyOf(x.root, x.cfg)].session
	x.mgr.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.publicationFull || len(s.publicationURIs) != maxPublicationURIs {
		t.Fatal("history did not remain bounded and fail closed")
	}
}

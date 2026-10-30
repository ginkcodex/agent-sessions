package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// A language that is off leaves the approval proposal, keeps the project's
// grant, and stays off when the attachment rediscovers.
func TestCodeDiagnosticsLanguageOff(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	for _, name := range []string{"gopls-server", "ruff-server", "a.go", "b.py"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("synthetic"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "languages"}, CWD: dir, RepoRoot: dir}})
	var launches atomic.Int32
	discover := func(root cd.AuthorizedRoot, off cd.DisabledLanguages) ([]cd.Registration, error) {
		var regs []cd.Registration
		for _, s := range []struct{ id, lang, ext string }{{"gopls", "go", ".go"}, {"ruff", "python", ".py"}} {
			if off.Disables(s.id) {
				continue
			}
			cfg := cd.ServerConfig{ProviderID: s.id, Executable: filepath.Join(root.Project().Root, s.id+"-server"), AdapterPolicy: "fake-only"}
			state := cd.ProviderState{ProviderID: s.id, Availability: "available", Phase: "idle", Freshness: "uncertain", Coverage: cd.Coverage{Kind: "openedDocuments", Roots: []string{""}, Partial: true, ExternalChanges: "unwatched"}}
			regs = append(regs, cd.Registration{ProviderID: s.id, Availability: "available", Live: &cd.LiveRegistration{State: state, Server: cfg, Resolve: func(path string) (string, bool) {
				return s.lang, strings.HasSuffix(path, s.ext)
			}}})
		}
		return regs, nil
	}
	owner, err := newDiagnosticsOwnerWithLive(t.TempDir(), "fake-host", discover, cd.SessionOptions{Launch: func(cd.Invocation) (cd.SessionProcess, cd.SessionStreams, error) {
		launches.Add(1)
		return nil, cd.SessionStreams{}, errors.New("synthetic failure")
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.discoverSaved = func(cd.AuthorizedRoot, cd.DisabledLanguages) []cd.Registration { return nil }
	e := NewEngine(WithService(NewService(catalog, nil)), WithCacheEnabled(false))
	e.codeDiagnostics = owner
	go owner.events(e)
	t.Cleanup(func() { _ = e.Close() })
	ctx := context.Background()
	discovered := func(disabled []string) cd.Snapshot {
		t.Helper()
		attached, err := e.AttachCodeDiagnosticsWithout(ctx, dir, 1, disabled)
		if err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			s, err := e.GetCodeDiagnosticsSnapshot(ctx, attached.Context)
			if err != nil {
				t.Fatal(err)
			}
			if s.Proposal != nil {
				return s
			}
			if time.Now().After(deadline) {
				t.Fatal("discovery did not complete")
			}
		}
	}
	proposed := func(s cd.Snapshot) []string {
		var ids []string
		for _, tool := range s.Proposal.Tools {
			ids = append(ids, tool.ProviderID)
		}
		return ids
	}

	all := discovered(nil)
	if ids := proposed(all); !slices.Equal(ids, []string{"gopls", "ruff"}) {
		t.Fatal(ids)
	}
	if _, err := e.ApproveCodeDiagnostics(ctx, all.Context, "persistent", all.Proposal.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DetachCodeDiagnostics(ctx, all.Context); err != nil {
		t.Fatal(err)
	}

	off := discovered([]string{"python"})
	if ids := proposed(off); !slices.Equal(ids, []string{"gopls"}) {
		t.Fatal(ids)
	}
	// The new fingerprint ends the old one's leases, which stops its sessions;
	// the project's grant carries over to it.
	if off.Proposal.Fingerprint == all.Proposal.Fingerprint || off.Approval.State != "approved" {
		t.Fatalf("%+v", off.Approval)
	}
	states := map[string]cd.ProviderState{}
	for _, p := range off.Providers {
		states[p.ProviderID] = p
	}
	for _, id := range []string{"ruff", "pyright"} {
		if p := states[id]; p.Availability != "disabled" || p.Notice != "Off in Settings" {
			t.Fatalf("%s: %+v", id, p)
		}
	}
	if _, err := e.OpenCodeDiagnosticDocument(ctx, off.Context, "b.py", "one", 1, "draft", ""); err == nil || launches.Load() != 0 {
		t.Fatal("a language that is off started a server", err, launches.Load())
	}

	// Rediscovery after approved setup keeps the attachment's languages.
	owner.mu.Lock()
	root := owner.roots[off.Context.AttachmentID]
	owner.mu.Unlock()
	regs, err := owner.rediscovery(off.Context.AttachmentID)(root)
	if err != nil {
		t.Fatal(err)
	}
	disabled := 0
	for _, r := range regs {
		if (r.ProviderID == "ruff" || r.ProviderID == "pyright") != (r.Availability == "disabled") {
			t.Fatalf("%+v", r)
		}
		if r.Availability == "disabled" {
			disabled++
		}
	}
	if disabled != 2 {
		t.Fatal(regs)
	}
	if _, err := e.DetachCodeDiagnostics(ctx, off.Context); err != nil {
		t.Fatal(err)
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if _, kept := owner.disabled[off.Context.AttachmentID]; kept {
		t.Fatal("detach kept the attachment's languages")
	}
}

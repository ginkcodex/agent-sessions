package app

import (
	"context"
	"slices"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/index"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

func TestPing(t *testing.T) {
	a := NewApp()
	if got := a.Ping(""); got != "Hello from agent-sessions backend!" {
		t.Errorf("Ping(\"\") = %q, want default greeting", got)
	}
	if got := a.Ping("dev"); got != "Hello dev, welcome to agent-sessions!" {
		t.Errorf("Ping(%q) = %q, want personalized greeting", "dev", got)
	}
}

func TestOnStartupStoresContext(t *testing.T) {
	// OnStartup runs before any bound call; the service must retain the
	// startup context, with the injected desktop, for later runtime calls.
	type key struct{}
	a := NewApp()
	ctx := context.WithValue(t.Context(), key{}, "startup")
	var emitted []string
	OnStartup(a, ctx, Desktop{Emit: func(name string, _ any) { emitted = append(emitted, name) }})
	t.Cleanup(func() { _ = Close(a) })
	if a.ctx == nil || a.ctx.Value(key{}) != "startup" {
		t.Error("OnStartup did not retain the runtime context")
	}
	a.desktopEvents.Emit("probe", nil)
	if !slices.Contains(emitted, "probe") {
		t.Errorf("injected emitter not used: %v", emitted)
	}
}

func TestOnStartupCacheFirstLoadsCatalog(t *testing.T) {
	// 1. Seed a private cache DB with one session.
	// NewAppWithService has no roots, so its index is the key of empty roots.
	db, err := index.Open(t.Context(), t.TempDir(), index.RootsKey("", "", "", `agents-v1:["claude-code"]`))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	seed := model.SessionMeta{
		Ref:   model.SessionRef{Agent: model.AgentClaude, ID: "cached1"},
		Title: "Cached Session",
	}
	if err := db.CommitScan(t.Context(), model.AgentClaude, provider.ScanResult{
		Changed: []model.SessionMeta{seed},
	}); err != nil {
		t.Fatalf("CommitScan: %v", err)
	}
	_ = db.Close()

	// 2. Build an App with a fake provider and the same cache dir.
	fake := providertest.NewFake(model.AgentClaude, "Claude")
	fake.DetectionData = provider.Detection{Present: true}
	fake.Sessions = []model.SessionMeta{{
		Ref:   model.SessionRef{Agent: model.AgentClaude, ID: "live1"},
		Title: "Live Session",
	}}

	catalog := scan.NewCatalog()
	svc := NewService(catalog, provider.Set{fake})
	a := NewAppWithService(svc)
	a.cacheEnabled = true
	a.cacheDirOverride = db.CacheDir()

	// Cache-first startup must synchronously populate the catalog.
	OnStartup(a, t.Context(), Desktop{})
	// Release the index before TempDir cleanup; Windows cannot delete open files.
	t.Cleanup(func() { _ = Close(a) })

	if m, ok := a.svc.Catalog().Get(model.SessionRef{Agent: model.AgentClaude, ID: "cached1"}); !ok || m.Title != "Cached Session" {
		t.Errorf("expected cached1 in catalog after OnStartup, got %+v", m)
	}
	if a.refresher == nil {
		t.Fatal("expected refresher to be wired")
	}
	if a.refresher.Generation() != 1 {
		t.Errorf("expected generation 1, got %d", a.refresher.Generation())
	}
}

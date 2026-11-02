package engine

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/index"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/all"
)

func TestAgentSettingsStartupAndRestart(t *testing.T) {
	root := t.TempDir()
	roots := paths.Roots{Config: filepath.Join(root, "config"), Claude: filepath.Join(root, "claude"), Codex: filepath.Join(root, "codex"), OpenCodeData: filepath.Join(root, "opencode")}
	if _, err := all.SaveEnabled(roots.Config, []string{"codex"}); err != nil {
		t.Fatal(err)
	}
	e := NewEngine(WithRoots(roots), WithCacheEnabled(false))
	settings, err := e.GetAgentSettings(t.Context())
	if err != nil || len(settings.Supported) != 3 || !reflect.DeepEqual(settings.Active, []string{"codex"}) || settings.RestartRequired {
		t.Fatalf("settings=%+v %v", settings, err)
	}
	if len(e.runner.Providers) != 1 || len(e.Service().NewSessionAgents()) != 1 {
		t.Fatal("unfiltered scanner or launch list")
	}
	if _, err := e.Service().NewSessionLaunch(model.AgentClaude, root); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("disabled launch=%v", err)
	}
	key := e.indexKey()
	settings, err = e.SetAgentSettings(t.Context(), []string{"opencode", "claude-code", "opencode"})
	if err != nil || !settings.RestartRequired || !reflect.DeepEqual(settings.Enabled, []string{"claude-code", "opencode"}) || !reflect.DeepEqual(settings.Active, []string{"codex"}) {
		t.Fatalf("updated=%+v %v", settings, err)
	}
	if e.indexKey() != key || len(e.runner.Providers) != 1 {
		t.Fatal("setter mutated runtime")
	}
	restarted := NewEngine(WithRoots(roots), WithCacheEnabled(false))
	settings, err = restarted.GetAgentSettings(t.Context())
	if err != nil || settings.RestartRequired || !reflect.DeepEqual(settings.Active, settings.Enabled) || restarted.indexKey() == key {
		t.Fatalf("restarted=%+v %v", settings, err)
	}
	if len(restarted.Service().NewSessionAgents()) != 2 {
		t.Fatal("launch list missing startup subset")
	}
}

func TestAgentCacheSubsetSeparation(t *testing.T) {
	roots := paths.Roots{Config: t.TempDir(), Claude: filepath.Join(t.TempDir(), "missing")}
	dir := t.TempDir()
	makeEngine := func(ids []string) *Engine {
		t.Helper()
		if _, err := all.SaveEnabled(roots.Config, ids); err != nil {
			t.Fatal(err)
		}
		e := NewEngine(WithRoots(roots), WithCacheDir(dir))
		if err := e.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = e.Close() })
		return e
	}
	a := makeEngine([]string{"claude-code"})
	// Wait for bootstrap before seeding a row that would leak through a roots-only key.
	waitFor(t, "bootstrap", func() bool { status, _ := a.DiscoveryStatus(t.Context()); return status.Complete })
	db, err := index.Open(t.Context(), dir, a.indexKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CommitScan(t.Context(), model.AgentClaude, provider.ScanResult{Changed: []model.SessionMeta{claudeSession("disabled")}}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	b := makeEngine([]string{"codex"})
	c := makeEngine([]string{"codex", "codex"})
	if a.indexKey() == b.indexKey() || b.indexKey() != c.indexKey() {
		t.Fatal("incorrect subset cache identity")
	}
	if a.lock == nil || b.lock == nil || c.lock != nil || c.readDB == nil {
		t.Fatal("subset writer/reader separation failed")
	}
	// Different provider sets must not seed each other's cached metadata.
	if _, ok := b.Service().Catalog().Get(model.SessionRef{Agent: model.AgentClaude, ID: "disabled"}); ok {
		t.Fatal("disabled cached session leaked")
	}
}

func TestAgentSettingsWithConfigOnlyRoots(t *testing.T) {
	roots := paths.Roots{Config: t.TempDir()}
	e := NewEngine(WithRoots(roots), WithCacheEnabled(false))
	if e.roots.Config != roots.Config {
		t.Fatal("WithRoots overwritten")
	}
	if _, err := e.SetAgentSettings(t.Context(), []string{"codex"}); err != nil {
		t.Fatal(err)
	}
}

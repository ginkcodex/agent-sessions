package rpc

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func TestAgentSettingsRPCRoundTrip(t *testing.T) {
	roots := paths.Roots{Config: t.TempDir(), Claude: filepath.Join(t.TempDir(), "missing")}
	e := engine.NewEngine(engine.WithRoots(roots), engine.WithCacheEnabled(false))
	s, c, cleanup := setupPipePair(e)
	defer cleanup()
	_ = s
	init, err := c.Initialize(t.Context(), InitializeRequest{ProtocolVersion: ProtocolVersion})
	if err != nil || !init.Capabilities.AgentSettings {
		t.Fatalf("initialize=%+v %v", init, err)
	}
	got, err := c.GetAgentSettings(t.Context())
	if err != nil || len(got.Supported) != 3 || len(got.Active) != 3 || got.RestartRequired {
		t.Fatalf("get=%+v %v", got, err)
	}
	got, err = c.SetAgentSettings(t.Context(), []string{"codex", "codex"})
	if err != nil || !got.RestartRequired || !reflect.DeepEqual(got.Enabled, []string{"codex"}) || len(got.Active) != 3 {
		t.Fatalf("set=%+v %v", got, err)
	}
	loaded, err := c.GetAgentSettings(t.Context())
	if err != nil || !reflect.DeepEqual(got, loaded) {
		t.Fatalf("round trip=%+v %v", loaded, err)
	}
	if _, err := c.SetAgentSettings(t.Context(), []string{}); err == nil {
		t.Fatal("accepted empty selection")
	}
}

func TestAgentSettingsOldServerUnsupported(t *testing.T) {
	_, c, cleanup := setupPipePair(nil)
	defer cleanup()
	if _, err := c.GetAgentSettings(t.Context()); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("get=%v", err)
	}
	if _, err := c.SetAgentSettings(t.Context(), []string{"codex"}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("set=%v", err)
	}
}

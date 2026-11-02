package app

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func TestAgentSettingsBindingRoutesOnlyToActiveHost(t *testing.T) {
	localRoots := paths.Roots{Config: t.TempDir(), Claude: filepath.Join(t.TempDir(), "local")}
	remoteRoots := paths.Roots{Config: t.TempDir(), Claude: filepath.Join(t.TempDir(), "remote")}
	local := engine.NewEngine(engine.WithRoots(localRoots), engine.WithCacheEnabled(false))
	remote := engine.NewEngine(engine.WithRoots(remoteRoots), engine.WithCacheEnabled(false))
	a := &App{localEngine: local, backend: remote}
	got, err := a.SetAgentSettings([]string{"codex"})
	if err != nil || !got.RestartRequired {
		t.Fatalf("set=%+v %v", got, err)
	}
	loaded, err := a.GetAgentSettings()
	if err != nil || !reflect.DeepEqual(got, loaded) {
		t.Fatalf("get=%+v %v", loaded, err)
	}
	localGot, err := local.GetAgentSettings(t.Context())
	if err != nil || len(localGot.Enabled) != 3 {
		t.Fatalf("local settings changed=%+v %v", localGot, err)
	}
	a.backend = offlineBackend{host: "unsupported"}
	if _, err := a.GetAgentSettings(); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("get=%v", err)
	}
	if _, err := a.SetAgentSettings([]string{"opencode"}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("set=%v", err)
	}
	localGot, err = local.GetAgentSettings(t.Context())
	if err != nil || len(localGot.Enabled) != 3 {
		t.Fatal("unsupported route fell back to local")
	}
}

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type repairAppBackend struct {
	engine.Backend
	ref   model.SessionRef
	calls int
}

func (b *repairAppBackend) RepairCodexSession(_ context.Context, ref model.SessionRef) (engine.CodexRepairResult, error) {
	b.ref = ref
	b.calls++
	return engine.CodexRepairResult{BackupPath: "remote backup"}, nil
}
func TestRepairCodexBindingUsesActiveHostOnly(t *testing.T) {
	remote := &repairAppBackend{}
	app := &App{backend: remote}
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "synthetic"}
	result, err := app.RepairCodexSession(ref)
	if err != nil {
		t.Fatal(err)
	}
	if remote.ref != ref || result.BackupPath != "remote backup" {
		t.Fatal("repair was not routed to active host")
	}
	app.backend = offlineBackend{host: "unsupported host"}
	if _, err := app.RepairCodexSession(ref); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
	if remote.calls != 1 {
		t.Fatal("unsupported repair fell back to another host")
	}
}

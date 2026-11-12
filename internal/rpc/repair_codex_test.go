package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type repairRPCBackend struct {
	engine.Backend
	ref model.SessionRef
}

func (b *repairRPCBackend) RepairCodexSession(_ context.Context, ref model.SessionRef) (engine.CodexRepairResult, error) {
	b.ref = ref
	return engine.CodexRepairResult{Path: "host/canonical.jsonl", BackupPath: "host/backup"}, nil
}
func TestRepairCodexRPCRoundTrip(t *testing.T) {
	backend := &repairRPCBackend{}
	_, client, cleanup := setupPipePair(backend)
	defer cleanup()
	init, err := client.Initialize(t.Context(), InitializeRequest{ProtocolVersion: ProtocolVersion})
	if err != nil || !init.Capabilities.RepairCodexSession {
		t.Fatalf("capabilities=%+v %v", init, err)
	}
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "synthetic"}
	result, err := client.RepairCodexSession(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if backend.ref != ref || result.BackupPath != "host/backup" {
		t.Fatalf("result=%+v ref=%+v", result, backend.ref)
	}
}
func TestRepairCodexOldServerUnsupported(t *testing.T) {
	_, client, cleanup := setupPipePair(nil)
	defer cleanup()
	if _, err := client.RepairCodexSession(t.Context(), model.SessionRef{}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}

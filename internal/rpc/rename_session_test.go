package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type renameRPCBackend struct {
	engine.Backend
	ref   model.SessionRef
	title string
}

func (b *renameRPCBackend) RenameSession(_ context.Context, ref model.SessionRef, title string) error {
	b.ref = ref
	b.title = title
	return nil
}
func TestRenameSessionRPCRoundTrip(t *testing.T) {
	b := &renameRPCBackend{}
	_, c, cleanup := setupPipePair(b)
	defer cleanup()
	init, err := c.Initialize(t.Context(), InitializeRequest{ProtocolVersion: ProtocolVersion})
	if err != nil || !init.Capabilities.RenameSession {
		t.Fatalf("initialize=%+v %v", init, err)
	}
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "synthetic"}
	if err := c.RenameSession(t.Context(), ref, "native title"); err != nil {
		t.Fatal(err)
	}
	if b.ref != ref || b.title != "native title" {
		t.Fatalf("ref=%v title=%s", b.ref, b.title)
	}
}
func TestRenameSessionOldServerUnsupported(t *testing.T) {
	_, c, cleanup := setupPipePair(nil)
	defer cleanup()
	if err := c.RenameSession(t.Context(), model.SessionRef{}, "title"); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}

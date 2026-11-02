package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type renameAppBackend struct {
	engine.Backend
	ref   model.SessionRef
	title string
}

func (b *renameAppBackend) RenameSession(_ context.Context, ref model.SessionRef, title string) error {
	b.ref = ref
	b.title = title
	return nil
}
func TestRenameSessionBindingActiveHostOnly(t *testing.T) {
	remote := &renameAppBackend{}
	a := &App{backend: remote}
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "synthetic"}
	if err := a.RenameSession(ref, "title"); err != nil {
		t.Fatal(err)
	}
	if remote.ref != ref || remote.title != "title" {
		t.Fatal("not routed to remote")
	}
	a.backend = offlineBackend{host: "old server"}
	if err := a.RenameSession(ref, "never local"); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
	if remote.title != "title" {
		t.Fatal("unsupported host fell back")
	}
}

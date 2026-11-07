package app

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/rpc"
)

type cappedBackendStub struct {
	engine.Backend
	got *engine.PageCap
}

func (b cappedBackendStub) GetMessagesCapped(_ context.Context, _ model.SessionRef, offset, limit int, pc engine.PageCap) (engine.MessagesPage, error) {
	*b.got = pc
	return engine.MessagesPage{Offset: offset, Limit: limit, Capped: true}, nil
}

func TestGetMessagesCappedBinding(t *testing.T) {
	a := NewApp()
	var got engine.PageCap
	a.backend = cappedBackendStub{got: &got}
	pc := PageCap{MaxBytes: 256 << 10, Anchor: 12}
	page, err := a.GetMessagesCapped(model.SessionRef{}, 2, 50, pc)
	if err != nil || got != pc || !page.Capped || page.Offset != 2 {
		t.Fatalf("page %+v, cap %+v, %v", page, got, err)
	}
	a.backend = offlineBackend{host: "h"}
	if _, err := a.GetMessagesCapped(model.SessionRef{}, -1, 50, pc); !errors.Is(err, rpc.ErrDisconnected) {
		t.Fatalf("offline: %v", err)
	}
}

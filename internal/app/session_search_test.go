package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/rpc"
)

type sessionSearchStub struct {
	engine.Backend
	result engine.SessionSearchResult
	err    error
}

func (b sessionSearchStub) SearchSession(context.Context, engine.SessionSearchRequest) (engine.SessionSearchResult, error) {
	return b.result, b.err
}

func TestSearchSessionReplyRoutes(t *testing.T) {
	req := SessionSearchRequest{Query: "retry"}
	a := NewApp()
	a.backend = sessionSearchStub{result: engine.SessionSearchResult{Revision: "e.1.1"}}
	reply, err := a.SearchSession(req)
	if err != nil || !reply.Supported || reply.Result == nil || reply.Result.Revision != "e.1.1" {
		t.Fatalf("local reply=%+v err=%v", reply, err)
	}
	a.conn = &connection{gen: 7, wantHost: "box", state: ConnectionState{Phase: ConnConnected, Host: "box", Generation: 7}, client: sessionSearchStub{result: engine.SessionSearchResult{Revision: "r.2.1"}}}
	reply, err = a.SearchSession(req)
	if err != nil || !reply.Supported || reply.Result == nil || reply.Result.Revision != "r.2.1" {
		t.Fatalf("remote reply=%+v err=%v", reply, err)
	}
	// An old host, or a backend without session search, is unsupported, not
	// an error.
	for _, b := range []engine.Backend{markerBackend{}, sessionSearchStub{err: engine.ErrUnsupportedAction}} {
		a.conn.client = b
		reply, err = a.SearchSession(req)
		if err != nil || reply.Supported || reply.Result != nil {
			t.Fatalf("unsupported=%+v err=%v", reply, err)
		}
		data, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != `{"supported":false}` {
			t.Fatalf("absent result must be omitted: %s", data)
		}
	}
	// Other errors surface.
	failed := errors.New("generated failure")
	a.conn.client = sessionSearchStub{err: failed}
	if _, err := a.SearchSession(req); !errors.Is(err, failed) {
		t.Fatalf("error=%v", err)
	}
	// Once a host has been selected, offline must not fall back to Local.
	a.conn = &connection{wantHost: "box", localActive: false}
	if _, err := a.SearchSession(req); !errors.Is(err, rpc.ErrDisconnected) {
		t.Fatalf("offline error=%v", err)
	}
}

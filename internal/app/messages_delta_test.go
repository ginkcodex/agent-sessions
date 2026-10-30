package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/rpc"
)

type deltaBackendStub struct {
	engine.Backend
	delta engine.MessagesDelta
	err   error
	got   *engine.MessagesSinceRequest
}

func (b deltaBackendStub) GetMessagesSince(_ context.Context, req engine.MessagesSinceRequest) (engine.MessagesDelta, error) {
	if b.got != nil {
		*b.got = req
	}
	return b.delta, b.err
}

func TestMessagesSinceReplyRoutes(t *testing.T) {
	req := MessagesSinceRequest{BaseRevision: "e.1.1", HaveOffset: 3, HaveCount: 4, Offset: -1, Limit: 20}
	local := engine.MessagesDelta{MessagesPage: engine.MessagesPage{Revision: "e.1.2", Limit: 20}, Reused: 4}
	remote := engine.MessagesDelta{MessagesPage: engine.MessagesPage{Revision: "r.3.1", Limit: 20}}
	a := NewApp()
	var got engine.MessagesSinceRequest
	a.backend = deltaBackendStub{delta: local, got: &got}
	reply, err := a.GetMessagesSince(req)
	if err != nil || !reply.Supported || reply.Delta == nil || reply.Delta.Revision != "e.1.2" || reply.Delta.Reused != 4 || reply.Host != "" || reply.Generation != 0 || got != req {
		t.Fatalf("local reply=%+v err=%v request=%+v", reply, err, got)
	}
	a.conn = &connection{gen: 7, wantHost: "box", state: ConnectionState{Phase: ConnConnected, Host: "box", Generation: 7}, client: deltaBackendStub{delta: remote}}
	reply, err = a.GetMessagesSince(req)
	if err != nil || !reply.Supported || reply.Delta == nil || reply.Delta.Revision != "r.3.1" || reply.Host != "box" || reply.Generation != 7 {
		t.Fatalf("remote reply=%+v err=%v", reply, err)
	}
	// An old host, or a backend without deltas, is unsupported, not an error.
	for _, b := range []engine.Backend{markerBackend{}, deltaBackendStub{err: engine.ErrUnsupportedAction}} {
		a.conn.client = b
		reply, err = a.GetMessagesSince(req)
		if err != nil || reply.Supported || reply.Delta != nil || reply.Host != "box" || reply.Generation != 7 {
			t.Fatalf("unsupported=%+v err=%v", reply, err)
		}
		data, err := json.Marshal(reply)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["delta"]; ok {
			t.Fatalf("absent delta must be omitted: %s", data)
		}
	}
	// Other errors surface.
	failed := errors.New("generated failure")
	a.conn.client = deltaBackendStub{err: failed}
	if _, err := a.GetMessagesSince(req); !errors.Is(err, failed) {
		t.Fatalf("error=%v", err)
	}
	// Once a host has been selected, offline must not fall back to Local.
	a.conn = &connection{wantHost: "box", localActive: false}
	if _, err := a.GetMessagesSince(req); !errors.Is(err, rpc.ErrDisconnected) {
		t.Fatalf("offline error=%v", err)
	}
}

package rpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// A capped page over RPC equals the engine's; without a cap the page is whole.
func TestGetMessagesCappedRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fake := providertest.NewFake(model.AgentClaude, "generated")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "s-1"}
	tr := &model.Transcript{}
	for i := range 10 {
		tr.Messages = append(tr.Messages, model.Message{ID: string(rune('a' + i)), Parts: []model.Part{{Text: strings.Repeat("x", 1000)}}})
	}
	fake.Transcripts[ref.ID] = tr
	e := engine.NewEngine(engine.WithCacheEnabled(false), engine.WithService(engine.NewService(nil, provider.Set{fake})))
	defer func() { _ = e.Close() }()
	s, c, cleanup := setupPipePair(e)
	defer cleanup()
	s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return e, Capabilities{}, paths.Roots{}, nil
	})
	if _, err := c.Initialize(ctx, InitializeRequest{}); err != nil {
		t.Fatal(err)
	}

	pc := engine.PageCap{MaxBytes: 3 * model.MessageBytes(&tr.Messages[0]), Anchor: -1}
	remote, err := c.GetMessagesCapped(ctx, ref, -1, 50, pc)
	if err != nil {
		t.Fatal(err)
	}
	local, err := e.GetMessagesCapped(ctx, ref, -1, 50, pc)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := json.Marshal(remote)
	l, _ := json.Marshal(local)
	if string(r) != string(l) || len(remote.Messages) != 3 || remote.Offset != 7 || !remote.Capped {
		t.Fatalf("remote %s, local %s", r, l)
	}
	whole, err := c.GetMessages(ctx, ref, -1, 50)
	if err != nil || len(whole.Messages) != 10 || whole.Capped {
		t.Fatalf("uncapped page: %d messages, capped %v, %v", len(whole.Messages), whole.Capped, err)
	}
}

// The cap is an optional field: a request without it is the old request,
// and a server whose params have no Cap still decodes a capped request.
func TestGetMessagesParamsCapIsOptional(t *testing.T) {
	raw, _ := json.Marshal(GetMessagesParams{Offset: -1, Limit: 50})
	if strings.Contains(string(raw), "cap") {
		t.Fatalf("uncapped params %s", raw)
	}
	raw, _ = json.Marshal(GetMessagesParams{Offset: -1, Limit: 50, Cap: &engine.PageCap{MaxBytes: 1, Anchor: -1}})
	var old struct {
		Ref    model.SessionRef `json:"ref"`
		Offset int              `json:"offset"`
		Limit  int              `json:"limit"`
	}
	if err := json.Unmarshal(raw, &old); err != nil || old.Offset != -1 || old.Limit != 50 {
		t.Fatalf("older server decodes %s as %+v, %v", raw, old, err)
	}
}

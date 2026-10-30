package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

var _ engine.TranscriptDeltaBackend = (*Client)(nil)

// A delta over RPC equals the engine's, through a reuse and a reset.
func TestMessagesDeltaRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fake := providertest.NewFake(model.AgentClaude, "generated")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "s-1"}
	tr := &model.Transcript{}
	for i := range 30 {
		tr.Messages = append(tr.Messages, model.Message{ID: string(rune('a' + i)), Parts: []model.Part{{Text: "generated"}}})
	}
	fake.Transcripts[ref.ID] = tr
	svc := engine.NewService(nil, provider.Set{fake})
	e := engine.NewEngine(engine.WithCacheEnabled(false), engine.WithService(svc))
	defer func() { _ = e.Close() }()
	s, c, cleanup := setupPipePair(e)
	defer cleanup()
	s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return e, Capabilities{}, paths.Roots{}, nil
	})
	res, err := c.Initialize(ctx, InitializeRequest{})
	if err != nil || !res.Capabilities.TranscriptDelta {
		t.Fatalf("initialize = %+v, %v", res, err)
	}

	page, err := c.GetMessages(ctx, ref, -1, 20)
	if err != nil || page.Revision == "" || len(page.Messages) != 20 {
		t.Fatalf("page %q with %d messages, %v", page.Revision, len(page.Messages), err)
	}
	check := func(req engine.MessagesSinceRequest, reused int) {
		t.Helper()
		remote, err := c.GetMessagesSince(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		local, err := e.GetMessagesSince(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		r, _ := json.Marshal(remote)
		l, _ := json.Marshal(local)
		if remote.Reused != reused || remote.Revision == "" || !bytes.Equal(r, l) {
			t.Fatalf("remote %s, local %s", r, l)
		}
	}
	// Unchanged: the whole page is reused.
	req := engine.MessagesSinceRequest{Ref: ref, BaseRevision: page.Revision, HaveOffset: page.Offset, HaveCount: len(page.Messages), Offset: -1, Limit: 20}
	check(req, 20)
	// A provider without resumable loads starts a generation: a full page.
	fake.Transcripts[ref.ID] = &model.Transcript{Messages: append(tr.Messages[:30:30], model.Message{ID: "new"})}
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	check(req, 0)

	// Errors are not mistaken for an old host.
	req.Limit = 0
	if _, err := c.GetMessagesSince(ctx, req); err == nil || errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("invalid request: %v", err)
	}
}

func TestMessagesDeltaOldHostFallback(t *testing.T) {
	for _, mode := range []string{"no_capability", "method_not_found"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			old := &mockBackend{}
			s, c, cleanup := setupPipePair(old)
			defer cleanup()
			s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
				return old, Capabilities{}, paths.Roots{}, nil
			})
			res, err := c.Initialize(ctx, InitializeRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if res.Capabilities.TranscriptDelta {
				t.Fatal("old host advertised transcript deltas")
			}
			if mode == "method_not_found" {
				c.mu.Lock()
				c.caps.TranscriptDelta = true
				c.mu.Unlock()
			}
			req := engine.MessagesSinceRequest{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "s-1"}, Offset: -1, Limit: 10}
			if _, err := c.GetMessagesSince(ctx, req); !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatalf("fallback = %v", err)
			}
			if page, err := c.GetMessages(ctx, req.Ref, 0, 10); err != nil || page.Revision != "" {
				t.Fatalf("old paging: %+v, %v", page, err)
			}
		})
	}
	// Absent fields are omitted, so old peers see the same JSON.
	for _, v := range []any{Capabilities{}, engine.MessagesPage{}} {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["transcriptDelta"]; present {
			t.Fatal("absent capability must be omitted")
		}
		if _, present := fields["revision"]; present {
			t.Fatal("absent revision must be omitted")
		}
	}
	// No capability means no request, even without a transport.
	if _, err := (&Client{}).GetMessagesSince(t.Context(), engine.MessagesSinceRequest{}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}

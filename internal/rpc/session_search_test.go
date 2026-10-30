package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

var _ engine.SessionSearchBackend = (*Client)(nil)

// A session search over RPC equals the engine's.
func TestSearchSessionRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	fake := providertest.NewFake(model.AgentClaude, "generated")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "s-1"}
	fake.Transcripts[ref.ID] = &model.Transcript{Messages: []model.Message{
		{Role: model.RoleUser, Parts: []model.Part{{Kind: model.PartText, Text: "fix the retry policy"}}},
		{Role: model.RoleAssistant, Parts: []model.Part{{Kind: model.PartText, Text: "done"}, {Kind: model.PartTool, Tool: &model.ToolCall{Name: "Bash", Output: "retry ok"}}}},
	}}
	svc := engine.NewService(nil, provider.Set{fake})
	e := engine.NewEngine(engine.WithCacheEnabled(false), engine.WithService(svc))
	defer func() { _ = e.Close() }()
	s, c, cleanup := setupPipePair(e)
	defer cleanup()
	s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return e, Capabilities{}, paths.Roots{}, nil
	})
	res, err := c.Initialize(ctx, InitializeRequest{})
	if err != nil || !res.Capabilities.SessionSearch {
		t.Fatalf("initialize = %+v, %v", res, err)
	}

	req := engine.SessionSearchRequest{Ref: ref, Query: "RETRY"}
	remote, err := c.SearchSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	local, err := e.SearchSession(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	want := []engine.SessionSearchHit{{MessageIndex: 0, Parts: []int{0}}, {MessageIndex: 1, Parts: []int{1}}}
	if !reflect.DeepEqual(remote, local) || !reflect.DeepEqual(remote.Hits, want) || remote.Revision == "" {
		t.Fatalf("remote %+v, local %+v", remote, local)
	}

	// Errors are not mistaken for an old host.
	req.Query = `"open`
	if _, err := c.SearchSession(ctx, req); err == nil || errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatalf("invalid query: %v", err)
	}
}

func TestSearchSessionOldHostFallback(t *testing.T) {
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
			if res.Capabilities.SessionSearch {
				t.Fatal("old host advertised session search")
			}
			if mode == "method_not_found" {
				c.mu.Lock()
				c.caps.SessionSearch = true
				c.mu.Unlock()
			}
			req := engine.SessionSearchRequest{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "s-1"}, Query: "retry"}
			if _, err := c.SearchSession(ctx, req); !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatalf("fallback = %v", err)
			}
		})
	}
	// An absent capability is omitted, so old peers see the same JSON.
	b, err := json.Marshal(Capabilities{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatal(err)
	}
	if _, present := fields["sessionSearch"]; present {
		t.Fatal("absent capability must be omitted")
	}
	// No capability means no request, even without a transport.
	if _, err := (&Client{}).SearchSession(t.Context(), engine.SessionSearchRequest{}); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}

package all_test

import (
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/provider/all"
)

func TestProviders(t *testing.T) {
	roots := paths.Roots{
		Claude:       "/tmp/fake-claude",
		Codex:        "/tmp/fake-codex",
		OpenCodeData: "/tmp/fake-opencode",
		Antigravity:  "/tmp/fake-antigravity",
	}
	set := all.Providers(roots, nil)
	if len(set) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(set))
	}
	if got := strings.Join(all.IDs(), ","); got != "claude-code,codex,opencode,antigravity" {
		t.Fatalf("IDs = %s", got)
	}
	if got, err := all.Canonical([]string{"antigravity", "claude-code", "antigravity"}); err != nil || strings.Join(got, ",") != "claude-code,antigravity" {
		t.Fatalf("Canonical = %v, %v", got, err)
	}
	for _, id := range []model.AgentID{model.AgentClaude, model.AgentCodex, model.AgentOpenCode, model.AgentAntigravity} {
		p, ok := set.Get(id)
		if !ok || p == nil {
			t.Fatalf("expected %s provider in set", id)
		}
	}
}

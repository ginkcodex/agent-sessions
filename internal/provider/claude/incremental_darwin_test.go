//go:build darwin

package claude_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/claude"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func TestIncrementalLiveEndedDarwin(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", "live.jsonl")
	write(t, path, []byte("{\"type\":\"assistant\",\"message\":{\"id\":\"a\",\"content\":[{\"type\":\"tool_use\",\"id\":\"tool\",\"name\":\"Task\",\"input\":{}}]}}\n"))
	evidence := filepath.Join(root, "sessions", "live.json")
	write(t, evidence, []byte(fmt.Sprintf(`{"pid":%d,"sessionId":"live","status":"idle"}`, os.Getpid())))
	p := claude.New(root, nil)
	cold := claude.New(root, nil)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "live"}
	var h providertest.IncrementalHarness
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	live, err := p.Load(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !live.Meta.Live || live.Messages[0].Parts[0].Tool.Status != model.ToolPending {
		t.Fatal("fixture not live")
	}
	if err := os.Remove(evidence); err != nil {
		t.Fatal(err)
	}
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	ended, err := p.Load(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Meta.Live || ended.Messages[0].Parts[0].Tool.Status != model.ToolUnknown {
		t.Fatal("fixture not ended")
	}
}

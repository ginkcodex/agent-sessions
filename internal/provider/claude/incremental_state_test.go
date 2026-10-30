package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"

	"github.com/ginkcode/agent-sessions/internal/model"
)

func TestIncrementalStateCap(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", "cap.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"user\",\"message\":{\"content\":\"hello\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := New(root, nil)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "cap"}
	r, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.State == nil || r.State.RetainedBytes() <= 0 {
		t.Fatal("missing accounted state")
	}
	old := stateCap
	stateCap = 0
	t.Cleanup(func() { stateCap = old })
	r, err = p.LoadIncremental(t.Context(), ref, r.State)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Resumed || r.State != nil || len(r.Transcript.Messages) != 1 {
		t.Fatal("cap should drop state, not transcript")
	}
}

func TestRetainedCheckpointAndSharedPayload(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", "weight.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old := stateCap
	stateCap = 32 << 10
	t.Cleanup(func() { stateCap = old })
	load := func(data string) provider.IncrementalResult {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		r, err := New(root, nil).LoadIncremental(t.Context(), model.SessionRef{Agent: model.AgentClaude, ID: "weight"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	large := strings.Repeat("x", 64<<10)
	r := load(fmt.Sprintf("{\"type\":\"summary\",\"summary\":%q}\n{\"type\":\"ai-title\",\"aiTitle\":\"short\"}\n", large))
	if r.State != nil {
		t.Fatal("unpublished checkpoint string escaped cap")
	}
	r = load(fmt.Sprintf("{\"type\":\"assistant\",\"message\":{\"id\":\"a\",\"content\":[{\"type\":\"text\",\"text\":%q},{\"type\":\"tool_use\",\"id\":\"t\",\"name\":\"Task\",\"input\":{}}]}}\n", large))
	if r.State == nil {
		t.Fatal("shared text was charged as additional retained storage")
	}
	if r.State.(*loadState).l.diag != nil {
		t.Fatal("diagnostics retained")
	}
}

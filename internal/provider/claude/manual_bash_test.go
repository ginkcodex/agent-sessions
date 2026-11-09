package claude

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/tidwall/gjson"
)

func TestManualBashRecognition(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		text    string
		ok      bool
		command string
	}{
		{"<bash-input> go test ./...\n</bash-input>", true, "go test ./..."},
		{"<bash-input>printf '<script>hi</script>'</bash-input>", true, "printf '<script>hi</script>'"},
		{"<bash-stdout></bash-stdout><bash-stderr>FAIL\n</bash-stderr>", true, ""},
		{"<bash-stdout>ok\n</bash-stdout>\n<bash-stderr></bash-stderr>", true, ""},
		{"<bash-stdout></bash-stdout><bash-stderr></bash-stderr>", true, ""},
		{"Explain <bash-input>ls</bash-input>", false, ""},
		{"`<bash-input>ls</bash-input>`", false, ""},
		{"<bash-input>ls", false, ""},
		{"<bash-input>ls</bash-input> trailing", false, ""},
		{"<bash-input>a</bash-input><bash-input>b</bash-input>", false, ""},
		{"<bash-stdout>incomplete</bash-stdout>", false, ""},
		{"<bash-stderr>error</bash-stderr>", false, ""},
		{"<bash-stdout>a</bash-stdout> prose <bash-stderr>b</bash-stderr>", false, ""},
		{"<bash-stdout>a</bash-stdout><bash-stderr>b</bash-stderr> trailing", false, ""},
	} {
		t.Run(tt.text, func(t *testing.T) {
			got, ok := parseManualBash(tt.text)
			if ok != tt.ok || got.command != tt.command {
				t.Fatalf("parse = %+v, %t; want command %q, %t", got, ok, tt.command, tt.ok)
			}
		})
	}
	for _, raw := range []string{
		`[{"type":"text","text":"<bash-input>ls</bash-input>"},{"type":"image"}]`,
		`[{"type":"text","text":"<bash-input>ls</bash-input>"},{"type":"text","text":"explain this"}]`,
		`[{"type":"tool_result","content":"<bash-input>ls</bash-input>"}]`,
	} {
		if _, ok := manualBashContent(gjson.Parse(raw)); ok {
			t.Fatalf("mixed record recognized: %s", raw)
		}
	}
}

func TestManualBashMetadata(t *testing.T) {
	t.Parallel()
	for _, blocks := range []bool{false, true} {
		var cp checkpoint
		var d provider.Diagnostics
		consume := func(text string) {
			var content any = text
			if blocks {
				content = []any{map[string]string{"type": "text", "text": text}}
			}
			line, err := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"content": content}})
			if err != nil {
				t.Fatal(err)
			}
			cp.consume(line, &d)
		}
		cp.Meta.Turn = model.TurnDone
		consume("<bash-input> go test ./... </bash-input>")
		consume("<bash-stdout></bash-stdout><bash-stderr>FAIL</bash-stderr>")
		meta := cp.finalize(time.Time{})
		if meta.Title != "! go test ./..." || meta.FirstPrompt != "" || meta.Turn != model.TurnDone || meta.Counts.User != 2 {
			t.Fatalf("manual-only metadata: %+v", meta)
		}
		consume("Fix the test harness")
		cp.Meta.Turn = model.TurnTool
		cp.PendingTools = []string{"tool-1"}
		consume("<bash-input>printf '<xml>'</bash-input>")
		consume("<bash-stdout>ok</bash-stdout><bash-stderr></bash-stderr>")
		meta = cp.finalize(time.Time{})
		if meta.Title != "Fix the test harness" || meta.FirstPrompt != "Fix the test harness" || meta.Turn != model.TurnTool || len(cp.PendingTools) != 1 {
			t.Fatalf("conversation metadata: %+v", meta)
		}
		// Checkpoint round trips retain the manual-only fallback independently.
		raw, err := json.Marshal(cp)
		if err != nil {
			t.Fatal(err)
		}
		var restored checkpoint
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.LastBashCommand != cp.LastBashCommand || restored.finalize(time.Time{}).Title != meta.Title {
			t.Fatalf("checkpoint round trip: %+v", restored)
		}
		cp.AITitle = "AI title"
		if cp.finalize(time.Time{}).Title != "AI title" {
			t.Fatal("AI title lost precedence")
		}
		cp.CustomTitle = "My title"
		if cp.finalize(time.Time{}).Title != "My title" {
			t.Fatal("custom title lost precedence")
		}
	}
	var cp checkpoint
	var d provider.Diagnostics
	cp.consume([]byte(`{"type":"user","message":{"content":"<bash-stdout></bash-stdout><bash-stderr>noise</bash-stderr>"}}`), &d)
	if cp.finalize(time.Time{}).Title != "(untitled)" {
		t.Fatal("output-only session acquired a title")
	}
}

package claude

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
)

const completedNotification = `<task-notification>
<task-id>agent-123</task-id>
<tool-use-id>call-123</tool-use-id>
<output-file>/tmp/tasks/agent-123.output</output-file>
<status>completed</status>
<summary>Agent "Review setup wrapper safety" finished</summary>
<note>The same task-id may notify more than once.</note>
<result>Report delivered separately.

` + "```xml\n&lt;Directory name=\"A &amp; B\" /&gt;\n```" + `
</result>
<usage><subagent_tokens>35364</subagent_tokens><tool_uses>19</tool_uses><duration_ms>145812</duration_ms></usage>
</task-notification>`

func TestTaskNotificationText(t *testing.T) {
	t.Parallel()
	got, ok := taskNotificationText("\n" + completedNotification + "\n")
	if !ok {
		t.Fatal("notification not recognized")
	}
	for _, want := range []string{
		"**Summary**\n\nAgent \"Review setup wrapper safety\" finished",
		"**Status**\n\ncompleted", "**Result**\n\nReport delivered separately.",
		"**Note**\n\nThe same task-id may notify more than once.",
		"```xml\n<Directory name=\"A & B\" />\n```",
		"**Task ID**\n\nagent-123", "**Tool use ID**\n\ncall-123",
		"**Output file**\n\n/tmp/tasks/agent-123.output",
		"Subagent tokens: 35364 · Tool uses: 19 · Duration (ms): 145812",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "<task-notification>") {
		t.Fatal("raw envelope in formatted text")
	}
}

func TestTaskNotificationVariants(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"completed", "failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			text := "<task-notification><status>" + status + "</status><summary>Background command finished</summary></task-notification>"
			got, ok := taskNotificationText(text)
			if !ok || !strings.Contains(got, "**Status**\n\n"+status) || strings.Contains(got, "**Result**") {
				t.Fatalf("got %q, %v", got, ok)
			}
		})
	}
	for _, body := range []string{
		"<result>raw <tag> & text</result>",
		"<result>&amp;lt;version&amp;gt;</result>",
	} {
		got, ok := taskNotificationText("<task-notification>" + body + "</task-notification>")
		if !ok {
			t.Fatalf("not recognized: %s", body)
		}
		if strings.Contains(body, "&amp;lt;") && !strings.Contains(got, "&lt;version&gt;") {
			t.Fatalf("entities decoded more than once: %q", got)
		}
	}
	got, ok := taskNotificationText(completedNotification + "\n" + completedNotification)
	if !ok || strings.Count(got, "**Summary**") != 2 {
		t.Fatalf("repeated notifications lost: %q, %v", got, ok)
	}
}

func TestTaskNotificationFallback(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"", "Please explain " + completedNotification,
		"```xml\n" + completedNotification + "\n```",
		completedNotification + "\nMy question",
		strings.TrimSuffix(completedNotification, "</task-notification>"),
		"<task-notification><task-id>123</task-id></task-notification>",
		"<task-notification><summary>done</summary><unknown>keep me</unknown></task-notification>",
		"<task-notification><summary>one</summary><summary>two</summary></task-notification>",
		"<task-notification><result>code </result> example</result></task-notification>",
		"<task-notification><result>missing end</task-notification>",
		"<task-notification><summary>done</summary><usage><new>1</new></usage></task-notification>",
	} {
		if got, ok := taskNotificationText(text); ok || got != "" {
			t.Errorf("unexpected normalization of %q: %q", text, got)
		}
	}
}

func TestLoadTaskNotificationFixture(t *testing.T) {
	t.Parallel()
	tx := loadFixture(t, "task_notification")
	if len(tx.Messages) != 5 {
		t.Fatalf("messages = %d, want 5 (queue copies ignored)", len(tx.Messages))
	}
	for i, want := range []model.Role{model.RoleUser, model.RoleAssistant, model.RoleAssistant, model.RoleAssistant, model.RoleUser} {
		if tx.Messages[i].Role != want || tx.Messages[i].IsMeta {
			t.Errorf("message %d has role %s, meta %v", i, tx.Messages[i].Role, tx.Messages[i].IsMeta)
		}
	}
	if !strings.Contains(tx.Messages[2].Parts[0].Text, "No defects found.") || !strings.Contains(tx.Messages[3].Parts[0].Text, "finished again") {
		t.Fatal("repeated task notifications lost")
	}
}

func TestLoadTaskNotification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		content any
		meta    bool
		want    model.Role
	}{
		{"string", completedNotification, false, model.RoleAssistant},
		{"meta", completedNotification, true, model.RoleAssistant},
		{"blocks", []map[string]any{{"type": "text", "text": completedNotification}, {"type": "text", "text": ""}}, false, model.RoleAssistant},
		{"mixed prose", []map[string]any{{"type": "text", "text": completedNotification}, {"type": "text", "text": "My question"}}, false, model.RoleUser},
		{"mixed image", []map[string]any{{"type": "text", "text": completedNotification}, {"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aGVsbG8="}}}, false, model.RoleUser},
		{"mixed tool result", []map[string]any{{"type": "text", "text": completedNotification}, {"type": "tool_result", "tool_use_id": "call-1", "content": "result"}}, false, model.RoleUser},
		{"ordinary user", "Explain this notification", false, model.RoleUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var l loader
			// A pending call also verifies tool-result pairing in mixed records.
			l.record([]byte(`{"type":"assistant","message":{"id":"parent","content":[{"type":"tool_use","id":"call-1","name":"Bash","input":{}}]}}`), 0, 1)
			record := map[string]any{"type": "user", "timestamp": "2026-11-01T10:00:00Z", "isMeta": tc.meta, "isSidechain": true, "message": map[string]any{"role": "user", "content": tc.content}}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			l.record(data, 100, 2)
			if l.store.Len() != 2 {
				t.Fatalf("messages = %d", l.store.Len())
			}
			msg := l.store.At(1)
			if msg.Role != tc.want || !msg.IsSidechain || msg.Time.IsZero() {
				t.Fatalf("unexpected message: %+v", msg)
			}
			if tc.want == model.RoleAssistant && (msg.IsMeta || msg.Model != "" || msg.Tokens != (model.TokenUsage{})) {
				t.Fatalf("notification acquired model/meta/usage: %+v", msg)
			}
			if tc.name == "mixed tool result" && l.store.At(0).Parts[0].Tool.Output != "result" {
				t.Fatal("tool result pairing lost")
			}
			// A later chunk belongs to the original parent, not the notification.
			l.record([]byte(`{"type":"assistant","message":{"id":"parent","content":[{"type":"text","text":"Parent answer"}]}}`), 200, 3)
			if l.store.Len() != 2 || len(l.store.At(0).Parts) != 2 {
				t.Fatal("notification changed assistant coalescing")
			}
		})
	}
}

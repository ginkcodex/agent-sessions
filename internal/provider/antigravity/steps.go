package antigravity

import "strings"

// Transcript step types.
const (
	stepUserInput     = "USER_INPUT"
	stepPlanner       = "PLANNER_RESPONSE"
	stepGeneric       = "GENERIC"
	stepSystemMessage = "SYSTEM_MESSAGE"
	stepCheckpoint    = "CHECKPOINT"
	stepErrorMessage  = "ERROR_MESSAGE"
)

// userRequest is the prompt inside a USER_INPUT's <USER_REQUEST> envelope,
// without the <ADDITIONAL_METADATA> and <USER_SETTINGS_CHANGE> blocks agy
// appends. Content with no envelope is returned trimmed.
func userRequest(content string) string {
	if _, rest, ok := strings.Cut(content, "<USER_REQUEST>"); ok {
		inner, _, _ := strings.Cut(rest, "</USER_REQUEST>")
		return strings.TrimSpace(inner)
	}
	for _, tag := range []string{"ADDITIONAL_METADATA", "USER_SETTINGS_CHANGE"} {
		content = stripBlock(content, tag)
	}
	return strings.TrimSpace(content)
}

// stripBlock removes every <tag>...</tag> block from s.
func stripBlock(s, tag string) string {
	open, end := "<"+tag+">", "</"+tag+">"
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], end)
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+j+len(end):]
	}
}

// systemMessage is a parsed SYSTEM_MESSAGE step.
type systemMessage struct {
	// Sender is "system", a session ID (a parent's task or a child's report)
	// or "<session>/task-<step>" (a background task's result).
	Sender string
	Text   string
}

// smHeader starts the message proper, after agy's preamble that also names
// the <SYSTEM_MESSAGE> tag.
const smHeader = "<SYSTEM_MESSAGE>\n[Message] "

// parseSystemMessage reads the "[Message] timestamp=... sender=...
// priority=... content=<text>" line of a SYSTEM_MESSAGE's content.
func parseSystemMessage(content string) (systemMessage, bool) {
	i := strings.Index(content, smHeader)
	if i < 0 {
		return systemMessage{}, false
	}
	body := content[i+len(smHeader):]
	if j := strings.LastIndex(body, "</SYSTEM_MESSAGE>"); j >= 0 {
		body = body[:j]
	}
	head, text, ok := strings.Cut(body, " content=")
	if !ok {
		head, text = body, ""
	}
	var msg systemMessage
	for _, field := range strings.Fields(head) {
		if sender, ok := strings.CutPrefix(field, "sender="); ok {
			msg.Sender = sender
		}
	}
	msg.Text = strings.TrimSpace(text)
	return msg, true
}

// fromSession reports whether the sender is another session (a parent's task
// or a child's report), not the system or a background task.
func (m systemMessage) fromSession() bool {
	return m.Sender != "" && m.Sender != "system" && !strings.Contains(m.Sender, "/")
}

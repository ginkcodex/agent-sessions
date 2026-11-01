package claude

import (
	"html"
	"strings"
)

// notificationFields reads the harness's XML-like envelope without interpreting
// report Markdown as XML. Ambiguous delimiters and unknown fields retain the
// original message instead of silently discarding content.
func notificationFields(s string, allowed map[string]bool) (map[string]string, bool) {
	fields := make(map[string]string)
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		if !strings.HasPrefix(s, "<") {
			return nil, false
		}
		end := strings.IndexByte(s, '>')
		if end < 0 {
			return nil, false
		}
		name := s[1:end]
		if !allowed[name] {
			return nil, false
		}
		if _, exists := fields[name]; exists {
			return nil, false
		}
		closeTag := "</" + name + ">"
		body := s[end+1:]
		closeAt := strings.Index(body, closeTag)
		if closeAt < 0 || strings.Contains(body[closeAt+len(closeTag):], closeTag) {
			return nil, false
		}
		fields[name] = strings.TrimSpace(body[:closeAt])
		s = body[closeAt+len(closeTag):]
	}
	return fields, true
}

var taskNotificationFields = map[string]bool{
	"task-id": true, "tool-use-id": true, "output-file": true,
	"status": true, "summary": true, "note": true, "result": true, "usage": true,
}

var taskUsageFields = map[string]bool{
	"subagent_tokens": true, "tool_uses": true, "duration_ms": true,
}

// taskNotificationText recognizes only complete envelopes. Field values are
// entity-decoded once, after parsing, so escaped code cannot become delimiters.
func taskNotificationText(text string) (string, bool) {
	const openTag, closeTag = "<task-notification>", "</task-notification>"
	var notifications []string
	for s := strings.TrimSpace(text); s != ""; {
		if !strings.HasPrefix(s, openTag) {
			return "", false
		}
		s = s[len(openTag):]
		end := strings.Index(s, closeTag)
		if end < 0 || strings.Contains(s[:end], openTag) {
			return "", false
		}
		fields, ok := notificationFields(s[:end], taskNotificationFields)
		if !ok {
			return "", false
		}
		var sections []string
		for _, field := range []struct{ name, label string }{
			{"summary", "Summary"}, {"status", "Status"}, {"result", "Result"}, {"note", "Note"},
		} {
			if value := fields[field.name]; value != "" {
				sections = append(sections, "**"+field.label+"**\n\n"+html.UnescapeString(value))
			}
		}
		if len(sections) == 0 {
			return "", false
		}
		for _, field := range []struct{ name, label string }{
			{"task-id", "Task ID"}, {"tool-use-id", "Tool use ID"}, {"output-file", "Output file"},
		} {
			if value := fields[field.name]; value != "" {
				sections = append(sections, "**"+field.label+"**\n\n"+html.UnescapeString(value))
			}
		}
		if usage, exists := fields["usage"]; exists {
			values, ok := notificationFields(usage, taskUsageFields)
			if !ok {
				return "", false
			}
			var details []string
			for _, field := range []struct{ name, label string }{
				{"subagent_tokens", "Subagent tokens"}, {"tool_uses", "Tool uses"}, {"duration_ms", "Duration (ms)"},
			} {
				if value := values[field.name]; value != "" {
					details = append(details, field.label+": "+html.UnescapeString(value))
				}
			}
			if len(details) > 0 {
				sections = append(sections, "**Usage**\n\n"+strings.Join(details, " · "))
			}
		}
		notifications = append(notifications, strings.Join(sections, "\n\n"))
		s = strings.TrimSpace(s[end+len(closeTag):])
	}
	return strings.Join(notifications, "\n\n---\n\n"), len(notifications) > 0
}

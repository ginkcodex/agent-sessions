package claude

import (
	"html"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// notificationFields reads the harness's XML-like envelope without interpreting
// report Markdown as XML. Ambiguous delimiters and unknown fields retain the
// original message instead of silently discarding content. Only the fields in
// repeatable may appear more than once; their values are joined with "\n".
func notificationFields(s string, allowed, repeatable map[string]bool) (map[string]string, bool) {
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
		_, exists := fields[name]
		if exists && !repeatable[name] {
			return nil, false
		}
		closeTag := "</" + name + ">"
		body := s[end+1:]
		closeAt := strings.Index(body, closeTag)
		if closeAt < 0 {
			return nil, false
		}
		// A second closing tag makes the end ambiguous. For a field that may
		// repeat, only up to its next opening tag: later ones close the repeats.
		rest := body[closeAt+len(closeTag):]
		if next := strings.Index(rest, "<"+name+">"); repeatable[name] && next >= 0 {
			rest = rest[:next]
		}
		if strings.Contains(rest, closeTag) {
			return nil, false
		}
		value := strings.TrimSpace(body[:closeAt])
		if exists {
			value = fields[name] + "\n" + value
		}
		fields[name] = value
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

// A notification about tasks a previous run left behind lists one task-id
// per task.
var taskRepeatableFields = map[string]bool{"task-id": true}

// orphanMarker starts the internal ID the harness adds to an orphaned-tasks
// summary; its note says such IDs are scan markers, not tasks.
const orphanMarker = "__orphan_summary"

// taskIDs is the task IDs of a notification in order, decoded, without
// duplicates or orphan-summary markers.
func taskIDs(field string) []string {
	var ids []string
	seen := make(map[string]bool)
	for _, id := range strings.Split(field, "\n") {
		id = strings.TrimSpace(html.UnescapeString(id))
		if id == "" || seen[id] || strings.HasPrefix(id, orphanMarker) {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

// taskNotificationParts recognizes only complete envelopes. Field values are
// entity-decoded once, after parsing, so escaped code cannot become delimiters.
// Report text stays visible in Chat; diagnostic notices are Activity/All only.
func taskNotificationParts(text string) ([]model.Part, bool) {
	const openTag, closeTag = "<task-notification>", "</task-notification>"
	var parts []model.Part
	for s := strings.TrimSpace(text); s != ""; {
		if !strings.HasPrefix(s, openTag) {
			return nil, false
		}
		s = s[len(openTag):]
		end := strings.Index(s, closeTag)
		if end < 0 || strings.Contains(s[:end], openTag) {
			return nil, false
		}
		fields, ok := notificationFields(s[:end], taskNotificationFields, taskRepeatableFields)
		if !ok {
			return nil, false
		}
		var sections []string
		for _, field := range []struct{ name, label string }{
			{"summary", "Summary"}, {"result", "Result"}, {"note", "Note"},
		} {
			if value := fields[field.name]; value != "" {
				sections = append(sections, "**"+field.label+"**\n\n"+html.UnescapeString(value))
			}
		}
		if len(sections) == 0 && fields["status"] == "" {
			return nil, false
		}
		if len(sections) > 0 {
			if len(parts) > 0 {
				sections[0] = "---\n\n" + sections[0]
			}
			parts = append(parts, model.Part{Kind: model.PartText, Text: strings.Join(sections, "\n\n")})
		}
		sections = nil
		if value := fields["status"]; value != "" {
			sections = append(sections, "**Status**\n\n"+html.UnescapeString(value))
		}
		if ids := taskIDs(fields["task-id"]); len(ids) == 1 {
			sections = append(sections, "**Task ID**\n\n"+ids[0])
		} else if len(ids) > 1 {
			sections = append(sections, "**Task IDs**\n\n"+strings.Join(ids, ", "))
		}
		for _, field := range []struct{ name, label string }{
			{"tool-use-id", "Tool use ID"}, {"output-file", "Output file"},
		} {
			if value := fields[field.name]; value != "" {
				sections = append(sections, "**"+field.label+"**\n\n"+html.UnescapeString(value))
			}
		}
		if usage, exists := fields["usage"]; exists {
			values, ok := notificationFields(usage, taskUsageFields, nil)
			if !ok {
				return nil, false
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
		if len(sections) > 0 {
			parts = append(parts, model.Part{Kind: model.PartNotice, Text: strings.Join(sections, "\n\n")})
		}
		s = strings.TrimSpace(s[end+len(closeTag):])
	}
	return parts, len(parts) > 0
}

package antigravity

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

// maxOutputBytes is the tool output shown inline; Blob serves the rest.
const maxOutputBytes = 64 << 10

// step is one transcript line, with the fields the transcript shows.
type step struct {
	Index        int64  `json:"step_index"`
	Type         string `json:"type"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	Content      string `json:"content"`
	Error        string `json:"error"`
	Model        string `json:"model"`
	Thinking     string `json:"thinking"`
	InputTokens  int64  `json:"input_tokens"`
	CacheRead    int64  `json:"cache_read_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	ToolCalls    []struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
	} `json:"tool_calls"`
}

// loader builds messages from transcript steps. Tools are located by index,
// so a later result copies rather than mutates a published message.
type loader struct {
	store transcriptbuild.Builder
	// waiting maps the step index a result is expected at to its call.
	waiting map[int64]transcriptbuild.ToolLoc
	exists  func(id string) bool // whether a child session's transcript exists
}

// fork copies parser state for a disposable best-effort tail.
func (l *loader) fork() *loader {
	f := *l
	f.store = *l.store.Fork()
	f.waiting = maps.Clone(l.waiting)
	return &f
}

// step adds one line at byte offset start. Invalid lines are the scan's to
// report; unknown step types are skipped.
func (l *loader) step(line []byte, start int64) {
	var s step
	if json.Unmarshal(line, &s) != nil {
		return
	}
	l.expire(s.Index)
	stamp, _ := time.Parse(time.RFC3339Nano, s.CreatedAt)
	stamp = stamp.UTC()
	id := fmt.Sprintf("step-%d", s.Index)
	switch s.Type {
	case stepUserInput:
		l.store.Append(model.Message{ID: id, Role: model.RoleUser, Time: stamp, Parts: []model.Part{{Kind: model.PartText, Text: userRequest(s.Content)}}})
	case stepPlanner:
		msg := model.Message{
			ID: id, Role: model.RoleAssistant, Time: stamp, Model: s.Model,
			Tokens: model.TokenUsage{Input: s.InputTokens, Output: s.OutputTokens, CacheRead: s.CacheRead},
		}
		if s.Thinking != "" {
			msg.Parts = append(msg.Parts, model.Part{Kind: model.PartReasoning, Text: s.Thinking})
		}
		if s.Content != "" {
			msg.Parts = append(msg.Parts, model.Part{Kind: model.PartText, Text: s.Content})
		}
		first := len(msg.Parts)
		for k, call := range s.ToolCalls {
			var input json.RawMessage
			if len(call.Args) != 0 && string(call.Args) != "null" {
				input = call.Args
			}
			msg.Parts = append(msg.Parts, model.Part{Kind: model.PartTool, Tool: &model.ToolCall{
				ID: fmt.Sprintf("%s-%d", id, k), Name: call.Name, Input: input, Status: model.ToolPending,
			}})
		}
		i := l.store.Append(msg)
		for k := range s.ToolCalls {
			if l.waiting == nil {
				l.waiting = make(map[int64]transcriptbuild.ToolLoc)
			}
			// The result of call k is step index+1+k, whatever lies between.
			l.waiting[s.Index+1+int64(k)] = transcriptbuild.ToolLoc{Message: i, Part: first + k}
		}
	case stepGeneric:
		loc, ok := l.waiting[s.Index]
		if !ok {
			text := model.TruncateUTF8(resultText(s), maxOutputBytes)
			l.store.Append(model.Message{ID: id, Role: model.RoleSystem, Time: stamp, IsMeta: true, Parts: []model.Part{{Kind: model.PartNotice, Text: text}}})
			return
		}
		delete(l.waiting, s.Index)
		call := l.store.Tool(loc)
		out := resultText(s)
		if call.Name == "invoke_subagent" {
			if m := childRE.FindStringSubmatch(out); m != nil && l.exists != nil && l.exists(m[1]) {
				call.Child = &model.SessionRef{Agent: model.AgentAntigravity, ID: m[1]}
			}
		}
		if len(out) > maxOutputBytes {
			out = model.TruncateUTF8(out, maxOutputBytes)
			call.OutputTruncated = true
			call.OutputRef = fmt.Sprintf("rec:%d:%d", start, s.Index)
		}
		call.Output = out
		switch s.Status {
		case "DONE":
			call.Status = model.ToolCompleted
		case "ERROR":
			call.Status = model.ToolError
		case "RUNNING":
			call.Status = model.ToolPending // a background task, still running
		default:
			call.Status = model.ToolUnknown
		}
	}
}

// expire fails the calls whose result step was passed over: a denied call
// may write none.
func (l *loader) expire(index int64) {
	for at, loc := range l.waiting {
		if at < index {
			l.store.Tool(loc).Status = model.ToolError
			delete(l.waiting, at)
		}
	}
}

// childRE finds the session invoke_subagent created; agy pads the colon.
var childRE = regexp.MustCompile(`"conversationId":\s*"([0-9a-fA-F-]{36})"`)

// resultText is a GENERIC step's output without agy's leading "Created At:"
// and "Completed At:" lines, or its error when there is no other text.
func resultText(s step) string {
	out := strings.ReplaceAll(s.Content, "\r\n", "\n")
	for {
		line, rest, _ := strings.Cut(out, "\n")
		if !strings.HasPrefix(line, "Created At: ") && !strings.HasPrefix(line, "Completed At: ") {
			break
		}
		out = rest
	}
	if strings.TrimSpace(out) == "" && s.Error != "" {
		return strings.ReplaceAll(s.Error, "\r\n", "\n")
	}
	return out
}

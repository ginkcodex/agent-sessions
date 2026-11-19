package antigravity

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

const (
	// maxOutputBytes is the tool output shown inline; Blob serves the rest.
	maxOutputBytes     = 64 << 10
	maxCompactionRunes = 4096
)

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
	id    string // the session's own ID, which its task senders start with
	store transcriptbuild.Builder
	// waiting maps the step index a result is expected at to its call.
	waiting map[int64]transcriptbuild.ToolLoc
	// running maps a background task's step to its call, children a child
	// session to the invoke_subagent call that started it.
	running  map[int64]transcriptbuild.ToolLoc
	children map[string]transcriptbuild.ToolLoc
	// refs is the Blob key of each call whose output later messages extend.
	refs     map[transcriptbuild.ToolLoc]string
	prompted bool                 // a user prompt or a subagent's task was seen
	exists   func(id string) bool // whether a child session's transcript exists
}

// fork copies parser state for a disposable best-effort tail.
func (l *loader) fork() *loader {
	f := *l
	f.store = *l.store.Fork()
	f.waiting = maps.Clone(l.waiting)
	f.running = maps.Clone(l.running)
	f.children = maps.Clone(l.children)
	f.refs = maps.Clone(l.refs)
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
	notice := func(text string) {
		l.store.Append(model.Message{ID: id, Role: model.RoleSystem, Time: stamp, IsMeta: true, Parts: []model.Part{{Kind: model.PartNotice, Text: model.TruncateUTF8(text, maxOutputBytes)}}})
	}
	switch s.Type {
	case stepUserInput:
		l.prompted = true
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
			// The result of call k is step index+1+k, whatever lies between.
			put(&l.waiting, s.Index+1+int64(k), transcriptbuild.ToolLoc{Message: i, Part: first + k})
		}
	case stepGeneric:
		loc, ok := l.waiting[s.Index]
		if !ok {
			notice(resultText(s))
			return
		}
		delete(l.waiting, s.Index)
		call := l.store.Tool(loc)
		out := resultText(s)
		ref := segment(start, s.Index)
		if call.Name == "invoke_subagent" {
			if m := childRE.FindStringSubmatch(out); m != nil {
				put(&l.children, m[1], loc)
				put(&l.refs, loc, ref)
				if l.exists != nil && l.exists(m[1]) {
					call.Child = &model.SessionRef{Agent: model.AgentAntigravity, ID: m[1]}
				}
			}
		}
		setOutput(call, out, ref)
		switch s.Status {
		case "DONE":
			call.Status = model.ToolCompleted
		case "ERROR":
			call.Status = model.ToolError
		case "RUNNING":
			call.Status = model.ToolPending // a background task: a message completes it
			put(&l.running, s.Index, loc)
		default:
			call.Status = model.ToolUnknown
		}
	case stepSystemMessage:
		msg, ok := parseSystemMessage(s.Content)
		if !ok {
			notice(strings.TrimSpace(s.Content))
			return
		}
		text := systemText(msg)
		if task, ok := l.taskStep(msg.Sender); ok {
			loc, running := l.running[task]
			out, done := taskResult(msg)
			if !running || !done {
				notice(text)
				return
			}
			delete(l.running, task)
			ref := segment(start, s.Index)
			put(&l.refs, loc, ref)
			call := l.store.Tool(loc)
			setOutput(call, out, ref)
			call.Status = model.ToolCompleted
			return
		}
		if !msg.fromSession() {
			notice(text)
			return
		}
		if !l.prompted {
			// A subagent's transcript opens with its task from the parent.
			l.prompted = true
			l.store.Append(model.Message{ID: id, Role: model.RoleUser, Time: stamp, Parts: []model.Part{{Kind: model.PartText, Text: text}}})
			return
		}
		loc, ok := l.children[msg.Sender]
		if !ok {
			notice(text)
			return
		}
		// A child's report extends the output of the call that started it.
		ref := l.refs[loc] + "," + strings.TrimPrefix(segment(start, s.Index), "rec:")
		l.refs[loc] = ref
		call := l.store.Tool(loc)
		truncated := call.OutputTruncated
		setOutput(call, call.Output+"\n\n"+text, ref)
		call.OutputTruncated = call.OutputTruncated || truncated
		if call.OutputTruncated {
			call.OutputRef = ref
		}
	case stepCheckpoint:
		l.store.Append(model.Message{ID: id, Role: model.RoleSystem, Time: stamp, IsMeta: true, Parts: []model.Part{{Kind: model.PartCompaction, Text: model.TruncateRunes(strings.TrimSpace(s.Content), maxCompactionRunes)}}})
	case stepErrorMessage:
		notice(strings.TrimSpace(s.Content + s.Error))
	}
}

// put sets m[k], making the map on first use.
func put[K comparable, V any](m *map[K]V, k K, v V) {
	if *m == nil {
		*m = make(map[K]V)
	}
	(*m)[k] = v
}

// segment is the Blob key of one step's output; a key may join several.
func segment(start, index int64) string { return fmt.Sprintf("rec:%d:%d", start, index) }

// setOutput shows out, clipped to the inline limit with ref serving it all.
func setOutput(call *model.ToolCall, out, ref string) {
	call.Output, call.OutputTruncated, call.OutputRef = out, false, ""
	if len(out) > maxOutputBytes {
		call.Output = model.TruncateUTF8(out, maxOutputBytes)
		call.OutputTruncated, call.OutputRef = true, ref
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

// taskStep is the step of a background task sender "<own id>/task-<step>".
func (l *loader) taskStep(sender string) (int64, bool) {
	session, task, ok := strings.Cut(sender, "/task-")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(task, 10, 64)
	return n, err == nil && session == l.id
}

// childRE finds the session invoke_subagent created; agy pads the colon.
var childRE = regexp.MustCompile(`"conversationId":\s*"([0-9a-fA-F-]{36})"`)

// stripHeaders drops agy's leading "Created At:" and "Completed At:" lines.
func stripHeaders(out string) string {
	for {
		line, rest, _ := strings.Cut(out, "\n")
		if !strings.HasPrefix(line, "Created At: ") && !strings.HasPrefix(line, "Completed At: ") {
			return out
		}
		out = rest
	}
}

// resultText is a GENERIC step's output without its header lines, or its
// error when there is no other text.
func resultText(s step) string {
	out := stripHeaders(strings.ReplaceAll(s.Content, "\r\n", "\n"))
	if strings.TrimSpace(out) == "" && s.Error != "" {
		return strings.ReplaceAll(s.Error, "\r\n", "\n")
	}
	return out
}

func systemText(msg systemMessage) string { return strings.ReplaceAll(msg.Text, "\r\n", "\n") }

// taskResult is the output a background task's message reports:
// `Task id "<sender>" finished with result:` and the output.
func taskResult(msg systemMessage) (string, bool) {
	out, ok := strings.CutPrefix(systemText(msg), `Task id "`+msg.Sender+`" finished with result:`)
	if !ok {
		return "", false
	}
	return stripHeaders(strings.TrimPrefix(out, "\n")), true
}

// stepOutput is the output a Blob key segment names: a GENERIC result, a
// background task's result or a child's report.
func stepOutput(s step) (string, bool) {
	switch s.Type {
	case stepGeneric:
		return resultText(s), true
	case stepSystemMessage:
		msg, ok := parseSystemMessage(s.Content)
		if !ok {
			return "", false
		}
		if out, ok := taskResult(msg); ok {
			return out, true
		}
		return systemText(msg), msg.fromSession()
	}
	return "", false
}

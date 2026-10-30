package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

const (
	maxOutputBytes     = 64 << 10
	maxCompactionRunes = 4096
)

// rolloutFor uses Scan's canonical duplicate/index selection, rather than
// manufacturing a path from untrusted ref.ID. It also keeps Load.Meta identical
// to the scan metadata for an unchanged source. The resolver continues its
// previous private scan, so only changed rollouts are read.
func (p *Provider) rolloutFor(ctx context.Context, ref model.SessionRef) (model.SessionMeta, error) {
	if ref.Agent != model.AgentCodex || ref.ID == "" {
		return model.SessionMeta{}, fmt.Errorf("codex: invalid session reference (%q, %q)", ref.Agent, ref.ID)
	}
	if err := ctx.Err(); err != nil {
		return model.SessionMeta{}, err
	}
	meta, ok, err := p.resolver.resolve(ctx, p, ref)
	if err != nil {
		return model.SessionMeta{}, fmt.Errorf("codex: resolve session %q: %w", ref.ID, err)
	}
	if !ok {
		return model.SessionMeta{}, fmt.Errorf("codex: rollout for session %q not found (source may have vanished after scan)", ref.ID)
	}
	// Discovery/index validation precedes opening. Check again in case the
	// source was replaced between scan and load.
	if _, err := p.indexRolloutPath(meta.SourcePath); err != nil {
		return model.SessionMeta{}, fmt.Errorf("codex: rollout for session %q vanished or changed after scan: %w", ref.ID, err)
	}
	return meta, nil
}

// Load reconstructs conversation records from a validated rollout path.
func (p *Provider) Load(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	result, err := p.LoadIncremental(ctx, ref, nil)
	return result.Transcript, err
}

type rolloutRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	// Historic envelope-less message shape (only role/content are accepted).
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	ID      string          `json:"id"`
	CWD     string          `json:"cwd"`
}

type responseItem struct {
	Type             string          `json:"type"`
	ID               string          `json:"id"`
	MessageID        string          `json:"message_id"`
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	Summary          json.RawMessage `json:"summary"`
	CallID           string          `json:"call_id"`
	Name             string          `json:"name"`
	Arguments        json.RawMessage `json:"arguments"`
	Input            json.RawMessage `json:"input"`
	Output           json.RawMessage `json:"output"`
	Status           string          `json:"status"`
	EncryptedContent json.RawMessage `json:"encrypted_content"` // deliberately never displayed
}

type contentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Filename string          `json:"filename"`
	ImageURL json.RawMessage `json:"image_url"`
	FileData json.RawMessage `json:"file_data"`
	FileURL  json.RawMessage `json:"file_url"`
	FileID   json.RawMessage `json:"file_id"`
}

type rolloutLoader struct {
	path      string
	store     transcriptbuild.Builder
	tools     map[string]transcriptbuild.ToolLoc
	assistant map[string]int
	current   int // assistant index; -1 after user/system/compaction
	model     string
	diag      *provider.Diagnostics // the current pass only
}

// loadRollout returns diagnostics for package tests. Public Load returns only
// the transcript; scan diagnostics remain in ScanResult.Diag.
func (p *Provider) loadRollout(ctx context.Context, meta model.SessionMeta) (*model.Transcript, provider.Diagnostics, error) {
	result, diag, err := p.loadRolloutIncremental(ctx, meta, nil)
	if err != nil {
		return nil, provider.Diagnostics{}, err
	}
	return result.Transcript, *diag, nil
}

func (l *rolloutLoader) record(line []byte, start int64, lineNo int) {
	var rec rolloutRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		l.diag.ParseErrors++
		l.diag.Warn(l.path, lineNo, "invalid JSON record: %v", err)
		return
	}
	stamp := time.Time{}
	if rec.Timestamp != "" {
		if t, err := time.Parse(time.RFC3339Nano, rec.Timestamp); err == nil {
			stamp = t.UTC()
		} else {
			l.diag.Warn(l.path, lineNo, "invalid timestamp %q", rec.Timestamp)
		}
	}
	switch rec.Type {
	case "response_item":
		if len(rec.Payload) == 0 || string(rec.Payload) == "null" {
			l.malformed(lineNo, "missing response item payload")
			return
		}
		l.response(rec.Payload, stamp, start, lineNo)
	case "event_msg":
		l.event(rec.Payload, stamp, start, lineNo)
	case "turn_context":
		var contextItem struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(rec.Payload, &contextItem); err == nil && contextItem.Model != "" {
			l.model = contextItem.Model
		}
	case "compacted":
		l.compacted(rec.Payload, stamp, start)
	case "session_meta", "inter_agent_communication", "inter_agent_communication_metadata",
		"token_usage_record", "world_state", "retained_context", "security_risk_score", "realtime_item":
		// Scanner owns metadata; these are not conversation messages.
	case "":
		if rec.Role != "" && len(rec.Content) != 0 { // recognized historic message form
			l.message(responseItem{Role: rec.Role, Content: rec.Content, ID: rec.ID}, stamp, start, lineNo)
		} else if rec.ID == "" || rec.CWD == "" { // historic session_meta id/cwd only
			l.malformed(lineNo, "unrecognized envelope-less rollout record")
		}
	default:
		l.diag.Unknown(rec.Type)
	}
}

func (l *rolloutLoader) response(raw json.RawMessage, stamp time.Time, start int64, lineNo int) {
	var item responseItem
	if err := json.Unmarshal(raw, &item); err != nil {
		l.malformed(lineNo, "invalid response item: %v", err)
		return
	}
	switch item.Type {
	case "message":
		l.message(item, stamp, start, lineNo)
	case "reasoning":
		l.reasoning(item, stamp, start, lineNo)
	case "function_call", "custom_tool_call", "local_shell_call", "web_search_call":
		l.tool(item, stamp, start)
	case "function_call_output", "custom_tool_call_output", "local_shell_call_output", "web_search_call_output":
		l.toolOutput(item, stamp, start, lineNo)
	default:
		l.diag.Unknown("response_item:" + item.Type)
	}
}

func (l *rolloutLoader) appendMessage(role model.Role, stamp time.Time, id string, parts []model.Part, meta bool) int {
	return l.store.Append(model.Message{ID: id, Role: role, Time: stamp, Model: l.model, Parts: parts, IsMeta: meta})
}

// addParts appends to a message, copying it first if a snapshot can see it.
func (l *rolloutLoader) addParts(i int, parts ...model.Part) {
	m := l.store.Touch(i)
	m.Parts = append(m.Parts, parts...)
}

func recordID(id string, start int64) string {
	if id != "" {
		return id
	}
	return "rec:" + strconv.FormatInt(start, 10)
}

func (l *rolloutLoader) message(item responseItem, stamp time.Time, start int64, lineNo int) {
	var role model.Role
	switch item.Role {
	case "user":
		role = model.RoleUser
	case "assistant":
		role = model.RoleAssistant
	case "developer", "system":
		role = model.RoleSystem
	default:
		l.malformed(lineNo, "response message lacks recognized role %q", item.Role)
		return
	}
	parts, meta := l.content(item.Content, start, lineNo)
	if item.Role == "developer" {
		meta = true
	}
	id := item.ID
	if id == "" {
		id = item.MessageID
	}
	if role == model.RoleAssistant && id != "" {
		if idx, ok := l.assistant[id]; ok {
			// Split continuations append; identical replays do not.
			if len(parts) > 0 && !reflect.DeepEqual(l.store.At(idx).Parts, parts) {
				l.addParts(idx, parts...)
			}
			l.current = idx
			return
		}
	}
	idx := l.appendMessage(role, stamp, recordID(id, start), parts, meta)
	if role == model.RoleAssistant {
		l.current = idx
		if id != "" {
			if l.assistant == nil {
				l.assistant = make(map[string]int)
			}
			l.assistant[id] = idx
		}
	} else {
		l.current = -1
	}
}

func (l *rolloutLoader) content(raw json.RawMessage, start int64, lineNo int) ([]model.Part, bool) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []model.Part{{Kind: model.PartText, Text: text}}, isInjectedContext(text)
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		l.malformed(lineNo, "invalid message content: %v", err)
		return nil, false
	}
	parts := make([]model.Part, 0, len(blocks))
	meta, normal := false, false
	for i, rawBlock := range blocks {
		var block contentPart
		if err := json.Unmarshal(rawBlock, &block); err != nil {
			l.malformed(lineNo, "invalid content block %d: %v", i, err)
			continue
		}
		switch block.Type {
		case "input_text", "output_text", "text", "reasoning_text", "summary_text", "":
			if block.Text != "" {
				kind := model.PartText
				if block.Type == "reasoning_text" || block.Type == "summary_text" {
					kind = model.PartReasoning
				}
				parts = append(parts, model.Part{Kind: kind, Text: block.Text})
				if isInjectedContext(block.Text) {
					meta = true
				} else {
					normal = true
				}
			}
		case "input_image", "image", "output_image", "input_file", "file", "output_file":
			file := &model.FileRef{Name: block.Filename, Ref: fmt.Sprintf("rec:%d:%d", start, i)}
			file.Mime = dataMIME(block.ImageURL)
			if file.Mime == "" {
				file.Mime = dataMIME(block.FileData)
			}
			parts = append(parts, model.Part{Kind: model.PartFile, File: file})
			normal = true
		default:
			l.diag.Unknown("content:" + block.Type)
		}
	}
	return parts, meta && !normal
}

// assistantTurn is the index of the current assistant, starting one if needed.
func (l *rolloutLoader) assistantTurn(stamp time.Time, start int64) int {
	if l.current < 0 {
		l.current = l.appendMessage(model.RoleAssistant, stamp, recordID("", start), nil, false)
	}
	return l.current
}

func (l *rolloutLoader) reasoning(item responseItem, stamp time.Time, start int64, lineNo int) {
	var blocks []struct {
		Text string `json:"text"`
	}
	if len(item.Summary) != 0 && string(item.Summary) != "null" {
		if err := json.Unmarshal(item.Summary, &blocks); err != nil {
			l.malformed(lineNo, "invalid reasoning summary: %v", err)
		}
	}
	for _, b := range blocks {
		if b.Text != "" {
			l.addParts(l.assistantTurn(stamp, start), model.Part{Kind: model.PartReasoning, Text: b.Text})
		}
	}
	if len(item.Content) == 0 || string(item.Content) == "null" {
		return
	}
	parts, _ := l.content(item.Content, start, lineNo)
	for _, part := range parts {
		if part.Kind == model.PartText || part.Kind == model.PartReasoning {
			part.Kind = model.PartReasoning
			l.addParts(l.assistantTurn(stamp, start), part)
		}
	}
	// encrypted_content intentionally never decoded or rendered.
}

func (l *rolloutLoader) tool(item responseItem, stamp time.Time, start int64) {
	id := item.CallID
	if id == "" {
		id = item.ID
	}
	if id == "" {
		id = recordID("", start)
	}
	if _, exists := l.tools[id]; exists {
		return
	}
	name := item.Name
	if name == "" {
		name = item.Type
	}
	input := item.Arguments
	if len(input) == 0 {
		input = item.Input
	}
	// Preserve raw JSON (including integer precision and string-encoded JSON)
	// without decoding large arguments into float64 or duplicating them as text.
	call := &model.ToolCall{ID: id, Name: name, Input: input, Status: model.ToolPending}
	turn := l.assistantTurn(stamp, start)
	l.addParts(turn, model.Part{Kind: model.PartTool, Tool: call})
	if l.tools == nil {
		l.tools = make(map[string]transcriptbuild.ToolLoc)
	}
	l.tools[id] = transcriptbuild.ToolLoc{Message: turn, Part: len(l.store.At(turn).Parts) - 1}
}

func (l *rolloutLoader) toolOutput(item responseItem, stamp time.Time, start int64, lineNo int) {
	id := item.CallID
	if id == "" {
		id = item.ID
	}
	loc, ok := l.tools[id]
	if !ok || id == "" {
		l.diag.Warn(l.path, lineNo, "tool output for unknown call %q", id)
		l.appendMessage(model.RoleSystem, stamp, recordID("", start), []model.Part{{Kind: model.PartNotice, Text: "Tool output for unknown call"}}, true)
		l.current = -1
		return
	}
	call := l.store.Tool(loc)
	out := outputText(item.Output)
	if len(out) > maxOutputBytes {
		cut := maxOutputBytes
		for cut > 0 && !utf8.ValidString(out[:cut]) {
			cut--
		}
		call.Output = out[:cut]
		call.OutputTruncated = true
		call.OutputRef = fmt.Sprintf("rec:%d:%s", start, id)
	} else {
		call.Output = out
	}
	if item.Status == "failed" || item.Status == "error" {
		call.Status = model.ToolError
	} else {
		call.Status = model.ToolCompleted
	}
}

func outputText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	// Some tool outputs are structured; retain their exact JSON, but never
	// expose more than the inline output cap in the transcript.
	if json.Valid(raw) && string(raw) != "null" {
		return string(raw)
	}
	return ""
}

func (l *rolloutLoader) compacted(raw json.RawMessage, stamp time.Time, start int64) {
	var comp struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &comp) // replacement_history is intentionally ignored
	l.appendMessage(model.RoleSystem, stamp, recordID("", start), []model.Part{{Kind: model.PartCompaction, Text: model.TruncateRunes(comp.Message, maxCompactionRunes)}}, true)
	l.current = -1
}

func (l *rolloutLoader) event(raw json.RawMessage, stamp time.Time, start int64, lineNo int) {
	var event struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		l.malformed(lineNo, "invalid event message: %v", err)
		return
	}
	// user_message/agent_message duplicate response_item messages. Only
	// notices without a response-item counterpart are displayed.
	switch event.Type {
	case "turn_aborted", "error":
		text := event.Type
		if event.Message != "" {
			text += ": " + model.TruncateRunes(event.Message, maxCompactionRunes)
		}
		l.appendMessage(model.RoleSystem, stamp, recordID("", start), []model.Part{{Kind: model.PartNotice, Text: text}}, true)
		l.current = -1
	case "user_message", "agent_message", "token_count", "agent_reasoning", "agent_reasoning_raw_content", "agent_reasoning_section_break", "task_started", "task_complete", "turn_complete":
		// Token usage is already taken from the scanner's latest cumulative event.
	default:
		l.diag.Unknown("event_msg:" + event.Type)
	}
}

func (l *rolloutLoader) malformed(lineNo int, format string, args ...any) {
	l.diag.ParseErrors++
	l.diag.Warn(l.path, lineNo, format, args...)
}

// dataMIME reads only a short prefix of an encoded data URL. Neither image
// bytes nor data URLs are ever placed in a transcript.
func dataMIME(raw json.RawMessage) string {
	if !strings.HasPrefix(string(raw), `"data:`) {
		return ""
	}
	prefix := string(raw)
	if len(prefix) > 256 {
		prefix = prefix[:256]
	}
	mime, _, ok := strings.Cut(prefix[6:], ";")
	if ok && len(mime) < 128 {
		return mime
	}
	return ""
}

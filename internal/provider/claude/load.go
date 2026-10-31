package claude

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ginkcode/agent-sessions/internal/jsonl"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	// maxOutputBytes is the largest tool output inlined into a transcript.
	// Larger outputs are truncated; Blob returns the full text.
	maxOutputBytes = 64 << 10
	// maxAttachmentRunes caps the rendered text of attachment records.
	maxAttachmentRunes = 4096
	// toolResultsDir holds large tool outputs next to the session file.
	toolResultsDir = "tool-results"
	// toolResultsMarker appears in tool output text that references a
	// file in the tool-results directory; Windows writes backslashes.
	toolResultsMarker        = "/" + toolResultsDir + "/"
	toolResultsMarkerWindows = `\` + toolResultsDir + `\`
)

// rawRecord is the narrow decoding of one transcript line.
type rawRecord struct {
	Type             string          `json:"type"`
	UUID             string          `json:"uuid"`
	Timestamp        json.RawMessage `json:"timestamp"`
	IsMeta           bool            `json:"isMeta"`
	IsSidechain      bool            `json:"isSidechain"`
	IsCompactSummary bool            `json:"isCompactSummary"`
	Subtype          string          `json:"subtype"`
	Content          string          `json:"content"` // system records
	CompactMetadata  *compactMeta    `json:"compactMetadata"`
	Message          *rawMessage     `json:"message"`
	Attachment       *attachmentRec  `json:"attachment"`
	ToolUseResult    json.RawMessage `json:"toolUseResult"` // not decoded in M0
	IsAPIError       bool            `json:"isApiErrorMessage"`
}

type compactMeta struct {
	Trigger    string `json:"trigger"`
	PreTokens  int64  `json:"preTokens"`
	PostTokens int64  `json:"postTokens"`
}

type attachmentRec struct {
	Type     string `json:"type"`
	Rendered string `json:"rendered"`
}

type rawMessage struct {
	ID      string          `json:"id"`
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"` // string | []block
	Usage   *rawUsage       `json:"usage"`
}

type rawUsage struct {
	InputTokens         int64                `json:"input_tokens"`
	OutputTokens        int64                `json:"output_tokens"`
	CacheReadTokens     int64                `json:"cache_read_input_tokens"`
	CacheCreationTokens int64                `json:"cache_creation_input_tokens"`
	OutputTokensDetails *outputTokensDetails `json:"output_tokens_details"`
}

type outputTokensDetails struct {
	ThinkingTokens int64 `json:"thinking_tokens"`
}

func (u *rawUsage) usage() model.TokenUsage {
	if u == nil {
		return model.TokenUsage{}
	}
	t := model.TokenUsage{
		Input:      u.InputTokens,
		Output:     u.OutputTokens,
		CacheRead:  u.CacheReadTokens,
		CacheWrite: u.CacheCreationTokens,
	}
	if u.OutputTokensDetails != nil {
		t.Reasoning = u.OutputTokensDetails.ThinkingTokens
	}
	return t
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"` // tool_result: string | []block
	IsError   bool            `json:"is_error"`
	Source    *imageSource    `json:"source"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
}

// imageData is decoded only on a Blob request, never while loading a
// transcript. Images can contain megabytes of base64 per record.
type imageData struct {
	Source struct {
		Data string `json:"data"`
	} `json:"source"`
}

// subMeta is the content of subagents/agent-X.meta.json.
type subMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	ToolUseID   string `json:"toolUseId"`
}

// Load returns the full transcript for a session reference.
func (p *Provider) Load(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	result, err := p.LoadIncremental(ctx, ref, nil)
	return result.Transcript, err
}

// loadFile is the cold diagnostic entry point used by parser tests.
func (p *Provider) loadFile(ctx context.Context, s source) (*model.Transcript, *provider.Diagnostics, error) {
	sources, _ := p.discover(ctx) // child links are best effort
	result, diag, err := p.loadIncrementalFile(ctx, s, sources, nil)
	return result.Transcript, diag, err
}

// readSubMeta loads a subagent's small meta.json, best effort.
func readSubMeta(path string) (subMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return subMeta{}, err
	}
	var m subMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return subMeta{}, fmt.Errorf("claude: parse %s: %w", path, err)
	}
	return m, nil
}

// loader builds messages from complete transcript lines. Tools are located by
// index, so later results copy rather than mutate published messages.
type loader struct {
	path  string
	store transcriptbuild.Builder
	curID string
	// curIdx is the index of the current (split) assistant message.
	curIdx      int
	hasCur      bool
	tools       map[string]transcriptbuild.ToolLoc
	compactMsg  int // index of the message holding the open compaction part
	compactPart int
	diag        *provider.Diagnostics
}

func (l *loader) record(line []byte, start int64, lineNo int) {
	var rec rawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		l.diag.ParseErrors++
		l.diag.Warn(l.path, lineNo, "invalid JSON record: %v", err)
		return
	}
	switch rec.Type {
	case "assistant":
		l.assistant(&rec, lineNo)
	case "user":
		l.user(&rec, start, lineNo)
	case "system":
		l.system(&rec)
	case "attachment":
		l.attachment(&rec)
	case "ai-title", "summary", "last-prompt", "mode", "cost-state",
		"queue-operation", "atis-latch":
		// Bookkeeping handled by the scan aggregator.
	default:
		l.diag.Unknown(rec.Type)
	}
}

func (l *loader) assistant(rec *rawRecord, lineNo int) {
	m := rec.Message
	if m == nil {
		l.diag.Warn(l.path, lineNo, "assistant record without message")
		return
	}
	if !l.hasCur || l.curID != m.ID {
		l.store.Append(model.Message{
			ID:          m.ID,
			Role:        model.RoleAssistant,
			Time:        parseTime(rec.Timestamp),
			Model:       m.Model,
			IsSidechain: rec.IsSidechain,
		})
		l.curIdx = l.store.Len() - 1
		l.hasCur = true
		l.curID = m.ID
	}
	cur := l.store.Touch(l.curIdx)
	if m.Model != "" {
		cur.Model = m.Model
	}
	cur.Tokens = m.Usage.usage()

	for _, b := range contentBlocks(m.Content) {
		switch b.Type {
		case "text":
			if b.Text != "" {
				cur.Parts = append(cur.Parts, model.Part{Kind: model.PartText, Text: b.Text})
			}
		case "thinking":
			if b.Thinking == "" {
				continue
			}
			kind := model.PartReasoning
			if isNarration(b.Signature) {
				kind = model.PartText
			}
			cur.Parts = append(cur.Parts, model.Part{Kind: kind, Text: b.Thinking})
		case "tool_use":
			tc := &model.ToolCall{ID: b.ID, Name: b.Name, Input: b.Input, Status: model.ToolPending}
			cur.Parts = append(cur.Parts, model.Part{Kind: model.PartTool, Tool: tc})
			if l.tools == nil {
				l.tools = make(map[string]transcriptbuild.ToolLoc)
			}
			l.tools[b.ID] = transcriptbuild.ToolLoc{Message: l.curIdx, Part: len(cur.Parts) - 1}
		}
	}
}

func (l *loader) user(rec *rawRecord, start int64, lineNo int) {
	m := rec.Message
	if m == nil {
		return
	}
	text, blocks := decodeContent(m.Content)

	if rec.IsCompactSummary {
		if text == "" {
			text = joinTextBlocks(blocks)
		}
		if text != "" && l.compactMsg >= 0 && l.compactMsg < l.store.Len() {
			part := &l.store.Touch(l.compactMsg).Parts[l.compactPart]
			part.Text += "\n\n" + text
		}
		l.compactMsg, l.compactPart = -1, 0
		return
	}

	// Pair tool results with their calls first.
	for _, b := range blocks {
		if b.Type != "tool_result" {
			continue
		}
		out := outputText(b.Content)
		out, ref, truncated := clipOutput(out, start, b.ToolUseID)
		if loc, ok := l.tools[b.ToolUseID]; ok {
			tc := l.store.Tool(loc)
			tc.Output = out
			tc.OutputRef = ref
			tc.OutputTruncated = truncated || tc.OutputTruncated
			if b.IsError {
				tc.Status = model.ToolError
			} else if tc.Status == model.ToolPending || tc.Status == model.ToolUnknown {
				tc.Status = model.ToolCompleted
			}
		} else {
			l.diag.Warn(l.path, lineNo, "tool result for unknown call %s", b.ToolUseID)
		}
	}

	metaText := rec.IsMeta
	var parts []model.Part
	if text != "" {
		if isMetaText(text) {
			metaText = true
		}
		parts = append(parts, model.Part{Kind: model.PartText, Text: text})
	}
	for i, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text == "" {
				continue
			}
			if isMetaText(b.Text) {
				metaText = true
			}
			parts = append(parts, model.Part{Kind: model.PartText, Text: b.Text})
		case "image":
			if b.Source == nil {
				continue
			}
			parts = append(parts, model.Part{
				Kind: model.PartFile,
				File: &model.FileRef{
					Mime: b.Source.MediaType,
					Ref:  fmt.Sprintf("rec:%d:%d", start, i),
				},
			})
		}
	}
	if len(parts) == 0 {
		return // tool results only, or nothing displayable
	}
	l.store.Append(model.Message{
		Role:        model.RoleUser,
		Time:        parseTime(rec.Timestamp),
		Parts:       parts,
		IsMeta:      metaText,
		IsSidechain: rec.IsSidechain,
	})
}

func (l *loader) system(rec *rawRecord) {
	if rec.Subtype == "compact_boundary" && rec.CompactMetadata != nil {
		text := fmt.Sprintf("Context compacted (%s · %s → %s tokens)",
			rec.CompactMetadata.Trigger,
			comma(rec.CompactMetadata.PreTokens),
			comma(rec.CompactMetadata.PostTokens))
		msg := model.Message{
			Role: model.RoleSystem,
			Time: parseTime(rec.Timestamp),
			Parts: []model.Part{
				{Kind: model.PartCompaction, Text: text},
			},
		}
		l.store.Append(msg)
		l.compactMsg, l.compactPart = l.store.Len()-1, 0
		return
	}
	if strings.TrimSpace(rec.Content) != "" {
		l.store.Append(model.Message{
			Role: model.RoleSystem,
			Time: parseTime(rec.Timestamp),
			Parts: []model.Part{
				{Kind: model.PartNotice, Text: rec.Content},
			},
		})
	}
}

func (l *loader) attachment(rec *rawRecord) {
	a := rec.Attachment
	if a == nil {
		return
	}
	text := fmt.Sprintf("attachment (%s): %s", a.Type, model.TruncateRunes(a.Rendered, maxAttachmentRunes))
	l.store.Append(model.Message{
		Role:   model.RoleSystem,
		Time:   parseTime(rec.Timestamp),
		IsMeta: true,
		Parts:  []model.Part{{Kind: model.PartNotice, Text: text}},
	})
}

// clipOutput applies the output size limit and tool-results file reference.
func clipOutput(out string, start int64, toolUseID string) (string, ref string, truncated bool) {
	idx := strings.LastIndex(out, toolResultsMarker)
	if win := strings.LastIndex(out, toolResultsMarkerWindows); win > idx {
		idx = win
	}
	if idx >= 0 {
		name := out[idx+len(toolResultsMarker):]
		if end := strings.IndexAny(name, " \t\r\n\"'`)}]"); end >= 0 {
			name = name[:end]
		}
		if name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".." {
			return out, "file:" + name, true
		}
	}
	if len(out) <= maxOutputBytes {
		return out, "", false
	}
	end := maxOutputBytes
	for end > 0 && !utf8.ValidString(out[:end]) {
		end--
	}
	return out[:end], fmt.Sprintf("rec:%d:%s", start, toolUseID), true
}

// isMetaText reports whether user text is injected context rather than a
// real prompt.
func isMetaText(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "<command-") ||
		strings.HasPrefix(t, "<local-command-") ||
		strings.HasPrefix(t, "Caveat:") ||
		strings.HasPrefix(t, "<system-reminder>")
}

// decodeContent decodes message content, which is either a plain string or an
// array of blocks.
func decodeContent(raw json.RawMessage) (string, []block) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var blocks []block
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return "", blocks
	}
	return "", nil
}

// narrationKind tags a thinking block that holds the prose the user saw
// between tool calls, not the model's reasoning. Claude Code shows such
// blocks as ordinary answers.
const narrationKind = "narration"

// signatureKindPath locates the block kind inside a decoded thinking
// signature, as Claude Code reads it: field 2, then 1, then 8.
var signatureKindPath = []protowire.Number{2, 1, 8}

// isNarration reports whether a thinking block's signature tags it as
// narration. Malformed or unknown signatures read as ordinary thinking.
func isNarration(signature string) bool {
	if signature == "" {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(signature); err != nil {
			return false
		}
	}
	for _, num := range signatureKindPath {
		if b = protoBytesField(b, num); b == nil {
			return false
		}
	}
	return string(b) == narrationKind
}

// protoBytesField returns the first length-delimited field num of a
// protobuf message, or nil.
func protoBytesField(b []byte, num protowire.Number) []byte {
	for len(b) > 0 {
		n, typ, l := protowire.ConsumeTag(b)
		if l < 0 {
			return nil
		}
		b = b[l:]
		if n == num && typ == protowire.BytesType {
			v, l := protowire.ConsumeBytes(b)
			if l < 0 {
				return nil
			}
			return v
		}
		if l = protowire.ConsumeFieldValue(n, typ, b); l < 0 {
			return nil
		}
		b = b[l:]
	}
	return nil
}

// contentBlocks returns the block array form of message content.
func contentBlocks(raw json.RawMessage) []block {
	_, blocks := decodeContent(raw)
	return blocks
}

// outputText extracts tool result output from its content (string or blocks).
func outputText(raw json.RawMessage) string {
	text, blocks := decodeContent(raw)
	if text != "" {
		return text
	}
	return joinTextBlocks(blocks)
}

func joinTextBlocks(blocks []block) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type != "text" || blk.Text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(blk.Text)
	}
	return b.String()
}

func parseTime(raw json.RawMessage) time.Time {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// comma formats n with thousands separators (169569 → "169,569").
func comma(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i, digit := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(digit)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Blob returns lazy content for a transcript reference: full tool output
// ("rec:<off>:<toolUseID>"), decoded image bytes ("rec:<off>:<blockIdx>"),
// or a large tool output file ("file:<name>").
func (p *Provider) Blob(ctx context.Context, ref model.SessionRef, key string) ([]byte, error) {
	s, err := p.sourceFor(ctx, ref)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasPrefix(key, "rec:"):
		offStr, id, ok := strings.Cut(key[len("rec:"):], ":")
		if !ok {
			return nil, fmt.Errorf("claude: malformed blob key %q", key)
		}
		off, err := strconv.ParseInt(offStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("claude: malformed blob key %q", key)
		}
		return p.recordBlob(s.Path, off, id)
	case strings.HasPrefix(key, "file:"):
		name := key[len("file:"):]
		if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
			return nil, fmt.Errorf("claude: invalid blob file name %q", name)
		}
		sessionDir := filepath.Join(filepath.Dir(s.Path), strings.TrimSuffix(filepath.Base(s.Path), ".jsonl"))
		if s.ParentID != "" {
			sessionDir = filepath.Dir(filepath.Dir(s.Path))
		}
		data, err := os.ReadFile(filepath.Join(sessionDir, toolResultsDir, name))
		if err != nil {
			return nil, fmt.Errorf("claude: blob %s: %w", key, err)
		}
		return data, nil
	default:
		return nil, fmt.Errorf("claude: unsupported blob key %q", key)
	}
}

// recordBlob reads the single record at off and extracts the referenced
// tool output or image.
func (p *Provider) recordBlob(path string, off int64, id string) ([]byte, error) {
	r, err := jsonl.Open(path, off)
	if err != nil {
		return nil, fmt.Errorf("claude: blob rec:%d:%s: %w", off, id, err)
	}
	defer func() { _ = r.Close() }()
	line, _, err := r.Next()
	if err != nil {
		return nil, fmt.Errorf("claude: blob rec:%d:%s: %w", off, id, err)
	}
	var rec rawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, fmt.Errorf("claude: blob rec:%d:%s: %w", off, id, err)
	}
	if rec.Message == nil {
		return nil, fmt.Errorf("claude: blob rec:%d:%s: no message", off, id)
	}
	blocks := contentBlocks(rec.Message.Content)

	// Numeric id addresses a content block (image).
	if idx, err := strconv.Atoi(id); err == nil {
		if idx < 0 || idx >= len(blocks) {
			return nil, fmt.Errorf("claude: blob rec:%d:%s: block out of range", off, id)
		}
		var rawBlocks []json.RawMessage
		if err := json.Unmarshal(rec.Message.Content, &rawBlocks); err != nil || idx >= len(rawBlocks) {
			return nil, fmt.Errorf("claude: blob rec:%d:%s: decode content: %w", off, id, err)
		}
		var img imageData
		if err := json.Unmarshal(rawBlocks[idx], &img); err != nil || img.Source.Data == "" {
			return nil, fmt.Errorf("claude: blob rec:%d:%s: no image data", off, id)
		}
		data, err := base64.StdEncoding.DecodeString(img.Source.Data)
		if err != nil {
			return nil, fmt.Errorf("claude: blob rec:%d:%s: %w", off, id, err)
		}
		return data, nil
	}

	// Otherwise the id addresses a tool_use call whose result is here.
	for _, b := range blocks {
		if b.Type == "tool_result" && b.ToolUseID == id {
			return []byte(outputText(b.Content)), nil
		}
	}
	return nil, fmt.Errorf("claude: blob rec:%d:%s: tool result not found", off, id)
}

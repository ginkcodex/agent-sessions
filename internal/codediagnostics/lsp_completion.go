package codediagnostics

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Completion triggers: the user asked (the shortcut or a word character), the
// user typed one of the server's trigger characters, or the previous answer
// was incomplete and the user typed on.
const (
	CompletionInvoked    = "invoked"
	CompletionCharacter  = "character"
	CompletionIncomplete = "incomplete"
)

// completionOp names completion among the operations a session offers.
const completionOp = "completion"

const (
	completionTimeout       = 5 * time.Second
	maxCompletionItems      = 2000
	maxCompletionShort      = 1 << 10  // label, label details, detail, sort and filter text
	maxCompletionText       = 16 << 10 // text an item inserts, per edit
	maxCompletionDocs       = 64 << 10 // documentation per item
	maxCompletionAnswer     = 2 << 20  // kept bytes per answer
	maxCompletionExtraEdits = 100
	maxCompletionTriggers   = 32
)

// CompletionTrigger says why completion was asked for. Character is the one
// trigger character typed, set only for CompletionCharacter.
type CompletionTrigger struct {
	Kind      string `json:"kind"`
	Character string `json:"character,omitempty"`
}

// TextEdit replaces Range (UTF-16, exclusive end) with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// MarkupText is Markdown or plain text from the server.
type MarkupText struct {
	Kind  string `json:"kind"` // markdown or plaintext
	Value string `json:"value"`
}

// CompletionItem is one normalized suggestion. Edit, when set, is on the
// requested position's line and covers it; without it InsertText (or the
// label) replaces the word at the position. Snippet marks LSP snippet syntax
// in the inserted text. ExtraEdits are elsewhere in the document (an import)
// and overlap neither Edit nor each other. The server's command, commit
// characters and opaque data are not kept.
type CompletionItem struct {
	Label            string      `json:"label"`
	LabelDetail      string      `json:"labelDetail,omitempty"`
	LabelDescription string      `json:"labelDescription,omitempty"`
	Kind             int         `json:"kind,omitempty"` // LSP CompletionItemKind 1-25
	Detail           string      `json:"detail,omitempty"`
	Documentation    *MarkupText `json:"documentation,omitempty"`
	SortText         string      `json:"sortText,omitempty"`
	FilterText       string      `json:"filterText,omitempty"`
	InsertText       string      `json:"insertText,omitempty"`
	Edit             *TextEdit   `json:"edit,omitempty"`
	Snippet          bool        `json:"snippet,omitempty"`
	ExtraEdits       []TextEdit  `json:"extraEdits,omitempty"`
	Deprecated       bool        `json:"deprecated,omitempty"`
}

// CompletionResult answers one request for the document at Version. Items is
// empty, never nil, when the server has nothing. Incomplete asks the caller
// to ask again as the user types on rather than filter locally; Truncated
// reports dropped items or shortened text and implies Incomplete when items
// were dropped. ResultID names the answer for ResolveCompletion, which takes
// an item's index in Items. TriggerCharacters are the answering server's,
// so the editor learns which typed characters to ask after.
type CompletionResult struct {
	DocumentID        string           `json:"documentId"`
	Version           int64            `json:"version"`
	ResultID          string           `json:"resultId,omitempty"`
	Items             []CompletionItem `json:"items"`
	Incomplete        bool             `json:"isIncomplete,omitempty"`
	Truncated         bool             `json:"truncated,omitempty"`
	TriggerCharacters string           `json:"triggerCharacters,omitempty"`
}

// CompletionRequest is one request to the session that holds the document at
// Generation, resolved by the Manager under its lock.
type CompletionRequest struct {
	Root       AuthorizedRoot
	Owner      DocumentOwnerIdentity
	Generation string
	Path       string
	Position   Position
	Trigger    CompletionTrigger
	Text       string // the document at the requested version
}

// CompletionProvider sends completion requests to running sessions. Like
// NavigationProvider it never starts a process.
type CompletionProvider interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error)
	ResolveCompletion(ctx context.Context, req CompletionResolveRequest) (CompletionDetails, error)
}

// completionClientCapabilities is what the client reads in an answer:
// snippets (the editor turns their tab stops into fields), no insert/replace
// pairs (one range per item), documentation in either format, details
// resolved later.
func completionClientCapabilities() map[string]any {
	kinds := make([]int, 25)
	for i := range kinds {
		kinds[i] = i + 1
	}
	return map[string]any{
		"contextSupport": true,
		"completionItem": map[string]any{
			"snippetSupport":          true,
			"documentationFormat":     []string{"markdown", "plaintext"},
			"labelDetailsSupport":     true,
			"deprecatedSupport":       true,
			"tagSupport":              map[string]any{"valueSet": []int{1}},
			"resolveSupport":          map[string]any{"properties": []string{"documentation", "detail"}},
			"insertReplaceSupport":    false,
			"commitCharactersSupport": false,
		},
		"completionItemKind": map[string]any{"valueSet": kinds},
	}
}

// completionProvider reads the completionProvider capability: whether it is
// offered, its trigger characters (single printable runes, deduplicated, at
// most 32) and whether items are resolved. A malformed value is not offered;
// it never fails the session.
func completionProvider(raw json.RawMessage) (bool, string, bool) {
	s := strings.TrimSpace(string(raw))
	switch {
	case s == "true":
		return true, "", false
	case s == "" || s[0] != '{':
		return false, "", false
	}
	var options struct {
		TriggerCharacters []json.RawMessage `json:"triggerCharacters"`
		ResolveProvider   bool              `json:"resolveProvider"`
	}
	if json.Unmarshal(raw, &options) != nil {
		return false, "", false
	}
	var triggers []rune
	for _, item := range options.TriggerCharacters {
		var c string
		if json.Unmarshal(item, &c) != nil || utf8.RuneCountInString(c) != 1 {
			continue
		}
		r, _ := utf8.DecodeRuneInString(c)
		if r == utf8.RuneError || unicode.IsControl(r) || strings.ContainsRune(string(triggers), r) {
			continue
		}
		if len(triggers) == maxCompletionTriggers {
			break
		}
		triggers = append(triggers, r)
	}
	return true, string(triggers), options.ResolveProvider
}

// validTrigger checks a trigger's shape: a character trigger names one
// printable character.
func validTrigger(t CompletionTrigger) bool {
	switch t.Kind {
	case CompletionInvoked, CompletionIncomplete:
		return t.Character == ""
	case CompletionCharacter:
		r, n := utf8.DecodeRuneInString(t.Character)
		return n == len(t.Character) && r != utf8.RuneError && !unicode.IsControl(r)
	}
	return false
}

// Complete asks the language server that holds an open document for
// completions at pos, with the checks Navigate makes: the document must be
// open at exactly version, the position inside its text, and a running
// session must offer completion. Of several owners, the one that offers
// completion and the most navigation answers is asked (Pyright, not Ruff).
// Items whose ranges do not fit the text at version are left out. The answer
// is kept for ResolveCompletion until a later answer replaces it or the
// document is reopened or closed; typing on keeps it.
func (m *Manager) Complete(ctx context.Context, c Context, documentID string, version int64, pos Position, trigger CompletionTrigger) (CompletionResult, error) {
	m.mu.Lock()
	provider, ok := m.provider.(CompletionProvider)
	route, err := m.documentRoute(c, documentID, version, completionOp, pos, ok, "Code completion is unavailable")
	m.completionSeq++
	seq := m.completionSeq
	m.mu.Unlock()
	if err != nil {
		return CompletionResult{}, err
	}
	if !validTrigger(trigger) {
		return CompletionResult{}, failure("invalidArgument", "Unknown completion trigger")
	}
	triggers := route.caps.CompletionTriggers
	// The editor asks after characters before it knows the server's: one the
	// server did not name is answered with nothing, without asking it.
	if trigger.Kind == CompletionCharacter && !strings.Contains(triggers, trigger.Character) {
		return CompletionResult{DocumentID: documentID, Version: version, Items: []CompletionItem{}, TriggerCharacters: triggers}, nil
	}
	r := route.req
	result, err := provider.Complete(ctx, CompletionRequest{Root: r.Root, Owner: r.Owner, Generation: r.Generation, Path: r.Path, Position: pos, Trigger: trigger, Text: route.text})
	if err != nil {
		return CompletionResult{}, err
	}
	result.DocumentID, result.Version, result.TriggerCharacters = documentID, version, triggers
	if result.Items == nil {
		result.Items = []CompletionItem{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// The attachment may have ended (or lost approval) while the server
	// answered.
	a, err := m.lookup(c)
	if err == nil {
		err = m.liveGate(a)
	}
	if err != nil {
		return CompletionResult{}, err
	}
	if d := a.docs[documentID]; d != nil && d.version == version && result.ResultID != "" && seq > d.completion.seq {
		d.completion = keptAnswer{seq: seq, resultID: result.ResultID, items: len(result.Items), version: version, owner: r.Owner, generation: r.Generation}
	}
	return result, nil
}

// Complete sends textDocument/completion to the running session that owns
// the document and normalizes its answer. Five seconds without one is a
// timeout; the server is told to cancel.
func (d SessionDocuments) Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error) {
	_, s, tr, target, err := d.sessionFor(req.Root, req.Path, req.Owner, req.Generation, completionOp)
	if err != nil {
		return CompletionResult{}, err
	}
	uri := fileURI(target)
	why := map[string]any{"triggerKind": 1}
	switch req.Trigger.Kind {
	case CompletionCharacter:
		why = map[string]any{"triggerKind": 2, "triggerCharacter": req.Trigger.Character}
	case CompletionIncomplete:
		why["triggerKind"] = 3
	}
	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     lspPosition{req.Position.Line, req.Position.Character},
		"context":      why,
	}
	id := s.beginCompletion(uri)
	cctx, cancel := s.deadline(ctx, completionTimeout)
	defer cancel()
	if err := s.reindex(cctx, tr); err != nil {
		s.dropCompletion(uri, id)
		return CompletionResult{}, navigationCallError(cctx, err)
	}
	raw, err := tr.Call(cctx, "textDocument/completion", params)
	if err != nil {
		s.dropCompletion(uri, id)
		return CompletionResult{}, navigationCallError(cctx, err)
	}
	result, items, err := completionResult(raw, req.Position, req.Text)
	if err != nil || !s.keepCompletion(uri, id, items) {
		s.dropCompletion(uri, id)
		return result, err
	}
	result.ResultID = id
	return result, nil
}

// completionResult reads CompletionItem[] or CompletionList and keeps what
// the editor uses, within the caps. Items with an invalid shape, a range that
// is not inside text, a main edit that is not on pos's line or does not cover
// pos, overlong text to insert, or more than 100 extra edits are left out
// (applying part of them could break the code). Extra edits overlapping the
// main edit or an earlier extra edit are dropped from the item. The server's
// items for the kept ones are returned too, in the same order, for resolve.
func completionResult(raw json.RawMessage, pos Position, text string) (CompletionResult, []json.RawMessage, error) {
	out := CompletionResult{Items: []CompletionItem{}}
	var items []json.RawMessage
	switch t := strings.TrimSpace(string(raw)); {
	case t == "" || t == "null":
		return out, nil, nil
	case t[0] == '[':
		if json.Unmarshal(raw, &items) != nil {
			return CompletionResult{}, nil, failure("toolFailed", "The language server answer is invalid")
		}
	default:
		var list struct {
			IsIncomplete bool              `json:"isIncomplete"`
			Items        []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(raw, &list) != nil {
			return CompletionResult{}, nil, failure("toolFailed", "The language server answer is invalid")
		}
		out.Incomplete, items = list.IsIncomplete, list.Items
	}
	budget := maxCompletionAnswer
	var kept []json.RawMessage
	lines := &lineIndex{text: text}
	for _, raw := range items {
		item, size, truncated, ok := completionItem(raw, pos, lines)
		if !ok {
			continue
		}
		if len(out.Items) == maxCompletionItems || size > budget {
			out.Incomplete, out.Truncated = true, true
			break
		}
		budget -= size
		out.Truncated = out.Truncated || truncated
		out.Items = append(out.Items, item)
		kept = append(kept, raw)
	}
	return out, kept, nil
}

// completionItem normalizes one item and reports the bytes it keeps and
// whether display text was shortened.
func completionItem(raw json.RawMessage, pos Position, lines *lineIndex) (CompletionItem, int, bool, bool) {
	var in struct {
		Label        string `json:"label"`
		LabelDetails *struct {
			Detail      string `json:"detail"`
			Description string `json:"description"`
		} `json:"labelDetails"`
		Kind             int             `json:"kind"`
		Tags             []int           `json:"tags"`
		Detail           string          `json:"detail"`
		Documentation    json.RawMessage `json:"documentation"`
		Deprecated       bool            `json:"deprecated"`
		SortText         string          `json:"sortText"`
		FilterText       string          `json:"filterText"`
		InsertText       string          `json:"insertText"`
		InsertTextFormat int             `json:"insertTextFormat"`
		TextEdit         *struct {
			Range   *Range  `json:"range"`
			Insert  *Range  `json:"insert"`
			NewText *string `json:"newText"`
		} `json:"textEdit"`
		AdditionalTextEdits []struct {
			Range   *Range  `json:"range"`
			NewText *string `json:"newText"`
		} `json:"additionalTextEdits"`
	}
	if json.Unmarshal(raw, &in) != nil || !bounded(in.Label, maxCompletionShort) {
		return CompletionItem{}, 0, false, false
	}
	if len(in.InsertText) > maxCompletionText || len(in.SortText) > maxCompletionShort || len(in.FilterText) > maxCompletionShort {
		return CompletionItem{}, 0, false, false
	}
	item := CompletionItem{
		Label:      in.Label,
		SortText:   in.SortText,
		FilterText: in.FilterText,
		InsertText: in.InsertText,
		Snippet:    in.InsertTextFormat == 2,
		Deprecated: in.Deprecated || slices.Contains(in.Tags, 1),
	}
	if in.Kind >= 1 && in.Kind <= 25 {
		item.Kind = in.Kind
	}
	truncated := false
	short := func(s string) string {
		if len(s) > maxCompletionShort {
			truncated = true
			return shorten(s, maxCompletionShort)
		}
		return s
	}
	item.Detail = short(in.Detail)
	if in.LabelDetails != nil {
		item.LabelDetail, item.LabelDescription = short(in.LabelDetails.Detail), short(in.LabelDetails.Description)
	}
	if docs, cut, ok := markupText(in.Documentation); ok {
		item.Documentation, truncated = docs, truncated || cut
	}
	main := Range{Start: pos, End: pos}
	if in.TextEdit != nil {
		r := in.TextEdit.Range
		if r == nil {
			r = in.TextEdit.Insert // an insert/replace pair: the insert range
		}
		if r == nil || in.TextEdit.NewText == nil || !lines.contains(*r) || len(*in.TextEdit.NewText) > maxCompletionText {
			return CompletionItem{}, 0, false, false
		}
		if r.Start.Line != pos.Line || r.End.Line != pos.Line || r.Start.Character > pos.Character || r.End.Character < pos.Character {
			return CompletionItem{}, 0, false, false
		}
		item.Edit, main = &TextEdit{Range: *r, NewText: *in.TextEdit.NewText}, *r
	}
	if len(in.AdditionalTextEdits) > maxCompletionExtraEdits {
		return CompletionItem{}, 0, false, false
	}
	taken := []Range{main}
	for _, e := range in.AdditionalTextEdits {
		if e.Range == nil || e.NewText == nil || !lines.contains(*e.Range) || len(*e.NewText) > maxCompletionText {
			return CompletionItem{}, 0, false, false
		}
		if overlapsAny(*e.Range, taken) {
			continue
		}
		taken = append(taken, *e.Range)
		item.ExtraEdits = append(item.ExtraEdits, TextEdit{Range: *e.Range, NewText: *e.NewText})
	}
	return item, item.size(), truncated, true
}

// size is the item's kept text, the bytes it adds to an answer.
func (c CompletionItem) size() int {
	n := len(c.Label) + len(c.LabelDetail) + len(c.LabelDescription) + len(c.Detail) + len(c.SortText) + len(c.FilterText) + len(c.InsertText)
	if c.Documentation != nil {
		n += len(c.Documentation.Value)
	}
	if c.Edit != nil {
		n += len(c.Edit.NewText)
	}
	for _, e := range c.ExtraEdits {
		n += len(e.NewText)
	}
	return n
}

// markupText reads MarkupContent or a plain string, capped at 64 KiB on a
// UTF-8 boundary. Empty documentation is none.
func markupText(raw json.RawMessage) (*MarkupText, bool, bool) {
	if t := strings.TrimSpace(string(raw)); t == "" || t == "null" {
		return nil, false, false
	}
	docs := MarkupText{Kind: "plaintext"}
	if json.Unmarshal(raw, &docs.Value) != nil {
		var content struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
		}
		if json.Unmarshal(raw, &content) != nil || (content.Kind != "markdown" && content.Kind != "plaintext") {
			return nil, false, false
		}
		docs = MarkupText{Kind: content.Kind, Value: content.Value}
	}
	if strings.TrimSpace(docs.Value) == "" {
		return nil, false, false
	}
	cut := len(docs.Value) > maxCompletionDocs
	if cut {
		docs.Value = shorten(docs.Value, maxCompletionDocs)
	}
	return &docs, cut, true
}

// lineIndex checks many ranges against one text, as rangeInText does, while
// reading each line once: an answer has up to 2,000 items, mostly on one line.
type lineIndex struct {
	text   string
	starts []int // byte offset of each line; nil until first used
	lines  map[int]lineShape
}

// lineShape is a line's UTF-16 length and the columns that fall inside a
// surrogate pair.
type lineShape struct {
	length int
	split  map[int]bool
}

func (x *lineIndex) contains(r Range) bool {
	return validRange(&r) && x.at(r.Start) && x.at(r.End)
}

func (x *lineIndex) at(p Position) bool {
	if x.starts == nil {
		x.starts, x.lines = []int{0}, map[int]lineShape{}
		for i := 0; i < len(x.text); {
			if w := separator(x.text, i); w > 0 {
				i += w
				x.starts = append(x.starts, i)
			} else {
				i++
			}
		}
	}
	if p.Line < 0 || p.Line >= len(x.starts) || p.Character < 0 {
		return false
	}
	shape, ok := x.lines[p.Line]
	if !ok {
		for i := x.starts[p.Line]; i < len(x.text) && separator(x.text, i) == 0; {
			r, size := utf8.DecodeRuneInString(x.text[i:])
			shape.length++
			if r >= 0x10000 {
				if shape.split == nil {
					shape.split = map[int]bool{}
				}
				shape.split[shape.length] = true
				shape.length++
			}
			i += size
		}
		x.lines[p.Line] = shape
	}
	return p.Character <= shape.length && !shape.split[p.Character]
}

// overlapsAny reports whether r shares text with any of taken. Two edits at
// the same place overlap even when empty: their order would be ambiguous.
func overlapsAny(r Range, taken []Range) bool {
	before := func(a, b Position) bool { return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character }
	for _, t := range taken {
		if r.Start == t.Start || before(r.Start, t.End) && before(t.Start, r.End) {
			return true
		}
	}
	return false
}

// Validate checks an answer that crossed a connection against the bounds
// Complete keeps, so a peer cannot hand the editor more, or other, than a
// local server would.
func (r CompletionResult) Validate() error {
	invalid := failure("toolFailed", "The completion answer is invalid")
	if len(r.Items) > maxCompletionItems || r.ResultID != "" && !bounded(r.ResultID, 64) || !validTriggers(r.TriggerCharacters) {
		return invalid
	}
	total := 0
	for _, item := range r.Items {
		if !validCompletionItem(item) {
			return invalid
		}
		total += item.size()
	}
	if total > maxCompletionAnswer {
		return invalid
	}
	return nil
}

// validTriggers checks trigger characters as completionProvider keeps them:
// distinct printable characters, at most 32.
func validTriggers(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxCompletionTriggers {
		return false
	}
	for i, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune(s[:i], r) {
			return false
		}
	}
	return true
}

func validCompletionItem(c CompletionItem) bool {
	if !bounded(c.Label, maxCompletionShort) || c.Kind < 0 || c.Kind > 25 || len(c.ExtraEdits) > maxCompletionExtraEdits {
		return false
	}
	for _, s := range []string{c.LabelDetail, c.LabelDescription, c.Detail, c.SortText, c.FilterText} {
		if len(s) > maxCompletionShort || !utf8.ValidString(s) {
			return false
		}
	}
	if len(c.InsertText) > maxCompletionText || !utf8.ValidString(c.InsertText) {
		return false
	}
	if d := c.Documentation; d != nil && ((d.Kind != "markdown" && d.Kind != "plaintext") || len(d.Value) > maxCompletionDocs || !utf8.ValidString(d.Value)) {
		return false
	}
	var taken []Range
	if c.Edit != nil {
		if !validRange(&c.Edit.Range) || c.Edit.Range.Start.Line != c.Edit.Range.End.Line || len(c.Edit.NewText) > maxCompletionText || !utf8.ValidString(c.Edit.NewText) {
			return false
		}
		taken = append(taken, c.Edit.Range)
	}
	for _, e := range c.ExtraEdits {
		if !validRange(&e.Range) || len(e.NewText) > maxCompletionText || !utf8.ValidString(e.NewText) || overlapsAny(e.Range, taken) {
			return false
		}
		taken = append(taken, e.Range)
	}
	return true
}

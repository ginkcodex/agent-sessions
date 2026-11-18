package codediagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"unicode/utf8"
)

// CompletionDetails is what resolving one item adds: its detail and
// documentation, capped as in a completion answer, and, when asked for at a
// version, the extra edits (imports) the server computes only on resolve
// (vtsls). ExtraEdits apply to the document at Version; both are set
// together or not at all.
type CompletionDetails struct {
	DocumentID    string      `json:"documentId"`
	ResultID      string      `json:"resultId"`
	Index         int         `json:"index"`
	Detail        string      `json:"detail,omitempty"`
	Documentation *MarkupText `json:"documentation,omitempty"`
	Truncated     bool        `json:"truncated,omitempty"`
	Version       int64       `json:"version,omitempty"`
	ExtraEdits    []TextEdit  `json:"extraEdits,omitempty"`
}

// CompletionResolveRequest names an item of a session's kept answer. Version
// above 0 asks for the resolved item's extra edits too, checked against Text,
// the document at Version.
type CompletionResolveRequest struct {
	Root       AuthorizedRoot
	Owner      DocumentOwnerIdentity
	Generation string
	Path       string
	ResultID   string
	Index      int
	Version    int64
	Text       string
}

// keptAnswer is the Manager's record of a document's last completion answer:
// which session gave it, for which version, and how many items it had. It
// stays through later changes: the editor filters a complete answer as the
// user types on, and still resolves its items.
type keptAnswer struct {
	seq        uint64
	resultID   string
	items      int
	version    int64
	owner      DocumentOwnerIdentity
	generation string
}

// keptItems is a session's last answer for one URI: the server's items, with
// their opaque data, which never leave the host. Items is nil while the
// request runs; bytes is their raw size.
type keptItems struct {
	id    string
	seq   uint64
	items []json.RawMessage
	bytes int
}

// maxKeptCompletion caps the raw item bytes a session keeps for resolve,
// over all its URIs. Only the normalized answer is capped otherwise, and a
// server's items can be several times larger (opaque data, unused fields).
const maxKeptCompletion = 8 << 20

// ResolveCompletion returns an item's detail and documentation, from the
// server when it resolves items and otherwise from the answer. The answer
// must be the document's last one; anything else is staleVersion. The
// document may have changed since: the server resolves with what it has.
//
// With version above 0, the current version, which the answering server must
// have, the server's extra edits for the item come too, checked against that
// version's text. They are dropped when the document changed while the server
// answered, as they may no longer fit.
func (m *Manager) ResolveCompletion(ctx context.Context, c Context, documentID, resultID string, index int, version int64) (CompletionDetails, error) {
	m.mu.Lock()
	req, provider, asked, err := m.resolveRoute(c, documentID, resultID, index, version)
	m.mu.Unlock()
	if err != nil {
		return CompletionDetails{}, err
	}
	details, err := provider.ResolveCompletion(ctx, req)
	if err != nil {
		return CompletionDetails{}, err
	}
	details.DocumentID, details.ResultID, details.Index = documentID, resultID, index
	details.Version = 0
	m.mu.Lock()
	defer m.mu.Unlock()
	// The attachment may have ended (or lost approval) while the server
	// answered.
	a, err := m.lookup(c)
	if err == nil {
		err = m.liveGate(a)
	}
	if err != nil {
		return CompletionDetails{}, err
	}
	// The edits fit the text of the lifecycle and version asked about.
	if d := a.docs[documentID]; version <= 0 || len(details.ExtraEdits) == 0 || d != asked || d.version != version {
		details.ExtraEdits = nil
	} else {
		details.Version = version
	}
	return details, nil
}

func (m *Manager) resolveRoute(c Context, documentID, resultID string, index int, version int64) (CompletionResolveRequest, CompletionProvider, *documentState, error) {
	a, d, err := m.ownedDocument(c, documentID)
	if err != nil {
		return CompletionResolveRequest{}, nil, nil, err
	}
	provider, ok := m.provider.(CompletionProvider)
	switch kept := d.completion; {
	case !ok:
		return CompletionResolveRequest{}, nil, nil, failure("unsupported", "Code completion is unavailable")
	case !bounded(resultID, 64) || version < 0:
		return CompletionResolveRequest{}, nil, nil, failure("invalidArgument", "Invalid completion result")
	case kept.resultID != resultID:
		return CompletionResolveRequest{}, nil, nil, completionGone()
	case index < 0 || index >= kept.items:
		return CompletionResolveRequest{}, nil, nil, failure("invalidArgument", "No such completion item")
	case d.contextBlocked:
		return CompletionResolveRequest{}, nil, nil, goContextChanged()
	case version > 0 && !answeredAt(d, kept, version):
		return CompletionResolveRequest{}, nil, nil, failure("staleVersion", "The document changed since this request")
	default:
		req := CompletionResolveRequest{Root: a.root, Owner: kept.owner, Generation: kept.generation, Path: d.path, ResultID: resultID, Index: index}
		if version > 0 {
			req.Version, req.Text = version, d.text
		}
		return req, provider, d, nil
	}
}

// answeredAt reports whether the document is at version and the session that
// gave the kept answer has received it, so edits it resolves fit d.text.
func answeredAt(d *documentState, kept keptAnswer, version int64) bool {
	if d.version != version {
		return false
	}
	routes := d.routes
	if len(routes) == 0 {
		routes = []*documentState{d}
	}
	for _, route := range routes {
		if route.owner == kept.owner {
			return !route.broken && route.generation == kept.generation && route.version == version
		}
	}
	return false
}

func completionGone() error {
	return failure("staleVersion", "The completion answer is no longer kept")
}

// ResolveCompletion resolves an item of the session's kept answer. A
// restarted session or a dropped answer is staleVersion. With req.Version,
// the resolved item's extra edits come too, checked against req.Text.
func (d SessionDocuments) ResolveCompletion(ctx context.Context, req CompletionResolveRequest) (CompletionDetails, error) {
	_, s, tr, target, err := d.sessionFor(req.Root, req.Path, req.Owner, req.Generation, completionOp)
	var e *Error
	if errors.As(err, &e) && e.Code == "resyncRequired" {
		return CompletionDetails{}, completionGone()
	}
	if err != nil {
		return CompletionDetails{}, err
	}
	raw, ok := s.keptCompletion(fileURI(target), req.ResultID, req.Index)
	if !ok {
		return CompletionDetails{}, completionGone()
	}
	listed, err := itemDetails(raw)
	// A server that does not resolve put its edits in the answer already.
	if err != nil || !s.info().Capabilities.CompletionResolve {
		return listed, err
	}
	cctx, cancel := s.deadline(ctx, completionTimeout)
	defer cancel()
	answer, err := tr.Call(cctx, "completionItem/resolve", raw)
	if err != nil {
		return CompletionDetails{}, navigationCallError(cctx, err)
	}
	resolved, err := itemDetails(answer)
	if err != nil {
		return CompletionDetails{}, err
	}
	// What the server leaves out keeps the value the list had.
	kept := false
	if resolved.Detail == "" {
		resolved.Detail, kept = listed.Detail, true
	}
	if resolved.Documentation == nil {
		resolved.Documentation, kept = listed.Documentation, true
	}
	resolved.Truncated = resolved.Truncated || kept && listed.Truncated
	if req.Version > 0 {
		resolved.ExtraEdits = resolvedEdits(answer, req.Text)
	}
	return resolved, nil
}

// resolvedEdits reads a resolved item's extra edits for text: all of them or
// none, as part of an import can break the code. Each must be inside text and
// within the bounds of a completion answer, and none may overlap another.
// The editor checks them against the main edit, which it maps.
func resolvedEdits(raw json.RawMessage, text string) []TextEdit {
	var in struct {
		AdditionalTextEdits []struct {
			Range   *Range  `json:"range"`
			NewText *string `json:"newText"`
		} `json:"additionalTextEdits"`
	}
	if json.Unmarshal(raw, &in) != nil || len(in.AdditionalTextEdits) > maxCompletionExtraEdits {
		return nil
	}
	lines := &lineIndex{text: text}
	var out []TextEdit
	var taken []Range
	for _, e := range in.AdditionalTextEdits {
		if e.Range == nil || e.NewText == nil || !lines.contains(*e.Range) || len(*e.NewText) > maxCompletionText || !utf8.ValidString(*e.NewText) {
			return nil
		}
		if overlapsAny(*e.Range, taken) {
			return nil
		}
		taken = append(taken, *e.Range)
		out = append(out, TextEdit{Range: *e.Range, NewText: *e.NewText})
	}
	return out
}

// itemDetails reads an item's detail and documentation within the caps.
func itemDetails(raw json.RawMessage) (CompletionDetails, error) {
	var in struct {
		Detail        string          `json:"detail"`
		Documentation json.RawMessage `json:"documentation"`
	}
	if json.Unmarshal(raw, &in) != nil {
		return CompletionDetails{}, failure("toolFailed", "The language server answer is invalid")
	}
	var out CompletionDetails
	if len(in.Detail) > maxCompletionShort {
		in.Detail, out.Truncated = shorten(in.Detail, maxCompletionShort), true
	}
	out.Detail = in.Detail
	if docs, cut, ok := markupText(in.Documentation); ok {
		out.Documentation, out.Truncated = docs, out.Truncated || cut
	}
	return out, nil
}

// Validate checks details that crossed a connection against the caps.
func (r CompletionDetails) Validate() error {
	invalid := failure("toolFailed", "The completion details are invalid")
	d := r.Documentation
	if !bounded(r.ResultID, 64) || r.Index < 0 || r.Index >= maxCompletionItems || len(r.Detail) > maxCompletionShort || !utf8.ValidString(r.Detail) ||
		d != nil && ((d.Kind != "markdown" && d.Kind != "plaintext") || len(d.Value) > maxCompletionDocs || !utf8.ValidString(d.Value)) {
		return invalid
	}
	// Edits only with the version they fit, and the version only with edits.
	if r.Version < 0 || (r.Version > 0) != (len(r.ExtraEdits) > 0) || len(r.ExtraEdits) > maxCompletionExtraEdits {
		return invalid
	}
	var taken []Range
	for _, e := range r.ExtraEdits {
		if !validRange(&e.Range) || len(e.NewText) > maxCompletionText || !utf8.ValidString(e.NewText) || overlapsAny(e.Range, taken) {
			return invalid
		}
		taken = append(taken, e.Range)
	}
	return nil
}

// beginCompletion starts a request for an open uri and returns its result ID.
// Its slot replaces the URI's kept answer at once, so an earlier request that
// answers later is not kept.
func (s *lspSession) beginCompletion(uri string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completionIDs++
	id := strconv.FormatUint(s.completionIDs, 10)
	if _, open := s.wireBases[uri]; open {
		if s.completions == nil {
			s.completions = make(map[string]keptItems)
		}
		s.completions[uri] = keptItems{id: id, seq: s.completionIDs}
	}
	return id
}

// keepCompletion stores the items of request id, unless a later request, a
// reopen or a close replaced or dropped its slot. A change keeps it. Within
// maxKeptCompletion: other URIs' answers are dropped, oldest first, and an
// answer larger than the cap keeps the items that fit; the rest cannot be
// resolved.
func (s *lspSession) keepCompletion(uri, id string, items []json.RawMessage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.completions[uri]
	if !ok || k.id != id {
		return false
	}
	n := 0
	for i, raw := range items {
		if n+len(raw) > maxKeptCompletion {
			items = slices.Clone(items[:i]) // not the dropped items' backing array
			break
		}
		n += len(raw)
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	k.items, k.bytes = items, n
	s.completions[uri] = k
	for total := s.keptCompletionBytes(); total > maxKeptCompletion; {
		oldest := ""
		for other, o := range s.completions {
			if other != uri && o.bytes > 0 && (oldest == "" || o.seq < s.completions[oldest].seq) {
				oldest = other
			}
		}
		total -= s.completions[oldest].bytes
		delete(s.completions, oldest)
	}
	return true
}

func (s *lspSession) keptCompletionBytes() int {
	n := 0
	for _, k := range s.completions {
		n += k.bytes
	}
	return n
}

// dropCompletion forgets uri's kept answer if it is request id's.
func (s *lspSession) dropCompletion(uri, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.completions[uri]; ok && k.id == id {
		delete(s.completions, uri)
	}
}

func (s *lspSession) keptCompletion(uri, id string, index int) (json.RawMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.completions[uri]
	if !ok || k.id != id || index < 0 || index >= len(k.items) {
		return nil, false
	}
	return k.items[index], true
}

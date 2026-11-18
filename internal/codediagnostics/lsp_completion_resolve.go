package codediagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

// CompletionDetails is what resolving one item adds: its detail and
// documentation, capped as in a completion answer. Resolved extra edits are
// ignored (applying them would need the edit checks again; gopls and vtsls
// send them in the list already).
type CompletionDetails struct {
	DocumentID    string      `json:"documentId"`
	ResultID      string      `json:"resultId"`
	Index         int         `json:"index"`
	Detail        string      `json:"detail,omitempty"`
	Documentation *MarkupText `json:"documentation,omitempty"`
	Truncated     bool        `json:"truncated,omitempty"`
}

// CompletionResolveRequest names an item of a session's kept answer.
type CompletionResolveRequest struct {
	Root       AuthorizedRoot
	Owner      DocumentOwnerIdentity
	Generation string
	Path       string
	ResultID   string
	Index      int
}

// keptAnswer is the Manager's record of a document's last completion answer:
// which session gave it, for which version, and how many items it had.
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
// request runs.
type keptItems struct {
	id    string
	items []json.RawMessage
}

// ResolveCompletion returns an item's detail and documentation, from the
// server when it resolves items and otherwise from the answer. The answer
// must be the document's last one, and the document unchanged since; anything
// else is staleVersion.
func (m *Manager) ResolveCompletion(ctx context.Context, c Context, documentID, resultID string, index int) (CompletionDetails, error) {
	m.mu.Lock()
	req, provider, err := m.resolveRoute(c, documentID, resultID, index)
	m.mu.Unlock()
	if err != nil {
		return CompletionDetails{}, err
	}
	details, err := provider.ResolveCompletion(ctx, req)
	if err != nil {
		return CompletionDetails{}, err
	}
	details.DocumentID, details.ResultID, details.Index = documentID, resultID, index
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
	return details, nil
}

func (m *Manager) resolveRoute(c Context, documentID, resultID string, index int) (CompletionResolveRequest, CompletionProvider, error) {
	a, d, err := m.ownedDocument(c, documentID)
	if err != nil {
		return CompletionResolveRequest{}, nil, err
	}
	provider, ok := m.provider.(CompletionProvider)
	switch kept := d.completion; {
	case !ok:
		return CompletionResolveRequest{}, nil, failure("unsupported", "Code completion is unavailable")
	case !bounded(resultID, 64):
		return CompletionResolveRequest{}, nil, failure("invalidArgument", "Invalid completion result")
	case kept.resultID != resultID || kept.version != d.version:
		return CompletionResolveRequest{}, nil, completionGone()
	case index < 0 || index >= kept.items:
		return CompletionResolveRequest{}, nil, failure("invalidArgument", "No such completion item")
	case d.contextBlocked:
		return CompletionResolveRequest{}, nil, goContextChanged()
	default:
		return CompletionResolveRequest{Root: a.root, Owner: kept.owner, Generation: kept.generation, Path: d.path, ResultID: resultID, Index: index}, provider, nil
	}
}

func completionGone() error {
	return failure("staleVersion", "The completion answer is no longer kept")
}

// ResolveCompletion resolves an item of the session's kept answer. A
// restarted session or a dropped answer is staleVersion.
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
	return resolved, nil
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
	d := r.Documentation
	if !bounded(r.ResultID, 64) || r.Index < 0 || r.Index >= maxCompletionItems || len(r.Detail) > maxCompletionShort || !utf8.ValidString(r.Detail) ||
		d != nil && ((d.Kind != "markdown" && d.Kind != "plaintext") || len(d.Value) > maxCompletionDocs || !utf8.ValidString(d.Value)) {
		return failure("toolFailed", "The completion details are invalid")
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
		s.completions[uri] = keptItems{id: id}
	}
	return id
}

// keepCompletion stores the items of request id, unless a later request or
// a document step replaced or dropped its slot.
func (s *lspSession) keepCompletion(uri, id string, items []json.RawMessage) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.completions[uri]
	if !ok || k.id != id {
		return false
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	k.items = items
	s.completions[uri] = k
	return true
}

// dropCompletion forgets uri's kept answer: any when id is empty, otherwise
// only request id's.
func (s *lspSession) dropCompletion(uri, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.completions[uri]; ok && (id == "" || k.id == id) {
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

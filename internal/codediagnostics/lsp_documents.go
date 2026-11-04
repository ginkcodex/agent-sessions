package codediagnostics

import (
	"context"
	"errors"
	"path/filepath"
)

// DocumentServer is the fixed, adapter-owned choice of server and language for
// one project file. It is never decoded from a request.
type DocumentServer struct {
	Config     ServerConfig
	LanguageID string
	Validate   func(path, text string) error
}

// SessionDocuments adapts approved, already-running SessionManager sessions to
// DocumentProvider. It never launches a process: Session fails with
// unsupported when no adapter claims the path, and approvalRequired /
// staleContext when no current approved session exists.
type SessionDocuments struct {
	Sessions *SessionManager
	// Resolve is supplied by concrete adapters (CD-22…CD-24). Nil resolves nothing.
	Resolve    func(root AuthorizedRoot, path string) (DocumentServer, bool)
	ResolveAll func(root AuthorizedRoot, path string) []DocumentServer
	// Cover reports every already-configured server whose build context can
	// depend on path. It is broader than ResolveAll (exclusive document ownership):
	// a saved Go change may affect a context that does not own the file. Nil
	// means coverage and ownership are the same.
	Cover func(root AuthorizedRoot, path string) []DocumentServer
}

func (d SessionDocuments) servers(root AuthorizedRoot, path string) []DocumentServer {
	if d.ResolveAll != nil {
		return d.ResolveAll(root, path)
	}
	if d.Resolve != nil {
		if server, ok := d.Resolve(root, path); ok {
			return []DocumentServer{server}
		}
	}
	return nil
}

// runningSession returns the already-running, current session for one exact
// configuration. It starts nothing and says nothing about document ownership.
func (d SessionDocuments) runningSession(root AuthorizedRoot, cfg ServerConfig) (*lspSession, error) {
	if d.Sessions == nil {
		return nil, failure("unsupported", "No language server adapter is available")
	}
	d.Sessions.mu.Lock()
	e := d.Sessions.entries[keyOf(root, cfg)]
	var s *lspSession
	if e != nil {
		s = e.session
	}
	d.Sessions.mu.Unlock()
	if s == nil || !live(s.phaseNow()) || !s.current() {
		return nil, failure("staleContext", "Language session is not running")
	}
	return s, nil
}

func (d SessionDocuments) serverFor(root AuthorizedRoot, path string, owner DocumentOwnerIdentity) (DocumentServer, *lspSession, error) {
	if d.Sessions == nil {
		return DocumentServer{}, nil, failure("unsupported", "No language server adapter is available")
	}
	servers := d.servers(root, path)
	for _, server := range servers {
		if owner.ProviderID != "" && owner != (DocumentOwnerIdentity{server.Config.ProviderID, server.Config.Policy()}) {
			continue
		}
		s, err := d.runningSession(root, server.Config)
		if err != nil {
			return DocumentServer{}, nil, err
		}
		return server, s, nil
	}
	return DocumentServer{}, nil, failure("unsupported", "No language server adapter is available for this file")
}

// ValidateDocument checks the displayed text and the saved header against the
// approved context that owns path. A zero owner checks every resolved owner.
// It starts nothing and never selects another context.
func (d SessionDocuments) ValidateDocument(root AuthorizedRoot, path string, owner DocumentOwnerIdentity, text string) error {
	for _, server := range d.servers(root, path) {
		if owner.ProviderID != "" && owner != (DocumentOwnerIdentity{server.Config.ProviderID, server.Config.Policy()}) {
			continue
		}
		if server.Validate != nil {
			if err := server.Validate(path, text); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d SessionDocuments) server(root AuthorizedRoot, path string) (DocumentServer, *lspSession, error) {
	if len(d.servers(root, path)) != 1 {
		return DocumentServer{}, nil, failure("unsupported", "No unique language server owns this file")
	}
	return d.serverFor(root, path, DocumentOwnerIdentity{})
}

func (d SessionDocuments) OwnerSessions(root AuthorizedRoot, path string) []DocumentSession {
	var out []DocumentSession
	for _, server := range d.servers(root, path) {
		owner := DocumentOwnerIdentity{server.Config.ProviderID, server.Config.Policy()}
		_, s, err := d.serverFor(root, path, owner)
		entry := DocumentSession{Owner: owner, Err: err}
		if err == nil {
			info := s.info()
			entry.Generation, entry.Capabilities = s.generation, info.Capabilities
			if info.Phase == phaseStarting {
				entry.Err = failure("busy", "Language session is starting")
			}
		}
		out = append(out, entry)
	}
	return out
}

// OwnerSession preserves the resolver's concrete provider and exact policy.
func (d SessionDocuments) OwnerSession(root AuthorizedRoot, path string) (DocumentOwnerIdentity, string, ServerCapabilities, error) {
	server, s, err := d.server(root, path)
	if err != nil {
		return DocumentOwnerIdentity{}, "", ServerCapabilities{}, err
	}
	info := s.info()
	if info.Phase == phaseStarting {
		return DocumentOwnerIdentity{}, "", ServerCapabilities{}, failure("busy", "Language session is starting")
	}
	return DocumentOwnerIdentity{server.Config.ProviderID, server.Config.Policy()}, s.generation, info.Capabilities, nil
}

// Session reports the current generation and negotiated capabilities.
func (d SessionDocuments) Session(root AuthorizedRoot, path string) (string, ServerCapabilities, error) {
	_, s, err := d.server(root, path)
	if err != nil {
		return "", ServerCapabilities{}, err
	}
	info := s.info()
	if info.Phase == phaseStarting {
		return "", ServerCapabilities{}, failure("busy", "Language session is starting")
	}
	return s.generation, info.Capabilities, nil
}

// maxPublicationURIs bounds transport-lifetime URI history, independently of the
// app's closed-ID FIFO. Exhaustion disables all path publications until restart.
const maxPublicationURIs = 256

// openLifecycle records an open attempt for uri and starts its wire versions:
// at zero for the URI's first open on this transport, otherwise above every
// version the transport has sent, so no earlier lifecycle's publication can
// name a version of this one.
func (s *lspSession) openLifecycle(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := s.recordPublicationURILocked(uri)
	base := int64(0)
	if !first {
		base = s.wireHigh
	}
	if s.wireBases == nil {
		s.wireBases = make(map[string]int64)
	}
	s.wireBases[uri] = base
}

// recordPublicationURILocked reports whether this is the URI's first recorded
// open. Once the history is full nothing is first: certification of later
// opens rests on wire versions alone.
func (s *lspSession) recordPublicationURILocked(uri string) bool {
	if s.publicationFull {
		return false
	}
	if reused, seen := s.publicationURIs[uri]; seen {
		if !reused {
			s.publicationURIs[uri] = true
		}
		return false
	}
	if len(s.publicationURIs) >= maxPublicationURIs {
		s.publicationFull = true
		return false
	}
	if s.publicationURIs == nil {
		s.publicationURIs = make(map[string]bool)
	}
	s.publicationURIs[uri] = false
	return true
}

// wireVersion is the version sent for a document version of the open
// lifecycle of uri. It is recorded before delivery: an interrupted write may
// still have reached the server.
func (s *lspSession) wireVersion(uri string, version int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, ok := s.wireBases[uri]
	if !ok {
		return 0, failure("resyncRequired", "Document is not open on this language session")
	}
	if version < 1 || version > maxSequence-base {
		return 0, failure("limitExceeded", "Document version exceeds the language session range")
	}
	wire := base + version
	s.wireHigh = max(s.wireHigh, wire)
	return wire, nil
}

func (s *lspSession) closeLifecycle(uri string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.wireBases, uri)
}

// PublicationVersionFor maps a versioned publication for path to the document
// version of its open lifecycle on the current transport. It fails when the
// version was not sent by that lifecycle, which also rejects every late
// publication from an earlier lifecycle of a reopened URI.
func (d SessionDocuments) PublicationVersionFor(root AuthorizedRoot, path, providerID, generation string, wire int64) (int64, bool) {
	s := d.publicationSession(root, path, providerID, generation)
	if s == nil {
		return 0, false
	}
	target, err := root.ResolvePath(root.Project().HostID, path)
	if err != nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	base, ok := s.wireBases[fileURI(target)]
	if !ok || wire <= base {
		return 0, false
	}
	return wire - base, true
}

func (d SessionDocuments) publicationSession(root AuthorizedRoot, path, providerID, generation string) *lspSession {
	for _, entry := range d.OwnerSessions(root, path) {
		if entry.Err == nil && entry.Owner.ProviderID == providerID && entry.Generation == generation {
			_, s, _ := d.serverFor(root, path, entry.Owner)
			return s
		}
	}
	return nil
}

// PublicationIsolated proves that exactly one open was attempted for this wire
// URI on the current transport. Matching versions cannot provide this proof.
func (d SessionDocuments) PublicationIsolated(root AuthorizedRoot, path, generation string) bool {
	entries := d.OwnerSessions(root, path)
	if len(entries) != 1 {
		return false
	}
	return d.PublicationIsolatedFor(root, path, entries[0].Owner.ProviderID, generation)
}

func (d SessionDocuments) PublicationIsolatedFor(root AuthorizedRoot, path, providerID, generation string) bool {
	s := d.publicationSession(root, path, providerID, generation)
	if s == nil {
		return false
	}
	target, err := root.ResolvePath(root.Project().HostID, path)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	reused, seen := s.publicationURIs[fileURI(target)]
	return seen && !reused && !s.publicationFull
}

// maxWatchedChanges bounds one workspace/didChangeWatchedFiles notification.
const maxWatchedChanges = 256

// WatchedFiles forwards app-watcher saved-file transitions to the already
// running session that owns these paths. It never launches, accepts no dynamic
// registration and sends only contained paths: created/changed files must
// resolve inside the root; deleted paths are validated portable joins under the
// canonical root. Unsupported/stopped sessions are silently skipped.
func (d SessionDocuments) WatchedFiles(ctx context.Context, root AuthorizedRoot, changes []FileChange) error {
	if len(changes) == 0 {
		return nil
	}
	// Group each change by the exact running session it can affect. Ownership
	// alone is not coverage: another build context of the same provider may
	// compile a file it does not own. A stopped session is skipped, never started.
	type batch struct {
		session *lspSession
		events  []map[string]any
	}
	groups := map[sessionKey]*batch{}
	var first error
	for _, c := range changes {
		if c.Kind < 1 || c.Kind > 3 || ValidatePath(c.Path) != nil {
			continue
		}
		servers := d.servers(root, c.Path)
		if d.Cover != nil {
			servers = d.Cover(root, c.Path)
		}
		for _, server := range servers {
			// Coverage is not ownership: look the exact configured session up
			// directly so a sibling context that does not own the path still
			// receives the change.
			s, err := d.runningSession(root, server.Config)
			if err != nil {
				if first == nil {
					first = err
				}
				continue
			}
			key := keyOf(root, server.Config)
			g := groups[key]
			if g == nil {
				g = &batch{session: s}
				groups[key] = g
			}
			if len(g.events) >= maxWatchedChanges {
				continue
			}
			target := filepath.Join(root.Project().Root, filepath.FromSlash(c.Path))
			if c.Kind != 3 {
				var err error
				if target, err = root.ResolvePath(root.Project().HostID, c.Path); err != nil {
					continue
				}
			}
			g.events = append(g.events, map[string]any{"uri": fileURI(target), "type": c.Kind})
		}
	}
	if len(groups) == 0 {
		if first != nil {
			return first
		}
		return failure("unsupported", "No language server owns these files")
	}
	sent := false
	for _, g := range groups {
		tr := g.session.transport()
		if tr == nil || !g.session.current() || len(g.events) == 0 {
			continue
		}
		if err := tr.Notify(ctx, "workspace/didChangeWatchedFiles", map[string]any{"changes": g.events}); err != nil {
			if first == nil {
				first = failure("disconnected", "Language session dropped the watched-file update")
			}
			continue
		}
		sent = true
	}
	if !sent && first != nil {
		return first
	}
	return nil
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int64  `json:"version"`
	Text       string `json:"text"`
}

// Apply sends one ordered notification. Transport failures are reported as
// bounded errors, so a dropped change is never acknowledged.
func (d SessionDocuments) Apply(ctx context.Context, step DocumentStep) error {
	server, s, err := d.serverFor(step.Root, step.Path, step.Owner)
	if err != nil {
		return err
	}
	if s.generation != step.Generation || !s.current() || step.Owner != (DocumentOwnerIdentity{server.Config.ProviderID, server.Config.Policy()}) {
		return failure("staleContext", "Language session generation changed")
	}
	target, err := step.Root.ResolvePath(step.Root.Project().HostID, step.Path)
	if err != nil {
		return err
	}
	uri := fileURI(target)
	tr := s.transport()
	if tr == nil {
		return failure("disconnected", "Language session is not connected")
	}
	var method string
	var params any
	switch step.Kind {
	case "open":
		// Record before attempting delivery: an interrupted write may have reached
		// the server. A failed open must never make the next open look isolated.
		s.openLifecycle(uri)
		wire, err := s.wireVersion(uri, step.Version)
		if err != nil {
			return err
		}
		method = "textDocument/didOpen"
		params = map[string]any{"textDocument": lspItem{uri, server.LanguageID, wire, step.Text}}
	case "change":
		wire, err := s.wireVersion(uri, step.Version)
		if err != nil {
			return err
		}
		method = "textDocument/didChange"
		changes := make([]map[string]any, 0, len(step.Changes))
		for _, c := range step.Changes {
			item := map[string]any{"text": c.Text}
			if c.Range != nil {
				item["range"] = map[string]any{
					"start": lspPosition{c.Range.Start.Line, c.Range.Start.Character},
					"end":   lspPosition{c.Range.End.Line, c.Range.End.Character},
				}
			}
			changes = append(changes, item)
		}
		params = map[string]any{"textDocument": map[string]any{"uri": uri, "version": wire}, "contentChanges": changes}
	case "save":
		method = "textDocument/didSave" // never includes text, by negotiation and by design
		params = map[string]any{"textDocument": map[string]any{"uri": uri}}
	case "close":
		// The lifecycle ends here even if delivery fails: nothing may certify it.
		s.closeLifecycle(uri)
		method = "textDocument/didClose"
		params = map[string]any{"textDocument": map[string]any{"uri": uri}}
	default:
		return failure("invalidArgument", "Unknown document operation")
	}
	if err := tr.Notify(ctx, method, params); err != nil {
		switch {
		case errors.Is(err, ErrLSPFrameTooLarge):
			return failure("limitExceeded", "Document text exceeds the language session frame limit")
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return failure("canceled", "Document synchronization was interrupted")
		}
		return failure("disconnected", "Language session dropped the document update")
	}
	return nil
}

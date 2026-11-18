package rpc

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
)

func (b *diagnosticsBackend) CompleteCode(ctx context.Context, c cd.Context, id string, v int64, pos cd.Position, trigger cd.CompletionTrigger) (cd.CompletionResult, error) {
	b.completeCalls++
	out, err := b.m.Complete(ctx, c, id, v, pos, trigger)
	if err == nil && b.tamperCompletion != nil {
		b.tamperCompletion(&out)
	}
	return out, err
}

func (b *diagnosticsBackend) ResolveCodeCompletion(ctx context.Context, c cd.Context, id, resultID string, index int, version int64) (cd.CompletionDetails, error) {
	b.completeCalls++
	out, err := b.m.ResolveCompletion(ctx, c, id, resultID, index, version)
	if err == nil && b.tamperDetails != nil {
		b.tamperDetails(&out)
	}
	return out, err
}

// completeSession is a running session that answers completion with a fixed
// list. With hold set, a request waits for its context to end.
type completeSession struct {
	*rpcSession
	mu       sync.Mutex
	items    []cd.CompletionItem
	reqs     []cd.CompletionRequest
	resolves []cd.CompletionResolveRequest
	hold     bool
	entered  chan struct{}
	ended    chan error
	ids      int
}

func (s *completeSession) OwnerSession(root cd.AuthorizedRoot, path string) (cd.DocumentOwnerIdentity, string, cd.ServerCapabilities, error) {
	owner, gen, caps, err := s.rpcSession.OwnerSession(root, path)
	caps.Completion, caps.CompletionTriggers, caps.CompletionResolve = true, ".", true
	return owner, gen, caps, err
}

func (s *completeSession) Complete(ctx context.Context, req cd.CompletionRequest) (cd.CompletionResult, error) {
	s.mu.Lock()
	s.reqs = append(s.reqs, req)
	s.ids++
	id, hold, items := "r"+string(rune('0'+s.ids)), s.hold, slices.Clone(s.items)
	s.mu.Unlock()
	if hold {
		s.entered <- struct{}{}
		<-ctx.Done()
		s.ended <- ctx.Err()
		return cd.CompletionResult{}, &cd.Error{Code: "canceled", Message: "canceled"}
	}
	return cd.CompletionResult{ResultID: id, Items: items}, nil
}

func (s *completeSession) ResolveCompletion(_ context.Context, req cd.CompletionResolveRequest) (cd.CompletionDetails, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolves = append(s.resolves, req)
	out := cd.CompletionDetails{Detail: "func alpha()", Documentation: &cd.MarkupText{Kind: "markdown", Value: "Alpha."}}
	if req.Version > 0 {
		out.ExtraEdits = []cd.TextEdit{importEdit}
	}
	return out, nil
}

// importEdit is what the fake session resolves an item's extra edits to.
var importEdit = cd.TextEdit{Range: cd.Range{Start: cd.Position{Line: 1}, End: cd.Position{Line: 1}}, NewText: "import \"a\"\n"}

func completeFixture(t *testing.T) (*diagnosticsBackend, *Client, cd.Context, *completeSession, context.Context, func()) {
	t.Helper()
	b, c, token, sess, ctx, done := liveFixture(t, true)
	if !c.Capabilities().CodeCompletion {
		t.Fatal("codeCompletion not advertised")
	}
	complete := &completeSession{rpcSession: sess, entered: make(chan struct{}, 1), ended: make(chan error, 1)}
	b.m.SetDocumentProvider(complete)
	if _, err := c.OpenCodeDiagnosticDocument(ctx, token, "doc.go", "d1", 1, "package x\n\nfunc f() { x. }\n", "r1"); err != nil {
		t.Fatal(err)
	}
	return b, c, token, complete, ctx, done
}

func TestCompletionOverRPC(t *testing.T) {
	b, c, token, sess, ctx, done := completeFixture(t)
	defer done()
	at := cd.Position{Line: 2, Character: 13}
	sess.items = []cd.CompletionItem{
		{Label: "alpha", Kind: 3, Edit: &cd.TextEdit{Range: cd.Range{Start: at, End: at}, NewText: "alpha()"}},
		{Label: "beta", Kind: 6, Detail: "int", Documentation: &cd.MarkupText{Kind: "plaintext", Value: "Beta."}},
	}
	dot := cd.CompletionTrigger{Kind: cd.CompletionCharacter, Character: "."}
	got, err := c.CompleteCode(ctx, token, "d1", 1, at, dot)
	if err != nil || got.DocumentID != "d1" || got.Version != 1 || got.ResultID != "r1" || got.TriggerCharacters != "." || len(got.Items) != 2 || got.Items[0].Edit.NewText != "alpha()" || got.Items[1].Documentation.Value != "Beta." {
		t.Fatal(got, err)
	}
	if req := sess.reqs[0]; req.Path != "doc.go" || req.Position != at || req.Trigger != dot || req.Text != "package x\n\nfunc f() { x. }\n" {
		t.Fatalf("%+v", req)
	}
	details, err := c.ResolveCodeCompletion(ctx, token, "d1", "r1", 1, 0)
	if err != nil || !reflect.DeepEqual(details, cd.CompletionDetails{DocumentID: "d1", ResultID: "r1", Index: 1, Detail: "func alpha()", Documentation: details.Documentation}) || details.Documentation.Value != "Alpha." {
		t.Fatal(details, err)
	}
	if req := sess.resolves[0]; req.Path != "doc.go" || req.ResultID != "r1" || req.Index != 1 || req.Version != 0 {
		t.Fatalf("%+v", req)
	}
	// At the current version, the version and the edits cross the wire.
	if details, err = c.ResolveCodeCompletion(ctx, token, "d1", "r1", 1, 1); err != nil || details.Version != 1 || !reflect.DeepEqual(details.ExtraEdits, []cd.TextEdit{importEdit}) {
		t.Fatal(details, err)
	}
	if req := sess.resolves[1]; req.Version != 1 || req.Text != "package x\n\nfunc f() { x. }\n" {
		t.Fatalf("%+v", req)
	}
	sess.items = nil
	if got, err = c.CompleteCode(ctx, token, "d1", 1, at, cd.CompletionTrigger{Kind: cd.CompletionInvoked}); err != nil || got.Items == nil || len(got.Items) != 0 {
		t.Fatal(got, err)
	}
	// A character the server did not name is answered with nothing, and its own named, without asking it.
	asked := len(sess.reqs)
	paren := cd.CompletionTrigger{Kind: cd.CompletionCharacter, Character: "("}
	if got, err = c.CompleteCode(ctx, token, "d1", 1, at, paren); err != nil || len(got.Items) != 0 || got.TriggerCharacters != "." || len(sess.reqs) != asked {
		t.Fatal(got, err)
	}

	// The manager's own refusals keep their codes across the wire.
	for name, tc := range map[string]struct {
		call func() error
		code string
	}{
		"old version": {func() error { _, e := c.CompleteCode(ctx, token, "d1", 2, at, dot); return e }, "staleVersion"},
		"outside":     {func() error { _, e := c.CompleteCode(ctx, token, "d1", 1, cd.Position{Line: 40}, dot); return e }, "invalidArgument"},
		"trigger": {func() error {
			_, e := c.CompleteCode(ctx, token, "d1", 1, at, cd.CompletionTrigger{Kind: cd.CompletionCharacter, Character: "(("})
			return e
		}, "invalidArgument"},
		"replaced": {func() error { _, e := c.ResolveCodeCompletion(ctx, token, "d1", "r1", 0, 0); return e }, "staleVersion"},
		"index":    {func() error { _, e := c.ResolveCodeCompletion(ctx, token, "d1", "r2", 5, 0); return e }, "invalidArgument"},
		"negative": {func() error { _, e := c.ResolveCodeCompletion(ctx, token, "d1", "r2", 0, -1); return e }, "invalidArgument"},
	} {
		if err := tc.call(); rpcCode(err) != tc.code {
			t.Fatal(name, err)
		}
	}

	// A peer that bypasses the client: a missing position or trigger, an
	// overlong result ID and a foreign context never reach the backend.
	calls := b.completeCalls
	var out cd.CompletionResult
	for _, p := range []codeDiagnosticsParams{
		{Context: token, DocumentID: "d1", Version: 1, Trigger: &dot},
		{Context: token, DocumentID: "d1", Version: 1, Position: &at},
	} {
		if err = c.call(ctx, "completeCode", p, &out); rpcCode(err) != "invalidArgument" {
			t.Fatal(p, err)
		}
	}
	var resolved cd.CompletionDetails
	if err = c.call(ctx, "resolveCodeCompletion", codeDiagnosticsParams{Context: token, DocumentID: "d1", ResultID: strings.Repeat("r", maxLiveIDBytes+1)}, &resolved); rpcCode(err) != "invalidArgument" {
		t.Fatal(err)
	}
	foreign := token
	foreign.ConnectionID = "other"
	if err = c.call(ctx, "completeCode", codeDiagnosticsParams{Context: foreign, DocumentID: "d1", Version: 1, Position: &at, Trigger: &dot}, &out); rpcCode(err) != "staleContext" {
		t.Fatal(err)
	}
	if err = c.call(ctx, "resolveCodeCompletion", codeDiagnosticsParams{Context: foreign, DocumentID: "d1", ResultID: "r2"}, &resolved); rpcCode(err) != "staleContext" {
		t.Fatal(err)
	}
	if b.completeCalls != calls {
		t.Fatal("refused request reached the backend")
	}
}

// An answer the client did not ask for, or one a local server could not have
// given, is refused.
func TestCompletionClientRefusesBadAnswers(t *testing.T) {
	b, c, token, sess, ctx, done := completeFixture(t)
	defer done()
	sess.items = []cd.CompletionItem{{Label: "alpha"}}
	at, invoked := cd.Position{Line: 2, Character: 13}, cd.CompletionTrigger{Kind: cd.CompletionInvoked}
	for name, tc := range map[string]struct {
		tamper func(*cd.CompletionResult)
		code   string
	}{
		"other version":  {func(r *cd.CompletionResult) { r.Version = 7 }, "staleContext"},
		"other document": {func(r *cd.CompletionResult) { r.DocumentID = "d2" }, "staleContext"},
		"no label":       {func(r *cd.CompletionResult) { r.Items[0].Label = "" }, "toolFailed"},
		"odd docs":       {func(r *cd.CompletionResult) { r.Items[0].Documentation = &cd.MarkupText{Kind: "html"} }, "toolFailed"},
		"long result ID": {func(r *cd.CompletionResult) { r.ResultID = strings.Repeat("r", 65) }, "toolFailed"},
		"too many items": {func(r *cd.CompletionResult) { r.Items = make([]cd.CompletionItem, 2001) }, "toolFailed"},
	} {
		b.tamperCompletion = tc.tamper
		if _, err := c.CompleteCode(ctx, token, "d1", 1, at, invoked); rpcCode(err) != tc.code {
			t.Fatal(name, err)
		}
	}
	b.tamperCompletion = nil
	got, err := c.CompleteCode(ctx, token, "d1", 1, at, invoked)
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		tamper func(*cd.CompletionDetails)
		code   string
	}{
		"other item":     {func(d *cd.CompletionDetails) { d.Index = 3 }, "staleContext"},
		"other answer":   {func(d *cd.CompletionDetails) { d.ResultID = "r9" }, "staleContext"},
		"other document": {func(d *cd.CompletionDetails) { d.DocumentID = "d2" }, "staleContext"},
		"odd docs":       {func(d *cd.CompletionDetails) { d.Documentation.Kind = "html" }, "toolFailed"},
		"long detail":    {func(d *cd.CompletionDetails) { d.Detail = strings.Repeat("d", 1025) }, "toolFailed"},
		"other version":  {func(d *cd.CompletionDetails) { d.Version = 7 }, "staleContext"},
		"edits unasked":  {func(d *cd.CompletionDetails) { d.Version, d.ExtraEdits = 0, []cd.TextEdit{importEdit} }, "toolFailed"},
		"bad edit": {func(d *cd.CompletionDetails) {
			d.ExtraEdits[0].Range.End.Line = 0
			d.ExtraEdits[0].Range.Start.Line = 1
		}, "toolFailed"},
		"overlapping": {func(d *cd.CompletionDetails) { d.ExtraEdits = append(d.ExtraEdits, importEdit) }, "toolFailed"},
	} {
		b.tamperDetails = tc.tamper
		if _, err := c.ResolveCodeCompletion(ctx, token, "d1", got.ResultID, 0, 1); rpcCode(err) != tc.code {
			t.Fatal(name, err)
		}
	}
	// A host from before CC-10 answers without the version and edits.
	b.tamperDetails = func(d *cd.CompletionDetails) { d.Version, d.ExtraEdits = 0, nil }
	if d, err := c.ResolveCodeCompletion(ctx, token, "d1", got.ResultID, 0, 1); err != nil || d.ExtraEdits != nil {
		t.Fatal(d, err)
	}
	b.tamperDetails = nil
	// Edits only for the version the document is at.
	if _, err := c.ResolveCodeCompletion(ctx, token, "d1", got.ResultID, 0, 2); rpcCode(err) != "staleVersion" {
		t.Fatal(err)
	}
}

// Canceling the caller's context cancels the server's request.
func TestCompletionCancelOverRPC(t *testing.T) {
	_, c, token, sess, ctx, done := completeFixture(t)
	defer done()
	sess.hold = true
	call, cancel := context.WithCancel(ctx)
	errs := make(chan error, 1)
	go func() {
		_, err := c.CompleteCode(call, token, "d1", 1, cd.Position{Line: 2, Character: 13}, cd.CompletionTrigger{Kind: cd.CompletionInvoked})
		errs <- err
	}()
	select {
	case <-sess.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("request never reached the session")
	}
	cancel()
	select {
	case err := <-sess.ended:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the server's request was not canceled")
	}
	if err := <-errs; err == nil {
		t.Fatal("canceled request answered")
	}
}

// A server without completion: the client does not ask, and a client that
// asks anyway is refused before the backend.
func TestCompletionUnsupportedFailsBeforeAnyWork(t *testing.T) {
	b, c, token, _, ctx, done := completeFixture(t)
	defer done()
	c.mu.Lock()
	c.caps.CodeCompletion = false
	c.mu.Unlock()
	at, invoked := cd.Position{Line: 2, Character: 13}, cd.CompletionTrigger{Kind: cd.CompletionInvoked}
	if _, err := c.CompleteCode(ctx, token, "d1", 1, at, invoked); rpcCode(err) != "unsupported" {
		t.Fatal(err)
	}
	if _, err := c.ResolveCodeCompletion(ctx, token, "d1", "r1", 0, 0); rpcCode(err) != "unsupported" {
		t.Fatal(err)
	}
	// Without live documents the server advertises no completion and
	// refuses a client that asks anyway.
	b2, c2, token2, _, ctx2, done2 := liveFixture(t, false)
	defer done2()
	if c2.Capabilities().CodeCompletion {
		t.Fatal("completion advertised without live documents")
	}
	var out cd.CompletionResult
	if err := c2.call(ctx2, "completeCode", codeDiagnosticsParams{Context: token2, DocumentID: "d1", Version: 1, Position: &at, Trigger: &invoked}, &out); rpcCode(err) != "unsupported" {
		t.Fatal(err)
	}
	var details cd.CompletionDetails
	if err := c2.call(ctx2, "resolveCodeCompletion", codeDiagnosticsParams{Context: token2, DocumentID: "d1", ResultID: "r1"}, &details); rpcCode(err) != "unsupported" {
		t.Fatal(err)
	}
	if b.completeCalls != 0 || b2.completeCalls != 0 {
		t.Fatal("unsupported request reached the backend")
	}
}

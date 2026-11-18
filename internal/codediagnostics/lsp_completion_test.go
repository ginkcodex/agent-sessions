package codediagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const completionInit = `{"capabilities":{"textDocumentSync":{"openClose":true,"change":2,"save":true},"definitionProvider":true,"completionProvider":{"triggerCharacters":[".",":"],"resolveProvider":true}}}`

func edit(r string, text string) string {
	quoted, _ := json.Marshal(text)
	return `{"range":` + r + `,"newText":` + string(quoted) + `}`
}

// completionFixture is a fake gopls answering with init, and a Manager with
// source.go open as d1 at version 1.
type completionFixture struct {
	e    *sessionEnv
	srv  *navServer
	root AuthorizedRoot
	docs SessionDocuments
	m    *Manager
	c    Context
}

func newCompletionFixture(t *testing.T, init, text string) completionFixture {
	t.Helper()
	e := newEnv(t, SessionOptions{})
	root, cfg := e.project()
	cfg.ProviderID = goplsProviderID
	e.approve(root, cfg)
	srv := &navServer{answers: map[string]string{}, init: init}
	e.script = srv.script
	if err := os.WriteFile(filepath.Join(root.Project().Root, "source.go"), []byte("package a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs := SessionDocuments{Sessions: e.mgr, Resolve: func(AuthorizedRoot, string) (DocumentServer, bool) {
		return DocumentServer{Config: cfg, LanguageID: "go"}, true
	}}
	if _, err := e.mgr.Attach(context.Background(), root, cfg); err != nil {
		t.Fatal(err)
	}
	e.next()
	m := NewManagerForTest(t, root)
	m.SetDocumentProvider(docs)
	if _, err := m.OpenDocument(context.Background(), m.snap.Context, "source.go", "d1", 1, text, ""); err != nil {
		t.Fatal(err)
	}
	return completionFixture{e: e, srv: srv, root: root, docs: docs, m: m.Manager, c: m.snap.Context}
}

func TestCompletionAgainstFakeServer(t *testing.T) {
	f := newCompletionFixture(t, completionInit, "package a\n\nfunc main() { fmt.Pr }\n")
	e, srv, root, m, ctx, c := f.e, f.srv, f.root, f.m, context.Background(), f.c
	at := Position{Line: 2, Character: 20} // after "fmt.Pr"
	invoked := CompletionTrigger{Kind: CompletionInvoked}
	complete := func(trigger CompletionTrigger) (CompletionResult, error) {
		return m.Complete(ctx, c, "d1", 1, at, trigger)
	}

	extras := make([]string, maxCompletionExtraEdits+1)
	for i := range extras {
		extras[i] = edit(navRange(0, i, 0, i+1), "x")
	}
	items := []string{
		// Kept, without its command, commit characters and data.
		`{"label":"Println","kind":3,"detail":"func(a ...any)","documentation":{"kind":"markdown","value":"Println formats."},` +
			`"sortText":"0001","filterText":"Println","labelDetails":{"detail":"(a ...any)","description":"fmt"},"tags":[1],` +
			`"textEdit":` + edit(navRange(2, 18, 2, 20), "Println") + `,"additionalTextEdits":[` + edit(navRange(0, 9, 0, 9), "\n\nimport \"fmt\"") + `],` +
			`"command":{"title":"t","command":"run"},"commitCharacters":["("],"data":{"secret":1}}`,
		// Insert text only; an unknown kind is none.
		`{"label":"Printf","insertText":"Printf","insertTextFormat":2,"kind":99,"deprecated":true,"documentation":"Printf formats."}`,
		// An insert/replace pair: the insert range.
		`{"label":"Print","textEdit":{"insert":` + navRange(2, 18, 2, 20) + `,"replace":` + navRange(2, 18, 2, 21) + `,"newText":"Print"}}`,
		// Left out: another line, not covering the position, inverted,
		// past the end of the line, no label, label over 1 KiB, too many extra
		// edits, an extra edit without text or outside the text.
		`{"label":"line","textEdit":` + edit(navRange(1, 0, 1, 0), "x") + `}`,
		`{"label":"before","textEdit":` + edit(navRange(2, 14, 2, 17), "x") + `}`,
		`{"label":"inverted","textEdit":` + edit(navRange(2, 20, 2, 18), "x") + `}`,
		`{"label":"past","textEdit":` + edit(navRange(2, 18, 2, 40), "x") + `}`,
		`{"kind":1}`,
		`{"label":"` + strings.Repeat("a", maxCompletionShort+1) + `"}`,
		`{"label":"many","additionalTextEdits":[` + strings.Join(extras, ",") + `]}`,
		`{"label":"textless","additionalTextEdits":[{"range":` + navRange(0, 0, 0, 0) + `}]}`,
		`{"label":"outside","additionalTextEdits":[` + edit(navRange(4, 0, 4, 0), "import os\n") + `]}`,
		// Extra edits overlapping each other, or the main edit (here the
		// position), leave the item out; touching is not overlapping.
		`{"label":"overlapping","additionalTextEdits":[` + edit(navRange(0, 0, 0, 2), "a") + `,` + edit(navRange(0, 1, 0, 3), "b") + `]}`,
		`{"label":"over the main","additionalTextEdits":[` + edit(navRange(2, 20, 2, 20), "c") + `]}`,
		`{"label":"touching","additionalTextEdits":[` + edit(navRange(0, 0, 0, 2), "a") + `,` + edit(navRange(0, 2, 0, 2), "d") + `]}`,
		// Long display text is cut; long documentation too.
		`{"label":"long","detail":"` + strings.Repeat("é", maxCompletionShort) + `","documentation":{"kind":"plaintext","value":"` + strings.Repeat("d", maxCompletionDocs+1) + `"}}`,
	}
	srv.set("textDocument/completion", `"result":{"isIncomplete":false,"items":[`+strings.Join(items, ",")+`]}`)
	got, err := complete(invoked)
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{}
	for _, item := range got.Items {
		labels = append(labels, item.Label)
	}
	if got.DocumentID != "d1" || got.Version != 1 || got.ResultID == "" || !got.Resolves || got.Incomplete || !got.Truncated || fmt.Sprint(labels) != "[Println Printf Print touching long]" {
		t.Fatalf("%v %+v", labels, got)
	}
	want := CompletionItem{
		Label: "Println", LabelDetail: "(a ...any)", LabelDescription: "fmt", Kind: 3, Detail: "func(a ...any)",
		Documentation: &MarkupText{Kind: "markdown", Value: "Println formats."}, SortText: "0001", FilterText: "Println",
		Edit:       &TextEdit{Range: Range{Position{2, 18}, Position{2, 20}}, NewText: "Println"},
		ExtraEdits: []TextEdit{{Range: Range{Position{0, 9}, Position{0, 9}}, NewText: "\n\nimport \"fmt\""}},
		Deprecated: true,
	}
	if want, _ := json.Marshal(want); string(must(json.Marshal(got.Items[0]))) != string(want) {
		t.Fatalf("%s", must(json.Marshal(got.Items[0])))
	}
	if p := got.Items[1]; p.Kind != 0 || !p.Snippet || !p.Deprecated || p.InsertText != "Printf" || p.Edit != nil || *p.Documentation != (MarkupText{Kind: "plaintext", Value: "Printf formats."}) {
		t.Fatalf("%+v", p)
	}
	if p := got.Items[2]; p.Edit == nil || p.Edit.Range != (Range{Position{2, 18}, Position{2, 20}}) {
		t.Fatalf("%+v", p)
	}
	if o := got.Items[3]; len(o.ExtraEdits) != 2 || o.ExtraEdits[0].NewText != "a" || o.ExtraEdits[1].NewText != "d" {
		t.Fatalf("%+v", o.ExtraEdits)
	}
	if l := got.Items[4]; len(l.Detail) > maxCompletionShort || !strings.HasPrefix(l.Detail, "éé") || len(l.Documentation.Value) != maxCompletionDocs {
		t.Fatal(len(l.Detail), len(l.Documentation.Value))
	}
	params := srv.last()
	if params["textDocument"].(map[string]any)["uri"] != uriOf(root, "source.go") || params["position"].(map[string]any)["character"].(float64) != 20 ||
		fmt.Sprint(params["context"]) != "map[triggerKind:1]" {
		t.Fatalf("%v", params)
	}

	// A trigger character, and asking again after an incomplete answer.
	srv.set("textDocument/completion", `"result":[{"label":"x"}]`)
	if got, err = complete(CompletionTrigger{Kind: CompletionCharacter, Character: "."}); err != nil || len(got.Items) != 1 || got.Incomplete || got.Truncated || got.TriggerCharacters != ".:" {
		t.Fatal(got, err)
	}
	if fmt.Sprint(srv.last()["context"]) != "map[triggerCharacter:. triggerKind:2]" {
		t.Fatal(srv.last())
	}
	srv.set("textDocument/completion", `"result":null`)
	if got, err = complete(CompletionTrigger{Kind: CompletionIncomplete}); err != nil || got.Items == nil || len(got.Items) != 0 {
		t.Fatal(got, err)
	}
	if fmt.Sprint(srv.last()["context"]) != "map[triggerKind:3]" {
		t.Fatal(srv.last())
	}
	srv.set("textDocument/completion", `"result":{"items":"nope"}`)
	if _, err = complete(invoked); errCode(err) != "toolFailed" {
		t.Fatal(err)
	}

	// At most 2,000 items, and at most 2 MiB kept per answer: the rest asks
	// again as the user types on.
	many := make([]string, maxCompletionItems+1)
	for i := range many {
		many[i] = fmt.Sprintf(`{"label":"item%d"}`, i)
	}
	srv.set("textDocument/completion", `"result":{"isIncomplete":false,"items":[`+strings.Join(many, ",")+`]}`)
	if got, err = complete(invoked); err != nil || len(got.Items) != maxCompletionItems || !got.Incomplete || !got.Truncated || got.Validate() != nil {
		t.Fatal(len(got.Items), got.Incomplete, got.Truncated, err)
	}
	big := make([]string, 200)
	for i := range big {
		big[i] = fmt.Sprintf(`{"label":"big%d","insertText":"%s"}`, i, strings.Repeat("b", maxCompletionText))
	}
	srv.set("textDocument/completion", `"result":[`+strings.Join(big, ",")+`]`)
	if got, err = complete(invoked); err != nil || len(got.Items) == 0 || len(got.Items) >= 200 || !got.Incomplete || !got.Truncated || got.Validate() != nil {
		t.Fatal(len(got.Items), got.Incomplete, got.Truncated, err)
	}
	srv.set("textDocument/completion", `"result":[{"label":"huge","insertText":"`+strings.Repeat("h", maxCompletionText+1)+`"}]`)
	if got, err = complete(invoked); err != nil || len(got.Items) != 0 {
		t.Fatal(got, err)
	}

	// Requests for another version, place or trigger are refused before
	// reaching the server.
	before := len(srv.requests)
	if _, err = m.Complete(ctx, c, "d1", 2, at, invoked); errCode(err) != "staleVersion" {
		t.Fatal(err)
	}
	if _, err = m.Complete(ctx, c, "d1", 1, Position{Line: 9}, invoked); errCode(err) != "invalidArgument" {
		t.Fatal(err)
	}
	for _, trigger := range []CompletionTrigger{{Kind: CompletionCharacter, Character: ".:"}, {Kind: CompletionCharacter}, {Kind: CompletionCharacter, Character: "\n"}, {Kind: CompletionCharacter, Character: "\xff"}, {Kind: "typed"}, {Kind: CompletionInvoked, Character: "."}} {
		if _, err = complete(trigger); errCode(err) != "invalidArgument" {
			t.Fatal(trigger, err)
		}
	}
	// A character the server did not name is answered with nothing, and with
	// the server's trigger characters.
	if got, err = complete(CompletionTrigger{Kind: CompletionCharacter, Character: "("}); err != nil || got.Items == nil || len(got.Items) != 0 ||
		got.ResultID != "" || got.TriggerCharacters != ".:" || got.DocumentID != "d1" || got.Version != 1 || got.Validate() != nil {
		t.Fatal(got, err)
	}
	if _, err = m.Complete(ctx, c, "nope", 1, at, invoked); errCode(err) != "notFound" {
		t.Fatal(err)
	}
	if len(srv.requests) != before {
		t.Fatal("refused request reached the server")
	}

	// The server's own errors.
	srv.set("textDocument/completion", `"error":{"code":-32801,"message":"content modified"}`)
	if _, err = complete(invoked); errCode(err) != "staleVersion" {
		t.Fatal(err)
	}
	srv.set("textDocument/completion", `"error":{"code":-32603,"message":"SECRET"}`)
	if _, err = complete(invoked); errCode(err) != "toolFailed" || strings.Contains(err.Error(), "SECRET") {
		t.Fatal(err)
	}

	// No answer in five seconds: timeout, and the server is told to cancel.
	srv.mu.Lock()
	delete(srv.answers, "textDocument/completion")
	srv.mu.Unlock()
	timers := e.clock.count(completionTimeout)
	done := make(chan error, 1)
	go func() { _, err := complete(invoked); done <- err }()
	eventually(t, "completion timer", func() bool { return e.clock.count(completionTimeout) > timers })
	e.clock.Advance(completionTimeout)
	if err = <-done; errCode(err) != "timeout" {
		t.Fatal(err)
	}
	eventually(t, "cancel sent", func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.cancels == 1 })

	// The caller giving up cancels too.
	cctx, cancel := context.WithCancel(ctx)
	go func() { _, err := m.Complete(cctx, c, "d1", 1, at, invoked); done <- err }()
	eventually(t, "completion sent", func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return len(srv.requests) > before+3 })
	cancel()
	if err = <-done; errCode(err) != "canceled" {
		t.Fatal(err)
	}
	eventually(t, "cancel sent", func() bool { srv.mu.Lock(); defer srv.mu.Unlock(); return srv.cancels == 2 })

	// A stopped server is not running.
	e.mgr.Revoke(root)
	if _, err = complete(invoked); errCode(err) != "notRunning" {
		t.Fatal(err)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// completeDocs adds completion to the fake document provider. Answers are
// r1, r2, ...; when hold is set, the next answer waits for it, and when
// resolveHold is set, the next resolve. A resolve at a version returns edits.
type completeDocs struct {
	MultiOwnedDocumentProvider
	mu          sync.Mutex
	reqs        []CompletionRequest
	resolves    []CompletionResolveRequest
	items       []CompletionItem
	edits       []TextEdit
	answers     int
	hold        chan struct{}
	resolveHold chan struct{}
}

func (n *completeDocs) Complete(_ context.Context, req CompletionRequest) (CompletionResult, error) {
	n.mu.Lock()
	n.reqs = append(n.reqs, req)
	n.answers++
	id, items, hold := fmt.Sprintf("r%d", n.answers), n.items, n.hold
	n.hold = nil
	n.mu.Unlock()
	if hold != nil {
		<-hold
	}
	return CompletionResult{ResultID: id, Items: items}, nil
}

func (n *completeDocs) ResolveCompletion(_ context.Context, req CompletionResolveRequest) (CompletionDetails, error) {
	n.mu.Lock()
	n.resolves = append(n.resolves, req)
	hold, edits := n.resolveHold, n.edits
	n.resolveHold = nil
	n.mu.Unlock()
	if hold != nil {
		<-hold
	}
	out := CompletionDetails{Detail: "resolved"}
	if req.Version > 0 {
		out.ExtraEdits = edits
	}
	return out, nil
}

func (n *completeDocs) resolveCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.resolves)
}

func (n *completeDocs) requests() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.reqs)
}

func TestCompletionChoosesTheLanguageServer(t *testing.T) {
	m, s, p := multiDocFixture(t)
	ruffCaps, pyrightCaps := incrementalCaps, incrementalCaps
	ruffCaps.Hover = true
	pyrightCaps.Definition, pyrightCaps.Hover, pyrightCaps.Completion, pyrightCaps.CompletionTriggers = true, true, true, "."
	p.entries[0].Capabilities, p.entries[1].Capabilities = ruffCaps, pyrightCaps
	ctx, c := context.Background(), s.Context
	if _, err := m.OpenDocument(ctx, c, "source.go", "d1", 1, "x = 1\nx.\n", ""); err != nil {
		t.Fatal(err)
	}
	at, dot := Position{Line: 1, Character: 2}, CompletionTrigger{Kind: CompletionCharacter, Character: "."}
	// Without a completion provider there is nothing to ask.
	if _, err := m.Complete(ctx, c, "d1", 1, at, dot); errCode(err) != "unsupported" {
		t.Fatal(err)
	}
	docs := &completeDocs{MultiOwnedDocumentProvider: p}
	m.SetDocumentProvider(docs)
	docs.items = []CompletionItem{{Label: "real"}}
	got, err := m.Complete(ctx, c, "d1", 1, at, dot)
	if err != nil || len(got.Items) != 1 || got.Items[0].Label != "real" || got.DocumentID != "d1" || got.Version != 1 || got.ResultID != "r1" {
		t.Fatal(got, err)
	}
	// The provider checks ranges against the text the position refers to.
	if req := docs.reqs[len(docs.reqs)-1]; req.Owner.ProviderID != "pyright" || req.Generation != "g-pyright" || req.Path != "source.go" || req.Position != at || req.Trigger != dot || req.Text != "x = 1\nx.\n" || req.Order == 0 {
		t.Fatalf("went to %+v", req)
	}
	// The session orders requests as the Manager does (CC-24).
	order := docs.reqs[len(docs.reqs)-1].Order
	if _, err := m.Complete(ctx, c, "d1", 1, at, dot); err != nil || docs.reqs[len(docs.reqs)-1].Order <= order {
		t.Fatal(docs.reqs, err)
	}
	// Both offer completion: the one with more navigation answers.
	ruffCaps.Completion, ruffCaps.CompletionTriggers = true, "."
	p.entries[0].Capabilities = ruffCaps
	if _, err = m.Complete(ctx, c, "d1", 1, at, dot); err != nil || docs.reqs[len(docs.reqs)-1].Owner.ProviderID != "pyright" {
		t.Fatal(err)
	}
	// The trigger is checked against the server that answers.
	ruffCaps.CompletionTriggers = ":"
	p.entries[0].Capabilities = ruffCaps
	p.entries[1].Err = failure("staleContext", "Language session is not running")
	asked := len(docs.reqs)
	if got, err := m.Complete(ctx, c, "d1", 1, at, dot); err != nil || len(got.Items) != 0 || got.TriggerCharacters != ":" || len(docs.reqs) != asked {
		t.Fatal(got, err)
	}
	if _, err = m.Complete(ctx, c, "d1", 1, at, CompletionTrigger{Kind: CompletionInvoked}); err != nil || docs.reqs[len(docs.reqs)-1].Owner.ProviderID != "ruff" {
		t.Fatal(err)
	}
	// Only Ruff runs, without completion.
	p.entries[0].Capabilities = incrementalCaps
	if _, err = m.Complete(ctx, c, "d1", 1, at, dot); errCode(err) != "notRunning" {
		t.Fatal(err)
	}
	// Pyright restarted and has not seen the document yet.
	p.entries[1].Err, p.entries[1].Generation = nil, "g-pyright-2"
	if _, err = m.Complete(ctx, c, "d1", 1, at, dot); errCode(err) != "resyncRequired" {
		t.Fatal(err)
	}
	// Neither offers completion.
	p.entries[1].Generation, p.entries[1].Capabilities = "g-pyright", incrementalCaps
	before := len(docs.reqs)
	if _, err = m.Complete(ctx, c, "d1", 1, at, CompletionTrigger{Kind: CompletionInvoked}); errCode(err) != "unsupported" || len(docs.reqs) != before {
		t.Fatal(err)
	}
}

func TestNegotiateCompletionCapabilities(t *testing.T) {
	forty := make([]string, 40)
	for i := range forty {
		forty[i] = fmt.Sprintf("%q", string(rune('A'+i)))
	}
	for raw, want := range map[string]ServerCapabilities{
		`{}`:                                             {},
		`{"completionProvider":false}`:                   {},
		`{"completionProvider":true}`:                    {Completion: true},
		`{"completionProvider":{}}`:                      {Completion: true},
		`{"completionProvider":"bogus"}`:                 {},
		`{"completionProvider":{"triggerCharacters":7}}`: {},
		`{"completionProvider":{"triggerCharacters":[".",".","::","",1,"\u0007","é"," "],"resolveProvider":true}}`: {Completion: true, CompletionTriggers: ".é ", CompletionResolve: true},
		`{"completionProvider":{"triggerCharacters":[` + strings.Join(forty, ",") + `]}}`:                          {Completion: true, CompletionTriggers: "ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`"},
	} {
		var caps map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &caps); err != nil {
			t.Fatal(raw, err)
		}
		caps["textDocumentSync"] = json.RawMessage(`1`)
		body, _ := json.Marshal(map[string]any{"capabilities": caps})
		got, err := negotiateCapabilities(body)
		if err != nil || got.Completion != want.Completion || got.CompletionTriggers != want.CompletionTriggers || got.CompletionResolve != want.CompletionResolve {
			t.Fatalf("%s: %+v %v", raw, got, err)
		}
	}
}

func TestCompletionResultValidate(t *testing.T) {
	line := func(a, b int) Range { return Range{Position{0, a}, Position{0, b}} }
	ok := CompletionResult{Items: []CompletionItem{{
		Label: "a", Kind: 6, Edit: &TextEdit{Range: line(0, 1), NewText: "a"},
		ExtraEdits:    []TextEdit{{Range: Range{Position{2, 0}, Position{2, 0}}, NewText: "import a\n"}},
		Documentation: &MarkupText{Kind: "markdown", Value: "doc"},
	}}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*CompletionItem){
		"no label":       func(c *CompletionItem) { c.Label = "" },
		"kind":           func(c *CompletionItem) { c.Kind = 26 },
		"detail":         func(c *CompletionItem) { c.Detail = strings.Repeat("d", maxCompletionShort+1) },
		"docs kind":      func(c *CompletionItem) { c.Documentation.Kind = "html" },
		"multi-line":     func(c *CompletionItem) { c.Edit.Range = Range{Position{0, 0}, Position{1, 0}} },
		"inverted":       func(c *CompletionItem) { c.Edit.Range = line(2, 1) },
		"overlap":        func(c *CompletionItem) { c.ExtraEdits[0].Range = line(0, 0) },
		"insert text":    func(c *CompletionItem) { c.InsertText = strings.Repeat("i", maxCompletionText+1) },
		"too many edits": func(c *CompletionItem) { c.ExtraEdits = make([]TextEdit, maxCompletionExtraEdits+1) },
	} {
		item := ok.Items[0]
		edit, docs := *item.Edit, *item.Documentation
		item.Edit, item.Documentation, item.ExtraEdits = &edit, &docs, append([]TextEdit(nil), item.ExtraEdits...)
		change(&item)
		if err := (CompletionResult{Items: []CompletionItem{item}}).Validate(); errCode(err) != "toolFailed" {
			t.Fatal(name, err)
		}
	}
	if err := (CompletionResult{Items: make([]CompletionItem, maxCompletionItems+1)}).Validate(); err == nil {
		t.Fatal("too many items")
	}
	for _, id := range []string{"", "r1"} {
		if err := (CompletionResult{ResultID: id, Items: []CompletionItem{}}).Validate(); err != nil {
			t.Fatal(id, err)
		}
	}
	for _, id := range []string{strings.Repeat("r", 65), "r\n1"} {
		if err := (CompletionResult{ResultID: id}).Validate(); errCode(err) != "toolFailed" {
			t.Fatal(id, err)
		}
	}
	for _, triggers := range []string{"", ".:", "é ", "ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`"} {
		if err := (CompletionResult{TriggerCharacters: triggers}).Validate(); err != nil {
			t.Fatal(triggers, err)
		}
	}
	for _, triggers := range []string{"..", "a\n", "\xff", "ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`!"} {
		if err := (CompletionResult{TriggerCharacters: triggers}).Validate(); errCode(err) != "toolFailed" {
			t.Fatal(triggers, err)
		}
	}
}

func TestCompletionResolveAgainstFakeServer(t *testing.T) {
	f := newCompletionFixture(t, completionInit, "package a\n\nfunc main() { a }\n")
	srv, m, ctx, c := f.srv, f.m, context.Background(), f.c
	at, invoked := Position{Line: 2, Character: 15}, CompletionTrigger{Kind: CompletionInvoked}
	srv.set("textDocument/completion", `"result":{"isIncomplete":false,"items":[`+
		`{"label":"skipped","textEdit":`+edit(navRange(1, 0, 1, 0), "x")+`},`+
		`{"label":"alpha","detail":"listed","data":{"id":7}},`+
		`{"label":"beta","documentation":"Beta from the list.","data":{"id":8}}]}`)
	got, err := m.Complete(ctx, c, "d1", 1, at, invoked)
	if err != nil || got.ResultID == "" || len(got.Items) != 2 {
		t.Fatal(got, err)
	}
	resolve := func(id string, index int) (CompletionDetails, error) {
		return m.ResolveCompletion(ctx, c, "d1", id, index, 0)
	}

	// The kept item goes back with its data; the index is in the kept items.
	// Without a version, extra edits are not asked for.
	srv.set("completionItem/resolve", `"result":{"label":"alpha","detail":"func alpha()","documentation":{"kind":"markdown","value":"Alpha does."},"data":{"id":7},`+
		`"additionalTextEdits":[`+edit(navRange(0, 0, 0, 0), "ignored")+`]}`)
	details, err := resolve(got.ResultID, 0)
	if err != nil || !reflect.DeepEqual(details, CompletionDetails{DocumentID: "d1", ResultID: got.ResultID, Index: 0, Detail: "func alpha()", Documentation: details.Documentation}) ||
		*details.Documentation != (MarkupText{Kind: "markdown", Value: "Alpha does."}) || details.Validate() != nil {
		t.Fatal(details, err)
	}
	if sent := srv.last(); sent["label"] != "alpha" || fmt.Sprint(sent["data"]) != "map[id:7]" {
		t.Fatal(sent)
	}
	// What the server leaves out keeps the list's value.
	srv.set("completionItem/resolve", `"result":{"label":"beta","detail":"var beta int"}`)
	if details, err = resolve(got.ResultID, 1); err != nil || details.Detail != "var beta int" || details.Documentation == nil || details.Documentation.Value != "Beta from the list." {
		t.Fatal(details, err)
	}
	// Documentation is capped.
	srv.set("completionItem/resolve", `"result":{"label":"beta","documentation":"`+strings.Repeat("d", maxCompletionDocs+1)+`"}`)
	if details, err = resolve(got.ResultID, 1); err != nil || len(details.Documentation.Value) != maxCompletionDocs || !details.Truncated || details.Validate() != nil {
		t.Fatal(len(details.Documentation.Value), details.Truncated, err)
	}
	srv.set("completionItem/resolve", `"error":{"code":-32603,"message":"SECRET"}`)
	if _, err = resolve(got.ResultID, 1); errCode(err) != "toolFailed" || strings.Contains(err.Error(), "SECRET") {
		t.Fatal(err)
	}
	srv.set("completionItem/resolve", `"result":{"label":"alpha"}`)

	// Unknown items and answers are refused before reaching the server.
	before := len(srv.requests)
	for _, bad := range []struct {
		id    string
		index int
		code  string
	}{{got.ResultID, 2, "invalidArgument"}, {got.ResultID, -1, "invalidArgument"}, {"", 0, "invalidArgument"}, {"999", 0, "staleVersion"}} {
		if _, err = resolve(bad.id, bad.index); errCode(err) != bad.code {
			t.Fatal(bad, err)
		}
	}
	if _, err = m.ResolveCompletion(ctx, c, "nope", got.ResultID, 0, 0); errCode(err) != "notFound" {
		t.Fatal(err)
	}
	if len(srv.requests) != before {
		t.Fatal("refused resolve reached the server")
	}

	// A newer answer replaces the kept one.
	next, err := m.Complete(ctx, c, "d1", 1, at, invoked)
	if err != nil || next.ResultID == got.ResultID {
		t.Fatal(next, err)
	}
	if _, err = resolve(got.ResultID, 0); errCode(err) != "staleVersion" {
		t.Fatal(err)
	}
	if _, err = resolve(next.ResultID, 0); err != nil {
		t.Fatal(err)
	}
	// The session keeps its items when the document changes, and drops them
	// when the session restarts (another generation) and when it closes.
	m.mu.Lock()
	kept := m.attachments[c.AttachmentID].docs["d1"].completion
	m.mu.Unlock()
	req := CompletionResolveRequest{Root: f.root, Owner: kept.owner, Generation: kept.generation, Path: "source.go", ResultID: next.ResultID}
	if _, err = f.docs.ResolveCompletion(ctx, req); err != nil {
		t.Fatal(err)
	}
	restarted := req
	restarted.Generation = "another"
	if _, err = f.docs.ResolveCompletion(ctx, restarted); errCode(err) != "staleVersion" {
		t.Fatal(err)
	}
	if _, err = m.ChangeDocument(ctx, c, "d1", 2, "package a\n\nfunc main() { al }\n"); err != nil {
		t.Fatal(err)
	}
	srv.set("completionItem/resolve", `"result":{"label":"alpha","detail":"func alpha()"}`)
	if details, err = resolve(next.ResultID, 0); err != nil || details.Detail != "func alpha()" {
		t.Fatal(details, err)
	}
	if _, err = f.docs.ResolveCompletion(ctx, req); err != nil {
		t.Fatal(err)
	}

	// At the current version, the resolved item's extra edits come too,
	// checked against that version's text.
	at2 := func(index int) (CompletionDetails, error) {
		return m.ResolveCompletion(ctx, c, "d1", next.ResultID, index, 2)
	}
	imports := `"result":{"label":"alpha","additionalTextEdits":[` + edit(navRange(1, 0, 1, 0), "import \"alpha\"\n") + `]}`
	srv.set("completionItem/resolve", imports)
	if details, err = at2(0); err != nil || details.Version != 2 || len(details.ExtraEdits) != 1 || details.ExtraEdits[0] != (TextEdit{Range: Range{Start: Position{Line: 1}, End: Position{Line: 1}}, NewText: "import \"alpha\"\n"}) || details.Validate() != nil {
		t.Fatal(details, err)
	}
	if details, err = resolve(next.ResultID, 0); err != nil || details.Version != 0 || details.ExtraEdits != nil {
		t.Fatal("edits without a version", details, err)
	}
	// Any edit that does not fit the text, or overlaps another, drops them all.
	for name, edits := range map[string]string{
		"outside the text": edit(navRange(1, 0, 1, 0), "a") + `,` + edit(navRange(3, 1, 3, 1), "b"),
		"overlapping":      edit(navRange(1, 0, 1, 0), "import \"alpha\"\n") + `,` + edit(navRange(1, 0, 1, 0), "overlaps"),
		"past the line":    edit(navRange(2, 0, 2, 40), "c"),
		"no text":          `{"range":` + navRange(0, 0, 0, 0) + `}`,
		"too long":         edit(navRange(0, 0, 0, 0), strings.Repeat("x", maxCompletionText+1)),
		"too many":         strings.TrimSuffix(strings.Repeat(edit(navRange(0, 0, 0, 0), "")+",", maxCompletionExtraEdits+1), ","),
	} {
		srv.set("completionItem/resolve", `"result":{"label":"alpha","additionalTextEdits":[`+edits+`]}`)
		if details, err = at2(0); err != nil || details.Version != 0 || details.ExtraEdits != nil || details.Detail != "listed" {
			t.Fatal(name, details, err)
		}
	}
	// Edits are only for the version the document is at.
	srv.set("completionItem/resolve", imports)
	before = len(srv.requests)
	for _, v := range []int64{1, 3, -1} {
		if _, err = m.ResolveCompletion(ctx, c, "d1", next.ResultID, 0, v); errCode(err) == "" {
			t.Fatal(v, "resolved at another version")
		}
	}
	if len(srv.requests) != before {
		t.Fatal("refused resolve reached the server")
	}
	if got, err = m.Complete(ctx, c, "d1", 2, Position{Line: 2, Character: 16}, invoked); err != nil || got.ResultID == "" {
		t.Fatal(got, err)
	}
	_, s, err := f.docs.serverFor(f.root, "source.go", kept.owner)
	if err != nil {
		t.Fatal(err)
	}
	keptURIs := func() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.completions) }
	if keptURIs() != 1 {
		t.Fatal("answer not kept")
	}
	if _, err = m.CloseDocument(ctx, c, "d1"); err != nil {
		t.Fatal(err)
	}
	if keptURIs() != 0 {
		t.Fatal("answer kept after close")
	}
}

// A server without resolve: the list's own detail and documentation.
func TestCompletionResolveWithoutResolveProvider(t *testing.T) {
	// Nor trigger characters: the answer says it has none.
	init := strings.Replace(completionInit, `"triggerCharacters":[".",":"],"resolveProvider":true`, `"triggerCharacters":[]`, 1)
	f := newCompletionFixture(t, init, "package a\n")
	ctx := context.Background()
	f.srv.set("textDocument/completion", `"result":[{"label":"alpha","detail":"`+strings.Repeat("é", maxCompletionShort)+`","documentation":{"kind":"plaintext","value":"Alpha."}}]`)
	got, err := f.m.Complete(ctx, f.c, "d1", 1, Position{Line: 0, Character: 7}, CompletionTrigger{Kind: CompletionInvoked})
	if err != nil || len(got.Items) != 1 || got.ResultID == "" || got.Resolves {
		t.Fatal(got, err)
	}
	var wire map[string]any
	if err = json.Unmarshal(must(json.Marshal(got)), &wire); err != nil || wire["triggerCharacters"] != "" || wire["resolves"] != nil {
		t.Fatal(wire, err)
	}
	before := len(f.srv.requests)
	details, err := f.m.ResolveCompletion(ctx, f.c, "d1", got.ResultID, 0, 0)
	if err != nil || details.Detail != got.Items[0].Detail || !details.Truncated || *details.Documentation != (MarkupText{Kind: "plaintext", Value: "Alpha."}) {
		t.Fatal(details, err)
	}
	// Its edits were in the list, so resolving at the version adds none.
	if details, err = f.m.ResolveCompletion(ctx, f.c, "d1", got.ResultID, 0, 1); err != nil || details.Version != 0 || details.ExtraEdits != nil || details.Validate() != nil {
		t.Fatal(details, err)
	}
	if len(f.srv.requests) != before {
		t.Fatal("resolve reached a server without resolve")
	}
}

func TestCompletionResolveKeepsTheLastAnswer(t *testing.T) {
	m, s, p := multiDocFixture(t)
	caps := incrementalCaps
	caps.Completion = true
	p.entries[1].Capabilities = caps
	docs := &completeDocs{MultiOwnedDocumentProvider: p, items: []CompletionItem{{Label: "a"}, {Label: "b"}}}
	m.SetDocumentProvider(docs)
	ctx, c := context.Background(), s.Context
	if _, err := m.OpenDocument(ctx, c, "source.go", "d1", 1, "x = 1\n", ""); err != nil {
		t.Fatal(err)
	}
	invoked := CompletionTrigger{Kind: CompletionInvoked}
	if _, err := m.Complete(ctx, c, "d1", 1, Position{}, invoked); err != nil {
		t.Fatal(err)
	}
	details, err := m.ResolveCompletion(ctx, c, "d1", "r1", 1, 0)
	if err != nil || !reflect.DeepEqual(details, CompletionDetails{DocumentID: "d1", ResultID: "r1", Index: 1, Detail: "resolved"}) {
		t.Fatal(details, err)
	}
	if req := docs.resolves[0]; req.Owner.ProviderID != "pyright" || req.Generation != "g-pyright" || req.Path != "source.go" || req.ResultID != "r1" || req.Index != 1 || req.Version != 0 || req.Text != "" {
		t.Fatalf("%+v", req)
	}
	// An earlier request answering after a later one does not replace it, and
	// comes back without a result ID.
	hold := make(chan struct{})
	docs.mu.Lock()
	docs.hold = hold
	docs.mu.Unlock()
	done := make(chan CompletionResult, 1)
	go func() { r, _ := m.Complete(ctx, c, "d1", 1, Position{}, invoked); done <- r }()
	eventually(t, "slow request sent", func() bool { return docs.requests() == 2 })
	if r, err := m.Complete(ctx, c, "d1", 1, Position{}, invoked); err != nil || r.ResultID != "r3" {
		t.Fatal(r, err)
	}
	close(hold)
	if r := <-done; r.ResultID != "" || len(r.Items) != 2 {
		t.Fatal(r)
	}
	if _, err = m.ResolveCompletion(ctx, c, "d1", "r2", 0, 0); errCode(err) != "staleVersion" {
		t.Fatal(err)
	}
	if _, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, 0); err != nil {
		t.Fatal(err)
	}
	// At the current version the server gets the version's text, and its
	// edits come back with that version.
	imports := []TextEdit{{NewText: "import x\n"}}
	docs.mu.Lock()
	docs.edits = imports
	docs.mu.Unlock()
	if details, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, 1); err != nil || details.Version != 1 || !reflect.DeepEqual(details.ExtraEdits, imports) {
		t.Fatal(details, err)
	}
	if req := docs.resolves[len(docs.resolves)-1]; req.Version != 1 || req.Text != "x = 1\n" {
		t.Fatalf("%+v", req)
	}
	// A change keeps the answer; only the new version can have edits.
	if _, err = m.ChangeDocument(ctx, c, "d1", 2, "x = 2\n"); err != nil {
		t.Fatal(err)
	}
	if details, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, 0); err != nil || details.Detail != "resolved" || details.ExtraEdits != nil {
		t.Fatal(details, err)
	}
	resolves := docs.resolveCount()
	for _, v := range []int64{1, 3} {
		if _, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, v); errCode(err) != "staleVersion" || docs.resolveCount() != resolves {
			t.Fatal(v, err)
		}
	}
	if details, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, 2); err != nil || details.Version != 2 || len(details.ExtraEdits) != 1 {
		t.Fatal(details, err)
	}
	// A change while the server resolves drops the edits, not the details.
	slow := make(chan struct{})
	docs.mu.Lock()
	docs.resolveHold = slow
	docs.mu.Unlock()
	type answer struct {
		details CompletionDetails
		err     error
	}
	held := make(chan answer, 1)
	go func() { d, err := m.ResolveCompletion(ctx, c, "d1", "r3", 0, 2); held <- answer{d, err} }()
	eventually(t, "slow resolve sent", func() bool { return docs.resolveCount() == resolves+2 })
	if _, err = m.ChangeDocument(ctx, c, "d1", 3, "x = 3\n"); err != nil {
		t.Fatal(err)
	}
	close(slow)
	if a := <-held; a.err != nil || a.details.Detail != "resolved" || a.details.Version != 0 || a.details.ExtraEdits != nil || a.details.Validate() != nil {
		t.Fatal(a.details, a.err)
	}
	// So does a close: the edits fit only the lifecycle asked about.
	slow = make(chan struct{})
	docs.mu.Lock()
	docs.resolveHold = slow
	docs.mu.Unlock()
	go func() { d, err := m.ResolveCompletion(ctx, c, "d1", "r3", 0, 3); held <- answer{d, err} }()
	eventually(t, "slow resolve sent", func() bool { return docs.resolveCount() == resolves+3 })
	if _, err = m.CloseDocument(ctx, c, "d1"); err != nil {
		t.Fatal(err)
	}
	close(slow)
	if a := <-held; a.err != nil || a.details.Version != 0 || a.details.ExtraEdits != nil {
		t.Fatal(a.details, a.err)
	}
	// A close ends the answer.
	if _, err = m.ResolveCompletion(ctx, c, "d1", "r3", 0, 0); err == nil {
		t.Fatal("resolved after close")
	}
}

func TestCompletionDetailsValidate(t *testing.T) {
	ok := CompletionDetails{DocumentID: "d1", ResultID: "1", Index: 3, Detail: "d", Documentation: &MarkupText{Kind: "markdown", Value: "v"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	ok.Version, ok.ExtraEdits = 2, []TextEdit{{NewText: "import a\n"}, {Range: Range{Start: Position{Line: 1}, End: Position{Line: 1, Character: 2}}, NewText: ""}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []CompletionDetails{
		{ResultID: ""},
		{ResultID: "1", Index: -1},
		{ResultID: "1", Index: maxCompletionItems},
		{ResultID: "1", Detail: strings.Repeat("d", maxCompletionShort+1)},
		{ResultID: "1", Documentation: &MarkupText{Kind: "html"}},
		{ResultID: "1", Documentation: &MarkupText{Kind: "plaintext", Value: strings.Repeat("v", maxCompletionDocs+1)}},
		{ResultID: "1", Version: -1},
		{ResultID: "1", Version: 2},
		{ResultID: "1", ExtraEdits: []TextEdit{{NewText: "x"}}},
		{ResultID: "1", Version: 2, ExtraEdits: make([]TextEdit, maxCompletionExtraEdits+1)},
		{ResultID: "1", Version: 2, ExtraEdits: []TextEdit{{Range: Range{Start: Position{Line: 1}, End: Position{Line: 0}}}}},
		{ResultID: "1", Version: 2, ExtraEdits: []TextEdit{{NewText: strings.Repeat("x", maxCompletionText+1)}}},
		{ResultID: "1", Version: 2, ExtraEdits: []TextEdit{{NewText: "\xff"}}},
		{ResultID: "1", Version: 2, ExtraEdits: []TextEdit{{NewText: "a"}, {NewText: "b"}}},
	} {
		if errCode(bad.Validate()) != "toolFailed" {
			t.Fatalf("%+v", bad)
		}
	}
}

// A session keeps at most maxKeptCompletion raw item bytes for resolve:
// keeping an answer drops other URIs' answers, oldest first, and an answer
// larger than the cap keeps only the items that fit.
func TestCompletionKeptBytesAreCapped(t *testing.T) {
	s := &lspSession{wireBases: map[string]int64{"a": 1, "b": 1, "c": 1}}
	quarter := json.RawMessage(`"` + strings.Repeat("x", maxKeptCompletion/4) + `"`)
	keep := func(uri string, items ...json.RawMessage) string {
		t.Helper()
		id := s.beginCompletion(uri, 0)
		if !s.keepCompletion(uri, id, items) {
			t.Fatal("not kept")
		}
		return id
	}
	a := keep("a", quarter)
	b := keep("b", quarter)
	c := keep("c", quarter, quarter) // past the cap with a and b: a goes
	if _, ok := s.keptCompletion("a", a, 0); ok {
		t.Fatal("the oldest answer was kept")
	}
	if _, ok := s.keptCompletion("b", b, 0); !ok {
		t.Fatal("a newer answer was dropped")
	}
	if _, ok := s.keptCompletion("c", c, 1); !ok {
		t.Fatal("the new answer was dropped")
	}
	if n := s.keptCompletionBytes(); n > maxKeptCompletion {
		t.Fatalf("kept %d bytes", n)
	}
	// An answer larger than the cap keeps the items that fit, and drops all
	// others; the rest cannot be resolved.
	b = keep("b", quarter, quarter, quarter, quarter, quarter)
	if _, ok := s.keptCompletion("b", b, 2); !ok {
		t.Fatal("an item within the cap was dropped")
	}
	if _, ok := s.keptCompletion("b", b, 3); ok {
		t.Fatal("an item past the cap was kept")
	}
	answers := 0
	for _, k := range s.completions {
		if k.id != "" {
			answers++
		}
	}
	if _, ok := s.keptCompletion("c", c, 0); ok || answers != 1 || s.keptCompletionBytes() > maxKeptCompletion {
		t.Fatalf("%d answers, %d bytes", answers, s.keptCompletionBytes())
	}
	// A pending request's slot holds no bytes and is not dropped.
	pending := s.beginCompletion("a", 0)
	keep("c", quarter)
	if k := s.completions["a"]; k.pending != pending {
		t.Fatal("a pending slot was dropped")
	}
	// Dropping a kept answer for the cap leaves the request in flight pending.
	keep("a", quarter)
	pending = s.beginCompletion("a", 0)
	keep("b", quarter, quarter, quarter, quarter)
	if k := s.completions["a"]; k.id != "" || k.pending != pending || !s.keepCompletion("a", pending, []json.RawMessage{quarter}) {
		t.Fatalf("%+v", k)
	}
}

// A session keeps the last answer for a URI while a newer request runs, so
// the answer the editor shows still resolves; the newer one replaces it once
// stored, and one that fails or times out leaves it.
func TestCompletionKeptWhileAskingAgain(t *testing.T) {
	s := &lspSession{wireBases: map[string]int64{"a": 1}}
	item := json.RawMessage(`{"label":"x"}`)
	first := s.beginCompletion("a", 0)
	if !s.keepCompletion("a", first, []json.RawMessage{item}) {
		t.Fatal("not kept")
	}
	resolvable := func(id string) bool { _, ok := s.keptCompletion("a", id, 0); return ok }
	// Held, then failed or timed out: the kept answer stays.
	failed := s.beginCompletion("a", 0)
	if !resolvable(first) || resolvable(failed) {
		t.Fatal("a request in flight replaced the kept answer")
	}
	s.dropCompletion("a", failed)
	if !resolvable(first) || s.keepCompletion("a", failed, []json.RawMessage{item}) {
		t.Fatal("a failed request changed the kept answer")
	}
	// Of two requests, only the later one's answer is stored.
	older, newer := s.beginCompletion("a", 0), s.beginCompletion("a", 0)
	if s.keepCompletion("a", older, []json.RawMessage{item}) || !resolvable(first) {
		t.Fatal("an earlier request replaced the kept answer")
	}
	s.dropCompletion("a", older) // its failure path: a no-op now
	if !s.keepCompletion("a", newer, []json.RawMessage{item}) || resolvable(first) || !resolvable(newer) {
		t.Fatal("the newer answer did not replace the kept one")
	}
	// Ordered by the Manager: a request ordered earlier that starts later is
	// not kept, so the session keeps the answer the Manager keeps (CC-24).
	later := s.beginCompletion("a", 7)
	earlier := s.beginCompletion("a", 6)
	if s.keepCompletion("a", earlier, []json.RawMessage{item}) || !resolvable(newer) {
		t.Fatal("a request ordered earlier replaced the kept answer")
	}
	if !s.keepCompletion("a", later, []json.RawMessage{item}) || !resolvable(later) {
		t.Fatal("the latest request's answer was not kept")
	}
	// The order outlives a failed request and the answer it replaced.
	failed = s.beginCompletion("a", 9)
	s.dropCompletion("a", failed)
	if stale := s.beginCompletion("a", 8); s.keepCompletion("a", stale, []json.RawMessage{item}) || !resolvable(later) {
		t.Fatal("a request ordered before a failed one was kept")
	}
	// A request for a document that is not open keeps nothing.
	closed := s.beginCompletion("b", 0)
	if s.keepCompletion("b", closed, []json.RawMessage{item}) {
		t.Fatal("kept an answer for a closed document")
	}
}

// The Manager keeps an answer for the document lifecycle it was asked about:
// typing while the server answers keeps it, a close does not.
func TestCompletionKeptThroughTypingWhileAsked(t *testing.T) {
	m, s, p := multiDocFixture(t)
	caps := incrementalCaps
	caps.Completion = true
	p.entries[1].Capabilities = caps
	docs := &completeDocs{MultiOwnedDocumentProvider: p, items: []CompletionItem{{Label: "a"}}, edits: []TextEdit{{NewText: "import x\n"}}}
	m.SetDocumentProvider(docs)
	ctx, c := context.Background(), s.Context
	if _, err := m.OpenDocument(ctx, c, "source.go", "d1", 1, "x = 1\n", ""); err != nil {
		t.Fatal(err)
	}
	invoked := CompletionTrigger{Kind: CompletionInvoked}
	held := func(version int64, during func()) CompletionResult {
		t.Helper()
		hold := make(chan struct{})
		docs.mu.Lock()
		docs.hold = hold
		asked := docs.answers
		docs.mu.Unlock()
		done := make(chan CompletionResult, 1)
		go func() { r, _ := m.Complete(ctx, c, "d1", version, Position{}, invoked); done <- r }()
		eventually(t, "request sent", func() bool { return docs.requests() == asked+1 })
		during()
		close(hold)
		return <-done
	}
	r := held(1, func() {
		if _, err := m.ChangeDocument(ctx, c, "d1", 2, "x = 2\n"); err != nil {
			t.Fatal(err)
		}
	})
	if r.ResultID != "r1" || r.Version != 1 {
		t.Fatal(r)
	}
	if d, err := m.ResolveCompletion(ctx, c, "d1", "r1", 0, 2); err != nil || d.Version != 2 || len(d.ExtraEdits) != 1 {
		t.Fatal(d, err)
	}
	// A close while the server answers: the answer comes back, but is not
	// kept, so it has no result ID.
	r = held(2, func() {
		if _, err := m.CloseDocument(ctx, c, "d1"); err != nil {
			t.Fatal(err)
		}
	})
	if r.ResultID != "" || len(r.Items) != 1 {
		t.Fatal(r)
	}
}

// One check for every completion edit: a server's item and resolve, and a
// peer's answer and details.
func TestValidEdits(t *testing.T) {
	r := func(l1, c1, l2, c2 int) Range {
		return Range{Start: Position{Line: l1, Character: c1}, End: Position{Line: l2, Character: c2}}
	}
	text := "ab😀c\nde\n" // the emoji is 2 UTF-16 units, columns 2–4
	many := func(n int) []TextEdit {
		out := make([]TextEdit, n)
		for i := range out {
			out[i] = TextEdit{Range: r(i, 0, i, 0), NewText: "x"}
		}
		return out
	}
	for _, c := range []struct {
		name  string
		edits []TextEdit
		text  *string // nil: a peer's answer, without the text
		taken []Range
		ok    bool
	}{
		{name: "none", ok: true},
		{name: "inside", edits: []TextEdit{{Range: r(0, 0, 0, 2), NewText: "x"}, {Range: r(1, 0, 1, 2), NewText: "y"}}, text: &text, ok: true},
		{name: "after a surrogate pair", edits: []TextEdit{{Range: r(0, 4, 0, 5)}}, text: &text, ok: true},
		{name: "inside a surrogate pair", edits: []TextEdit{{Range: r(0, 3, 0, 3)}}, text: &text},
		{name: "past the line", edits: []TextEdit{{Range: r(1, 0, 1, 3)}}, text: &text},
		{name: "past the text, with it", edits: []TextEdit{{Range: r(5, 0, 5, 0)}}, text: &text},
		{name: "past the text, without it", edits: []TextEdit{{Range: r(5, 0, 5, 0)}}, ok: true},
		{name: "backwards", edits: []TextEdit{{Range: r(0, 2, 0, 1)}}},
		{name: "negative", edits: []TextEdit{{Range: r(-1, 0, 0, 0)}}},
		{name: "text at the cap", edits: []TextEdit{{NewText: strings.Repeat("x", maxCompletionText)}}, ok: true},
		{name: "text over the cap", edits: []TextEdit{{NewText: strings.Repeat("x", maxCompletionText+1)}}},
		{name: "invalid UTF-8", edits: []TextEdit{{NewText: "\xff"}}},
		{name: "overlapping each other", edits: []TextEdit{{Range: r(0, 0, 0, 2)}, {Range: r(0, 1, 0, 3)}}},
		{name: "two at one place", edits: []TextEdit{{Range: r(0, 1, 0, 1)}, {Range: r(0, 1, 0, 1)}}},
		{name: "side by side", edits: []TextEdit{{Range: r(0, 0, 0, 1)}, {Range: r(0, 1, 0, 2)}}, ok: true},
		{name: "overlapping the main edit", edits: []TextEdit{{Range: r(0, 1, 0, 2)}}, taken: []Range{r(0, 0, 0, 2)}},
		{name: "at the cap", edits: many(maxCompletionExtraEdits), ok: true},
		{name: "over the cap", edits: many(maxCompletionExtraEdits + 1)},
	} {
		var lines *lineIndex
		if c.text != nil {
			lines = &lineIndex{text: *c.text}
		}
		taken, ok := validEdits(c.edits, lines, c.taken)
		if ok != c.ok {
			t.Errorf("%s: ok %v", c.name, ok)
			continue
		}
		if ok && len(taken) != len(c.taken)+len(c.edits) {
			t.Errorf("%s: taken %v", c.name, taken)
		}
	}
	main := r(0, 0, 0, 1)
	if taken, ok := validEdits([]TextEdit{{Range: r(1, 0, 1, 0)}}, nil, []Range{main}); !ok || !reflect.DeepEqual(taken, []Range{main, r(1, 0, 1, 0)}) {
		t.Fatal(taken, ok)
	}

	x, at := "x", r(0, 0, 0, 0)
	for _, c := range []struct {
		name string
		in   []wireEdit
		want []TextEdit
		ok   bool
	}{
		{name: "none", ok: true},
		{name: "whole", in: []wireEdit{{Range: &at, NewText: &x}}, want: []TextEdit{{Range: at, NewText: "x"}}, ok: true},
		{name: "no range", in: []wireEdit{{NewText: &x}}},
		{name: "no text", in: []wireEdit{{Range: &at}}},
		{name: "over the cap", in: make([]wireEdit, maxCompletionExtraEdits+1)},
	} {
		got, ok := wireEdits(c.in)
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("wireEdits %s: %v %v", c.name, got, ok)
		}
	}
}

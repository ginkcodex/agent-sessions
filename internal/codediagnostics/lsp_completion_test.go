package codediagnostics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const completionInit = `{"capabilities":{"textDocumentSync":{"openClose":true,"change":2,"save":true},"definitionProvider":true,"completionProvider":{"triggerCharacters":[".",":"],"resolveProvider":true}}}`

func edit(r string, text string) string {
	quoted, _ := json.Marshal(text)
	return `{"range":` + r + `,"newText":` + string(quoted) + `}`
}

func TestCompletionAgainstFakeServer(t *testing.T) {
	e := newEnv(t, SessionOptions{})
	root, cfg := e.project()
	cfg.ProviderID = goplsProviderID
	e.approve(root, cfg)
	srv := &navServer{answers: map[string]string{}, init: completionInit}
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
	ctx, c := context.Background(), m.snap.Context
	text := "package a\n\nfunc main() { fmt.Pr }\n"
	if _, err := m.OpenDocument(ctx, c, "source.go", "d1", 1, text, ""); err != nil {
		t.Fatal(err)
	}
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
		// edits, an extra edit without text.
		`{"label":"line","textEdit":` + edit(navRange(1, 0, 1, 0), "x") + `}`,
		`{"label":"before","textEdit":` + edit(navRange(2, 14, 2, 17), "x") + `}`,
		`{"label":"inverted","textEdit":` + edit(navRange(2, 20, 2, 18), "x") + `}`,
		`{"label":"past","textEdit":` + edit(navRange(2, 18, 2, 40), "x") + `}`,
		`{"kind":1}`,
		`{"label":"` + strings.Repeat("a", maxCompletionShort+1) + `"}`,
		`{"label":"many","additionalTextEdits":[` + strings.Join(extras, ",") + `]}`,
		`{"label":"textless","additionalTextEdits":[{"range":` + navRange(0, 0, 0, 0) + `}]}`,
		// Extra edits overlapping the main edit (here the position) or an
		// earlier one are dropped; touching is not overlapping.
		`{"label":"overlap","additionalTextEdits":[` + edit(navRange(0, 0, 0, 2), "a") + `,` + edit(navRange(0, 1, 0, 3), "b") + `,` +
			edit(navRange(2, 20, 2, 20), "c") + `,` + edit(navRange(0, 2, 0, 2), "d") + `]}`,
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
	if got.DocumentID != "d1" || got.Version != 1 || got.Incomplete || !got.Truncated || fmt.Sprint(labels) != "[Println Printf Print overlap long]" {
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
	if got, err = complete(CompletionTrigger{Kind: CompletionCharacter, Character: "."}); err != nil || len(got.Items) != 1 || got.Incomplete || got.Truncated {
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
	for _, trigger := range []CompletionTrigger{{Kind: CompletionCharacter, Character: "("}, {Kind: CompletionCharacter, Character: ".:"}, {Kind: CompletionCharacter}, {Kind: "typed"}, {Kind: CompletionInvoked, Character: "."}} {
		if _, err = complete(trigger); errCode(err) != "invalidArgument" {
			t.Fatal(trigger, err)
		}
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

// completeDocs adds completion to the fake document provider.
type completeDocs struct {
	MultiOwnedDocumentProvider
	mu    sync.Mutex
	reqs  []CompletionRequest
	items []CompletionItem
}

func (n *completeDocs) Complete(_ context.Context, req CompletionRequest) (CompletionResult, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reqs = append(n.reqs, req)
	return CompletionResult{Items: n.items}, nil
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
	// Items whose edits do not fit the text the position refers to are left out.
	docs.items = []CompletionItem{
		{Label: "real", Edit: &TextEdit{Range: Range{Position{1, 2}, Position{1, 2}}, NewText: "real"}},
		{Label: "past", Edit: &TextEdit{Range: Range{Position{1, 2}, Position{1, 5}}, NewText: "past"}},
		{Label: "import", ExtraEdits: []TextEdit{{Range: Range{Position{3, 0}, Position{3, 0}}, NewText: "import os\n"}}},
	}
	got, err := m.Complete(ctx, c, "d1", 1, at, dot)
	if err != nil || len(got.Items) != 1 || got.Items[0].Label != "real" || got.DocumentID != "d1" || got.Version != 1 {
		t.Fatal(got, err)
	}
	if req := docs.reqs[len(docs.reqs)-1]; req.Owner.ProviderID != "pyright" || req.Generation != "g-pyright" || req.Path != "source.go" || req.Position != at || req.Trigger != dot {
		t.Fatalf("went to %+v", req)
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
	if _, err = m.Complete(ctx, c, "d1", 1, at, dot); errCode(err) != "invalidArgument" {
		t.Fatal(err)
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
}

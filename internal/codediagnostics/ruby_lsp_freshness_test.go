package codediagnostics

// SYNTHETIC fake servers: they prove what the client sends for ruby-lsp's
// index freshness, not how ruby-lsp reacts (RL-07 checks the real server).

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
)

const freshnessInit = `{"capabilities":{"textDocumentSync":{"openClose":true,"change":2,"save":true},"definitionProvider":true,"referencesProvider":true,"hoverProvider":true,"documentSymbolProvider":true}}`

// freshnessServers records what each fake server receives, by the provider
// named in its first argument.
type freshnessServers struct {
	mu   sync.Mutex
	got  map[string][]map[string]any
	seen chan struct{}
	// symbolError answers documentSymbol with an error.
	symbolError bool
}

func (r *freshnessServers) script(f *fakeSrv) {
	provider := f.inv.Args[0]
	for {
		m, ok := f.read()
		if !ok {
			return
		}
		r.mu.Lock()
		r.got[provider] = append(r.got[provider], m)
		r.mu.Unlock()
		select {
		case r.seen <- struct{}{}:
		default:
		}
		id, _ := json.Marshal(m["id"])
		reply := func(result string) { f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":` + result + `}`) }
		switch method, _ := m["method"].(string); {
		case method == "initialize" && provider == rubocopProviderID:
			reply(defaultInit) // RuboCop offers no navigation (RB-00)
		case method == "initialize":
			reply(freshnessInit)
		case method == "shutdown":
			reply("null")
		case method == "exit":
			_ = f.Terminate()
			return
		case m["id"] != nil && method == "textDocument/documentSymbol" && r.failing():
			f.send(`{"jsonrpc":"2.0","id":` + string(id) + `,"error":{"code":-32603,"message":"synthetic"}}`)
		case m["id"] != nil && (method == "textDocument/documentSymbol" || method == "textDocument/definition"):
			reply("[]")
		}
	}
}

func (r *freshnessServers) failing() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.symbolError
}

// take returns and forgets the methods provider received, with the file name
// each request or watched change named.
func (r *freshnessServers) take(provider string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, m := range r.got[provider] {
		method, _ := m["method"].(string)
		params, _ := m["params"].(map[string]any)
		switch method {
		case "textDocument/documentSymbol", "textDocument/definition":
			uri := params["textDocument"].(map[string]any)["uri"].(string)
			method += " " + uri[strings.LastIndex(uri, "/")+1:]
		case "workspace/didChangeWatchedFiles":
			for _, c := range params["changes"].([]any) {
				uri := c.(map[string]any)["uri"].(string)
				method += " " + uri[strings.LastIndex(uri, "/")+1:]
			}
		}
		out = append(out, method)
	}
	r.got[provider] = nil
	return out
}

func TestRubyLspIndexFreshness(t *testing.T) {
	for _, reindex := range []string{rubyLspReindex, ""} {
		freshness(t, reindex)
	}
}

// freshness drives RuboCop, ruby-lsp (with the given Reindex) and gopls fakes
// through the displayed-document route. With an empty Reindex, ruby-lsp
// stands for every other adapter: no request is added.
func freshness(t *testing.T, reindex string) {
	e := newEnv(t, SessionOptions{})
	root, tools, _ := fixture(t)
	writeTree(t, root.Project().Root, map[string]string{"a.rb": "", "b.rb": "", "c.rb": "", "x.go": "", "Gemfile": ""})
	rec := &freshnessServers{got: map[string][]map[string]any{}, seen: make(chan struct{}, 1)}
	e.script = rec.script
	state := func(id string) ProviderState {
		return ProviderState{ProviderID: id, Availability: "available", Phase: "idle", Freshness: "uncertain", Coverage: Coverage{Kind: "openedDocuments", Roots: []string{""}}}
	}
	server := func(id string) ServerConfig {
		return ServerConfig{ProviderID: id, Executable: tools[0].Executable, Args: []string{id}, AdapterPolicy: "fake-" + id}
	}
	lsp := server(rubyLspProviderID)
	lsp.Reindex = reindex
	ruby := func(file string) (string, bool) { return RubyLanguageID(file) }
	regs := []Registration{
		liveRegistration(state(rubocopProviderID), server(rubocopProviderID), ruby),
		liveRegistration(state(rubyLspProviderID), lsp, ruby),
		liveRegistration(state(goplsProviderID), server(goplsProviderID), func(file string) (string, bool) { return "go", strings.HasSuffix(file, ".go") }),
	}
	m, err := NewManager("owner", e.trust, NewSupervisor(e.trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	d := NewLiveDocuments(e.trust, m, e.mgr.options)
	t.Cleanup(func() { m.Close(); d.Close() })
	if err := d.Register(root, regs); err != nil {
		t.Fatal(err)
	}
	m.SetDocumentProvider(d)
	s, err := m.Attach(root, 1, regs)
	if err != nil {
		t.Fatal(err)
	}
	if s, err = m.Approve(s.Context, s.Proposal.Fingerprint, "session"); err != nil {
		t.Fatal(err)
	}
	ctx, c := context.Background(), s.Context
	step := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	open := func(path, id string) { _, err := m.OpenDocument(ctx, c, path, id, 1, "x", ""); step(err) }
	change := func(id string, v int64) { _, err := m.ChangeDocument(ctx, c, id, v, "def m; end\n"); step(err) }
	navigate := func(id string, v int64) []string {
		t.Helper()
		_, err := m.Navigate(ctx, c, id, v, NavigateDefinition, Position{})
		step(err)
		if got := rec.take(rubocopProviderID); slices.ContainsFunc(got, func(s string) bool {
			return !strings.HasPrefix(s, "textDocument/did") && s != "initialize" && s != "initialized"
		}) {
			t.Fatalf("RuboCop got %v", got)
		}
		var sent []string
		for _, m := range rec.take(rubyLspProviderID) {
			if strings.HasPrefix(m, "textDocument/documentSymbol") || strings.HasPrefix(m, "textDocument/definition") {
				sent = append(sent, m)
			}
		}
		return sent
	}
	want := func(reindexed ...string) []string {
		var w []string
		if reindex != "" {
			for _, f := range reindexed {
				w = append(w, "textDocument/documentSymbol "+f)
			}
		}
		return w
	}
	check := func(name string, got, want []string) {
		t.Helper()
		if !slices.Equal(got, want) {
			t.Fatalf("%q %s: %v, want %v", reindex, name, got, want)
		}
	}

	open("a.rb", "a")
	check("after open", navigate("a", 1), append(want("a.rb"), "textDocument/definition a.rb"))
	check("unchanged", navigate("a", 1), []string{"textDocument/definition a.rb"})
	change("a", 2)
	check("after change", navigate("a", 2), append(want("a.rb"), "textDocument/definition a.rb"))
	// Every changed open document, in URI order, before the request.
	open("b.rb", "b")
	change("a", 3)
	check("two changed", navigate("b", 1), append(want("a.rb", "b.rb"), "textDocument/definition b.rb"))
	// A closed document is not asked about.
	open("c.rb", "c")
	_, err = m.CloseDocument(ctx, c, "c")
	step(err)
	check("closed", navigate("a", 3), []string{"textDocument/definition a.rb"})
	// An error answer still lets navigation run, and is not asked again.
	rec.mu.Lock()
	rec.symbolError = true
	rec.mu.Unlock()
	change("a", 4)
	check("error answer", navigate("a", 4), append(want("a.rb"), "textDocument/definition a.rb"))
	check("not asked again", navigate("a", 4), []string{"textDocument/definition a.rb"})
	rec.mu.Lock()
	rec.symbolError = false
	rec.mu.Unlock()
	// A request that was not answered (canceled) stays due.
	if reindex != "" {
		change("a", 5)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := m.Navigate(canceled, c, "a", 5, NavigateDefinition, Position{}); errCode(err) != "canceled" {
			t.Fatal(err)
		}
		rec.take(rubyLspProviderID)
		check("still due", navigate("a", 5), append(want("a.rb"), "textDocument/definition a.rb"))
	}

	// Saved transitions: Ruby files to ruby-lsp, Go files to gopls, nothing to
	// RuboCop; other files to nobody.
	open("x.go", "g")
	rec.take(goplsProviderID)
	step(d.WatchedFiles(ctx, root, []FileChange{{Path: "c.rb", Kind: 2}, {Path: "gone.rb", Kind: 3}, {Path: "Gemfile", Kind: 2}, {Path: "x.go", Kind: 3}, {Path: "notes.txt", Kind: 2}}))
	watched := func(provider string) []string {
		var w []string
		for _, m := range rec.take(provider) {
			if strings.HasPrefix(m, "workspace/didChangeWatchedFiles") {
				w = append(w, m)
			}
		}
		return w
	}
	var lspWatched, goWatched []string
	eventually(t, "watched files", func() bool {
		lspWatched, goWatched = append(lspWatched, watched(rubyLspProviderID)...), append(goWatched, watched(goplsProviderID)...)
		return len(lspWatched) > 0 && len(goWatched) > 0
	})
	check("ruby-lsp watched", lspWatched, []string{"workspace/didChangeWatchedFiles c.rb gone.rb"})
	check("gopls watched", goWatched, []string{"workspace/didChangeWatchedFiles x.go"})
	if got := watched(rubocopProviderID); len(got) != 0 {
		t.Fatalf("RuboCop got %v", got)
	}
}

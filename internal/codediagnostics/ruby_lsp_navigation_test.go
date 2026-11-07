package codediagnostics

// SYNTHETIC: a fake server replays ruby-lsp's recorded definition answers
// (testdata/ruby-lsp/evidence.json, case C1) with the placeholders replaced
// by temporary directories. Nothing is executed.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// rubyLspCase is one recorded case of testdata/ruby-lsp/evidence.json.
type rubyLspCase struct {
	Initialize json.RawMessage `json:"initialize"`
	Steps      []struct {
		Do       string          `json:"do"`
		Path     string          `json:"path"`
		Position Position        `json:"position"`
		Result   json.RawMessage `json:"result"`
	} `json:"steps"`
}

func rubyLspEvidence(t *testing.T) (map[string]rubyLspCase, map[string]any) {
	t.Helper()
	data, err := os.ReadFile("testdata/ruby-lsp/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Fixture map[string]any         `json:"fixture"`
		Cases   map[string]rubyLspCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatal(err)
	}
	return evidence.Cases, evidence.Fixture
}

// rubyLspDefinitions are C1's non-empty definition answers, in order.
func rubyLspDefinitions(t *testing.T) []json.RawMessage {
	t.Helper()
	cases, _ := rubyLspEvidence(t)
	var out []json.RawMessage
	for _, s := range cases["C1-wrapper-navigation-only"].Steps {
		if s.Do == "definition" && string(s.Result) != "[]" {
			out = append(out, s.Result)
		}
	}
	return out
}

// ruby-lsp's answers into the standard library, the interpreter's bundled
// gems and the rbs gem's core types are granted for reading; a place outside
// Ruby's library and the GEM_PATH directories is left out.
func TestRubyLspExternalLocations(t *testing.T) {
	e := newEnv(t, SessionOptions{})
	root, cfg := e.project()
	prefix, gems, elsewhere := t.TempDir(), t.TempDir(), t.TempDir()
	cfg.ProviderID = rubyLspProviderID
	cfg.ExternalRoots = []string{filepath.Join(prefix, "lib", "ruby"), gems}
	e.approve(root, cfg)
	srv := &navServer{answers: map[string]string{}}
	e.script = srv.script
	dir := root.Project().Root
	writeTree(t, dir, map[string]string{"app.rb": "x = 1\n", "lib/greeter.rb": "class Greeter\nend\n"})
	docs := SessionDocuments{Sessions: e.mgr, Resolve: func(AuthorizedRoot, string) (DocumentServer, bool) {
		return DocumentServer{Config: cfg, LanguageID: "ruby"}, true
	}}
	if _, err := e.mgr.Attach(context.Background(), root, cfg); err != nil {
		t.Fatal(err)
	}
	e.next()
	m := NewManagerForTest(t, root)
	m.SetDocumentProvider(docs)
	ctx, c := context.Background(), m.snap.Context
	if _, err := m.OpenDocument(ctx, c, "app.rb", "d1", 1, "x = 1\n", ""); err != nil {
		t.Fatal(err)
	}
	replay := strings.NewReplacer(
		"file:///fixture/", fileURI(dir)+"/",
		"file://<ruby prefix>/", fileURI(prefix)+"/",
		"file://<spike gem dir>/", fileURI(gems)+"/",
	)
	define := func(answer string) NavigationResult {
		t.Helper()
		srv.set("textDocument/definition", `"result":`+answer)
		got, err := m.Navigate(ctx, c, "d1", 1, NavigateDefinition, Position{})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	var external []string
	for _, raw := range rubyLspDefinitions(t) {
		var sent []json.RawMessage
		if err := json.Unmarshal(raw, &sent); err != nil {
			t.Fatal(err)
		}
		got := define(replay.Replace(string(raw)))
		if len(got.Locations) != len(sent) {
			t.Fatalf("%d of %d kept: %s", len(got.Locations), len(sent), raw)
		}
		for _, l := range got.Locations {
			if l.External {
				external = append(external, l.Path)
			} else if l.Path != "lib/greeter.rb" {
				t.Fatal("project place", l)
			}
		}
	}
	// The recorded answers reach all three kinds of place.
	for _, want := range []string{
		filepath.Join(prefix, "lib/ruby/4.0.0/openssl/pkey.rb"),
		filepath.Join(prefix, "lib/ruby/gems/4.0.0/gems/parallel-2.3.0/lib/parallel.rb"),
		filepath.Join(gems, "gems/rbs-4.2.0/core/string.rbs"),
	} {
		if !slices.Contains(external, want) {
			t.Fatal("not answered:", want, external)
		}
	}
	for _, path := range external {
		if err := m.NavigationGrant(c, path); err != nil {
			t.Fatal(path, err)
		}
	}

	// Outside the roots: another directory, a sibling of lib/ruby that shares
	// its name as a prefix, and the interpreter itself.
	inside := filepath.Join(gems, "gems/racc-1.8.1/lib/racc.rb")
	var answer []string
	for _, path := range []string{filepath.Join(elsewhere, "lib/evil.rb"), filepath.Join(prefix, "lib/ruby-other/x.rb"), filepath.Join(prefix, "bin/ruby"), inside} {
		answer = append(answer, `{"uri":"`+fileURI(path)+`","range":`+navRange(0, 0, 0, 1)+`}`)
	}
	got := define("[" + strings.Join(answer, ",") + "]")
	if len(got.Locations) != 1 || got.Locations[0] != (NavigationLocation{Path: inside, Range: Range{Position{0, 0}, Position{0, 1}}, External: true}) {
		t.Fatalf("%+v", got.Locations)
	}
	if err := m.NavigationGrant(c, filepath.Join(elsewhere, "lib/evil.rb")); errCode(err) != "notFound" {
		t.Fatal("dropped place granted:", err)
	}
}

// The roots belong to the discovered identity: a copy cannot change them.
func TestCloneServerCopiesExternalRoots(t *testing.T) {
	cfg := ServerConfig{ExternalRoots: []string{"/ruby/lib/ruby"}, ExitFailures: map[int]string{3: "x"}}
	clone := cloneServer(cfg)
	clone.ExternalRoots[0] = "/"
	clone.ExitFailures[3] = "y"
	if cfg.ExternalRoots[0] != "/ruby/lib/ruby" || cfg.ExitFailures[3] != "x" {
		t.Fatal("cloneServer shares", cfg)
	}
	if cloneServer(ServerConfig{}).ExternalRoots != nil {
		t.Fatal("empty roots became a list")
	}
}

// Every recorded initialize reply (default exe, Bundler project, the app's
// wrapper) negotiates the navigation this adapter asks for: definition,
// references and hover, incremental sync, no didSave. Only the wrapper's
// options take away pull diagnostics.
func TestRubyLspEvidenceCapabilities(t *testing.T) {
	cases, _ := rubyLspEvidence(t)
	for _, name := range []string{"A2-exe-bundle-gemfile-missing", "B1-exe-bundler-project", "C1-wrapper-navigation-only"} {
		caps, err := negotiateCapabilities(cases[name].Initialize)
		if err != nil {
			t.Fatal(name, err)
		}
		if !caps.Definition || !caps.References || !caps.Hover || caps.Change != 2 || !caps.OpenClose || caps.Save || caps.PullAdvertised != (name != "C1-wrapper-navigation-only") {
			t.Fatalf("%s: %+v", name, caps)
		}
	}
}

// C1 replayed through the session: the recorded initialize reply, then each
// recorded definition, references and hover answer at its recorded position.
// The answer before indexing is empty, not a failure; references keep both
// project places; hover Markdown arrives as sent.
func TestRubyLspEvidenceReplay(t *testing.T) {
	cases, fixture := rubyLspEvidence(t)
	c1 := cases["C1-wrapper-navigation-only"]
	e := newEnv(t, SessionOptions{})
	root, cfg := e.project()
	prefix, gems := t.TempDir(), t.TempDir()
	cfg.ProviderID = rubyLspProviderID
	cfg.ExternalRoots = []string{filepath.Join(prefix, "lib", "ruby"), gems}
	e.approve(root, cfg)
	srv := &navServer{answers: map[string]string{}, init: string(c1.Initialize)}
	e.script = srv.script
	dir := root.Project().Root
	// C1 ran in the Bundler project, which shares lib/greeter.rb.
	app := fixture["bundlerProject"].(map[string]any)["app.rb"].(string)
	writeTree(t, dir, map[string]string{"app.rb": app, "lib/greeter.rb": fixture["lib/greeter.rb"].(string)})
	docs := SessionDocuments{Sessions: e.mgr, Resolve: func(AuthorizedRoot, string) (DocumentServer, bool) {
		return DocumentServer{Config: cfg, LanguageID: "ruby"}, true
	}}
	if _, err := e.mgr.Attach(context.Background(), root, cfg); err != nil {
		t.Fatal(err)
	}
	e.next()
	m := NewManagerForTest(t, root)
	m.SetDocumentProvider(docs)
	ctx, c := context.Background(), m.snap.Context
	if _, err := m.OpenDocument(ctx, c, "app.rb", "d1", 1, app, ""); err != nil {
		t.Fatal(err)
	}
	replay := strings.NewReplacer(
		"file:///fixture/", fileURI(dir)+"/",
		"file://<ruby prefix>/", fileURI(prefix)+"/",
		"file://<spike gem dir>/", fileURI(gems)+"/",
	)
	methods := map[string]string{NavigateDefinition: "textDocument/definition", NavigateReferences: "textDocument/references", NavigateHover: "textDocument/hover"}
	ran := map[string]int{}
	empty := 0
	for _, step := range c1.Steps {
		method, ok := methods[step.Do]
		// The draft step after "change" belongs to RL-04's re-index, not to this replay.
		if !ok || step.Position.Line > 9 {
			continue
		}
		answer := replay.Replace(string(step.Result))
		srv.set(method, `"result":`+answer)
		got, err := m.Navigate(ctx, c, "d1", 1, step.Do, step.Position)
		if err != nil {
			t.Fatal(step.Do, step.Position, err)
		}
		sent := srv.last()["position"].(map[string]any)
		if int(sent["line"].(float64)) != step.Position.Line || int(sent["character"].(float64)) != step.Position.Character {
			t.Fatalf("%s sent at %v, recorded %+v", step.Do, sent, step.Position)
		}
		ran[step.Do]++
		switch step.Do {
		case NavigateHover:
			var want struct {
				Contents HoverText `json:"contents"`
			}
			if err := json.Unmarshal([]byte(answer), &want); err != nil {
				t.Fatal(err)
			}
			if got.Hover == nil || *got.Hover != want.Contents || len(got.Locations) != 0 {
				t.Fatalf("hover %+v, want %+v", got.Hover, want.Contents)
			}
		case NavigateReferences:
			want := []NavigationLocation{
				{Path: "lib/greeter.rb", Range: Range{Position{2, 6}, Position{2, 13}}},
				{Path: "app.rb", Range: Range{Position{5, 4}, Position{5, 11}}},
			}
			if !slices.Equal(got.Locations, want) {
				t.Fatalf("references %+v", got.Locations)
			}
		default:
			var sentLocations []json.RawMessage
			if err := json.Unmarshal([]byte(answer), &sentLocations); err != nil {
				t.Fatal(err)
			}
			if len(got.Locations) != len(sentLocations) || got.Hover != nil {
				t.Fatalf("definition at %+v: %d of %d", step.Position, len(got.Locations), len(sentLocations))
			}
			if len(sentLocations) == 0 {
				empty++
			}
		}
	}
	if ran[NavigateDefinition] != 7 || ran[NavigateReferences] != 1 || ran[NavigateHover] != 2 || empty != 2 {
		t.Fatalf("replayed %v, %d empty", ran, empty)
	}
}

package engine

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

// completionProbe sets the opened document to text, whose § marks the
// caret, and asks for suggestions there, looking for one item.
type completionProbe struct {
	name    string
	text    string
	trigger string // a typed trigger character; empty asks as invoked
	label   string // the item looked for, by label prefix
	extra   bool   // the item must carry extra edits (an import)
	known   string // a known gap, by task: logged rather than failed
}

type completionQACase struct {
	name     string
	provider string
	files    map[string]string
	root     string // an existing project, used as it is instead of files
	path     string // opened, as each probe's text
	probes   []completionProbe
}

// TestLocalLiveRealCompletionQA (CC-09) asks real, already installed language
// servers for completions through the production local route: discovery
// (DiscoverLocalLive over the login shell's PATH and the directories in
// AGENT_SESSIONS_COMPLETION_QA_DIRS, such as the app's managed installs),
// session approval, an opened draft, CompleteCode and ResolveCodeCompletion.
// Each project is a disposable temp directory. It logs, per probe, what the
// server answered, how long the first useful answer took, and the warm
// latency of a change plus a request, as the editor sends them (CC-05).
// Opt-in with AGENT_SESSIONS_COMPLETION_QA=1; it never installs or downloads
// anything. AGENT_SESSIONS_COMPLETION_QA_ONLY limits the cases.
func TestLocalLiveRealCompletionQA(t *testing.T) {
	if os.Getenv("AGENT_SESSIONS_COMPLETION_QA") == "" {
		t.Skip("real language servers: set AGENT_SESSIONS_COMPLETION_QA=1")
	}
	if !localLiveSupported() {
		t.Skip("local live platform gate")
	}
	managed := filepath.SplitList(os.Getenv("AGENT_SESSIONS_COMPLETION_QA_DIRS"))
	only := os.Getenv("AGENT_SESSIONS_COMPLETION_QA_ONLY")
	goMain := func(body string) string {
		return "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Print()\n\t" + body + "\n}\n"
	}
	tsPackage := map[string]string{
		"package.json":  `{"name":"qa","private":true,"type":"module"}` + "\n",
		"tsconfig.json": `{"compilerOptions":{"strict":true,"noEmit":true,"target":"es2022","module":"nodenext"}}` + "\n",
		"helper.ts":     "/** Doubles n. */\nexport function helperValue(n: number): number { return n * 2; }\n",
	}
	cases := []completionQACase{
		{
			name: "go", provider: "gopls", path: "main.go",
			files: map[string]string{
				"go.mod":    "module example.test/qa\n\ngo 1.26\n",
				"helper.go": "package main\n\n// HelperValue doubles n.\nfunc HelperValue(n int) int { return n * 2 }\n",
				"main.go":   goMain(""),
			},
			probes: []completionProbe{
				{"member after .", goMain("fmt.§"), ".", "Println", false, ""},
				{"typed on", goMain("fmt.Pr§"), "", "Println", false, ""},
				{"project symbol", goMain("HelperV§"), "", "HelperValue", false, ""},
				{"adds an import", goMain("strings.ToUp§"), "", "ToUpper", true, ""},
			},
		},
		{
			name: "python", provider: "pyright", path: "main.py",
			files: map[string]string{
				"pyproject.toml": "",
				"helpers.py":     "def helper_value(n: int) -> int:\n    \"\"\"Doubles n.\"\"\"\n    return n * 2\n",
				"main.py":        "import os\n",
			},
			probes: []completionProbe{
				{"member after .", "import os\nos.§\n", ".", "path", false, ""},
				{"typed on", "import os\nos.pa§\n", "", "path", false, ""},
				{"project symbol, imported", "import os\nhelper_va§\n", "", "helper_value", true, "CC-11"},
				// Pyright offers imports only from modules it has loaded (typing is, via
				// builtins; collections is not), so not collections.defaultdict.
				{"standard library, imported", "import os\nDefaultDi§\n", "", "DefaultDict", true, ""},
			},
		},
		{
			name: "typescript", provider: "typescript", path: "main.ts",
			files: merge(tsPackage, map[string]string{"main.ts": "export {};\n"}),
			probes: []completionProbe{
				{"member after .", "console.§\n", ".", "log", false, ""},
				{"typed on", "console.lo§\n", "", "log", false, ""},
				{"project symbol, imported", "helperVal§\n", "", "helperValue", true, "CC-10"},
			},
		},
		{
			name: "javascript", provider: "typescript", path: "main.mjs",
			files: merge(tsPackage, map[string]string{"main.mjs": "export {};\n"}),
			probes: []completionProbe{
				{"member after .", "const items = [1, 2];\nitems.§\n", ".", "map", false, ""},
				{"typed on", "const items = [1, 2];\nitems.ma§\n", "", "map", false, ""},
			},
		},
		{
			name: "ruby", provider: "ruby-lsp", path: "main.rb",
			files: map[string]string{
				"helper.rb": "# Doubles numbers.\nclass Helper\n  # Doubles n.\n  def self.value(n) = n * 2\nend\n",
				"main.rb":   "class Main\nend\n",
			},
			probes: []completionProbe{
				{"project constant", "class Main\n  def run\n    Hel§\n  end\nend\n", "", "Helper", false, ""},
				{"method after .", "class Main\n  def run\n    Helper.§\n  end\nend\n", ".", "value", false, ""},
				{"instance variable after @", "class Main\n  def run\n    @name = 1\n    @§\n  end\nend\n", "@", "@name", false, ""},
			},
		},
		{
			name: "rust", provider: "rust-analyzer", path: "src/main.rs",
			files: map[string]string{
				"Cargo.toml":  "[package]\nname = \"qa\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
				"src/main.rs": "/// Doubles n.\nfn helper_value(n: i32) -> i32 { n * 2 }\nfn main() {}\n",
			},
			probes: []completionProbe{
				{"member after .", "/// Doubles n.\nfn helper_value(n: i32) -> i32 { n * 2 }\nfn main() {\n    let v: Vec<i32> = Vec::new();\n    v.§\n}\n", ".", "len", false, ""},
				{"project symbol", "/// Doubles n.\nfn helper_value(n: i32) -> i32 { n * 2 }\nfn main() {\n    helper_va§\n}\n", "", "helper_value", false, ""},
			},
		},
	}
	// An existing project, such as this repository's frontend, opened as a
	// draft over an existing file: a real TypeScript project with its
	// node_modules, for the item cap, and Svelte when it has the package.
	if project := os.Getenv("AGENT_SESSIONS_COMPLETION_QA_PROJECT"); project != "" {
		cases = append(cases, completionQACase{
			name: "project-ts", provider: "typescript", root: project, path: "src/lib/about.ts",
			probes: []completionProbe{
				{"member after . (package)", "import { EditorView } from \"@codemirror/view\";\nEditorView.§\n", ".", "theme", false, ""},
				{"global, invoked", "export {};\nfunction local() {\n  §\n}\n", "", "local", false, ""},
				{"typed on", "export {};\nfunction local() {\n  con§\n}\n", "", "console", false, ""},
			},
		}, completionQACase{
			name: "project-svelte", provider: "svelte", root: project, path: "src/App.svelte",
			probes: []completionProbe{
				{"script member after .", "<script lang=\"ts\">\n  let count = 0;\n  console.§\n</script>\n\n<p>{count}</p>\n", ".", "log", false, ""},
				{"markup expression", "<script lang=\"ts\">\n  let count = 0;\n</script>\n\n<p>{cou§}</p>\n", "", "count", false, ""},
			},
		})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if only != "" && !slices.Contains(strings.Split(only, ","), c.name) {
				t.Skip("not in AGENT_SESSIONS_COMPLETION_QA_ONLY")
			}
			runCompletionQA(t, c, managed)
		})
	}
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// caret removes the § from text and returns where it was, in UTF-16.
func caret(text string) (string, cd.Position) {
	i := strings.Index(text, "§")
	before := text[:i]
	line := strings.Count(before, "\n")
	column := before[strings.LastIndex(before, "\n")+1:]
	return before + text[i+len("§"):], cd.Position{Line: line, Character: len(utf16.Encode([]rune(column)))}
}

func runCompletionQA(t *testing.T, c completionQACase, managed []string) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if c.root != "" {
		// Used as it is: the opened file's text is the draft's start, and only
		// the disk check below reads it again.
		dir, err = filepath.EvalSymlinks(c.root)
		if err == nil {
			var data []byte
			data, err = os.ReadFile(filepath.Join(dir, c.path))
			c.files = map[string]string{c.path: string(data)}
		}
		if err != nil {
			t.Fatal(err)
		}
		c.root = dir
	}
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range c.files {
		if c.root != "" {
			break
		}
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data := t.TempDir()
	discover := func(root cd.AuthorizedRoot) ([]cd.Registration, error) {
		return discoverLocalLiveIn(root, managed, filepath.Join(data, "rust-prepare"), nil)
	}
	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "qa"}, CWD: dir, RepoRoot: dir}})
	e := NewEngine(WithService(NewService(catalog, nil)), WithCacheEnabled(false), WithRoots(paths.Roots{Config: t.TempDir(), Data: data}),
		WithInjectedLiveCodeDiagnostics(LiveCodeDiagnosticsInjection{Discover: discover, Session: cd.SessionOptions{Launch: cd.HostSessionLaunch}}))
	t.Cleanup(func() { _ = e.Close() })
	ctx := context.Background()
	att, err := e.AttachCodeDiagnostics(ctx, dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctxID := att.Context
	var s cd.Snapshot
	for start := time.Now(); ; time.Sleep(100 * time.Millisecond) {
		s, err = e.GetCodeDiagnosticsSnapshot(ctx, ctxID)
		if err == nil && s.Proposal != nil {
			break
		}
		if time.Since(start) > 60*time.Second {
			// No proposal: nothing here needs approval, so it skips below.
			break
		}
	}
	var state *cd.ProviderState
	for i := range s.Providers {
		if s.Providers[i].ProviderID == c.provider {
			state = &s.Providers[i]
		}
	}
	if state == nil || (state.Availability != "available" && state.Availability != "approvalRequired") {
		for _, p := range s.Providers {
			t.Logf("provider %s %s %q", p.ProviderID, p.Availability, p.Notice)
		}
		t.Skipf("%s is not available here", c.provider)
	}
	if s.Proposal == nil {
		t.Fatalf("%s is %s but no approval was proposed: %v", c.provider, state.Availability, err)
	}
	if _, err = e.ApproveCodeDiagnostics(ctx, ctxID, "session", s.Proposal.Fingerprint); err != nil {
		t.Fatal(err)
	}
	version := int64(1)
	opened := time.Now()
	waitLocal(t, 120*time.Second, "document open", func() bool {
		_, err = e.OpenCodeDiagnosticDocument(ctx, ctxID, c.path, "qa", version, c.files[c.path], "")
		return err == nil
	})
	t.Logf("%s: opened %s after %s", c.provider, c.path, time.Since(opened).Round(time.Millisecond))
	for i, p := range c.probes {
		text, pos := caret(p.text)
		trigger := cd.CompletionTrigger{Kind: "invoked"}
		if p.trigger != "" {
			trigger = cd.CompletionTrigger{Kind: "character", Character: p.trigger}
		}
		// As the editor does: the pending change, then the request for its version.
		ask := func() (cd.CompletionResult, int, time.Duration, error) {
			start := time.Now()
			version++
			if _, err := e.ChangeCodeDiagnosticDocument(ctx, ctxID, "qa", version, text); err != nil {
				return cd.CompletionResult{}, -1, 0, err
			}
			r, err := e.CompleteCode(ctx, ctxID, "qa", version, pos, trigger)
			took := time.Since(start)
			if err != nil {
				return r, -1, took, err
			}
			for j, item := range r.Items {
				if strings.HasPrefix(item.Label, p.label) && (!p.extra || len(item.ExtraEdits) > 0) {
					return r, j, took, nil
				}
			}
			return r, -1, took, nil
		}
		// The first probe waits for the server to start and index.
		limit := 20 * time.Second
		if i == 0 {
			limit = 120 * time.Second
		}
		first := time.Now()
		var r cd.CompletionResult
		found, asks, empty := -1, 0, 0
		var lastErr error
		for time.Since(first) < limit {
			asks++
			r, found, _, lastErr = ask()
			if found >= 0 {
				break
			}
			if lastErr == nil && len(r.Items) == 0 {
				empty++
			}
			time.Sleep(250 * time.Millisecond)
		}
		if found < 0 {
			// The items whose label starts as the one looked for, else the first ones.
			var labels []string
			for _, item := range r.Items {
				if strings.HasPrefix(strings.ToLower(item.Label), strings.ToLower(p.label[:min(len(p.label), 4)])) {
					labels = append(labels, item.Label+"|"+item.LabelDescription+"|"+item.Detail+"|extra="+strconv.Itoa(len(item.ExtraEdits)))
				}
			}
			if len(labels) == 0 {
				for _, item := range r.Items[:min(len(r.Items), 12)] {
					labels = append(labels, item.Label)
				}
			}
			report := t.Errorf
			if p.known != "" {
				report = t.Logf
				t.Logf("KNOWN GAP (%s):", p.known)
			}
			report("%s %s: no %q after %d asks in %s (err %v); %d items, incomplete=%v, first: %q", c.provider, p.name, p.label, asks, limit, lastErr, len(r.Items), r.Incomplete, labels)
			continue
		}
		if p.known != "" {
			t.Errorf("%s %s: found, so %s fixed it; drop its known marker", c.provider, p.name, p.known)
		}
		ready := time.Since(first)
		item := r.Items[found]
		// Details for the item, as the info panel asks while the answer is the last one.
		resolveStart := time.Now()
		details, resolveErr := e.ResolveCodeCompletion(ctx, ctxID, "qa", r.ResultID, found)
		resolveTook := time.Since(resolveStart)
		var warm []time.Duration
		for range 5 {
			_, j, took, err := ask()
			if err != nil || j < 0 {
				t.Errorf("%s %s: warm ask found %d: %v", c.provider, p.name, j, err)
				break
			}
			warm = append(warm, took)
		}
		slices.Sort(warm)
		docLen := 0
		if details.Documentation != nil {
			docLen = len(details.Documentation.Value)
		}
		var extra []string
		for _, x := range item.ExtraEdits {
			extra = append(extra, strings.TrimSpace(x.NewText))
		}
		insert := item.InsertText
		if item.Edit != nil {
			insert = item.Edit.NewText
		}
		median, worst := time.Duration(0), time.Duration(0)
		if len(warm) > 0 {
			median, worst = warm[len(warm)/2], warm[len(warm)-1]
		}
		t.Logf("%s %s: %q kind=%d insert=%q snippet=%v extra=%q detail=%q | items=%d incomplete=%v truncated=%v triggers=%q | first useful answer after %s (%d asks, %d empty), warm change+complete median %s max %s | resolve %s err=%v detail=%q doc=%d chars",
			c.provider, p.name, item.Label, item.Kind, insert, item.Snippet, extra, item.Detail,
			len(r.Items), r.Incomplete, r.Truncated, r.TriggerCharacters,
			ready.Round(time.Millisecond), asks, empty, median.Round(time.Millisecond), worst.Round(time.Millisecond),
			resolveTook.Round(time.Millisecond), resolveErr, details.Detail, docLen)
	}
	for name, body := range c.files {
		if data, _ := os.ReadFile(filepath.Join(dir, name)); string(data) != body {
			t.Errorf("%s changed on disk", name)
		}
	}
}

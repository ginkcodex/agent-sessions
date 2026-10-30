package codediagnostics

import (
	"slices"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/managedtools"
)

func TestLanguagesCoverEveryProviderAndTool(t *testing.T) {
	providers := []string{goplsProviderID, "go-vet", GoSyntaxProviderID, tsProviderID, svelteLSProviderID, "svelte-check", ruffProviderID, pyrightProviderID, rustAnalyzerProviderID, "cargo-check"}
	seen := map[string]string{}
	for _, l := range Languages() {
		for _, id := range append(slices.Clone(l.Live), l.Saved...) {
			if other, ok := seen[id]; ok {
				t.Fatalf("provider %s is in %s and %s", id, other, l.ID)
			}
			seen[id] = l.ID
		}
	}
	for _, id := range providers {
		if l, ok := LanguageOf(id); !ok || seen[id] != l.ID {
			t.Fatalf("provider %s has no language", id)
		}
	}
	if len(seen) != len(providers) {
		t.Fatalf("languages name providers that do not exist: %v", seen)
	}
	if _, ok := LanguageOf("lsp"); ok {
		t.Fatal("unknown provider has a language")
	}
	tools := ManagedToolNames()
	if want := managedtools.ProposableTools(); len(tools) != len(want) || !slices.Equal(slices.Sorted(slices.Values(tools)), slices.Sorted(slices.Values(want))) {
		t.Fatalf("managed tools %v, proposable %v", tools, want)
	}
	if got := LiveProviderIDs(); !slices.Equal(got, []string{"gopls", "typescript", "svelte", "ruff", "pyright", "rust-analyzer"}) {
		t.Fatalf("live providers %v", got)
	}
	// Callers cannot change the table.
	Languages()[0].Live[0] = "changed"
	if l, _ := LanguageOf(goplsProviderID); l.Live[0] != goplsProviderID {
		t.Fatal("table changed through a copy")
	}
}

func TestLiveBridgeCoversEveryLiveProvider(t *testing.T) {
	for _, id := range LiveProviderIDs() {
		if liveBridge(id, AuthorizedRoot{}, nil, nil) == nil {
			t.Errorf("live provider %q has no bridge", id)
		}
	}
	for _, id := range []string{"", "lsp", "go-vet", "cargo-check"} {
		if liveBridge(id, AuthorizedRoot{}, nil, nil) != nil {
			t.Errorf("provider %q should be ignored", id)
		}
	}
}

func TestDiscoverLocalLiveFollowsLanguageOrder(t *testing.T) {
	skipUnlessProduction(t)
	bin, goEnv := goplsHost(t, "v0.24.0", goplsModulePath)
	tsPath, tsEnv := tsHost(t)
	env := LocalLiveEnvironment{GoPath: []string{bin}, Path: tsPath, Go: goEnv, TS: tsEnv, Ruff: RuffEnvironment{Home: tsEnv.Home}, Pyright: PyrightEnvironment{Home: tsEnv.Home, NodeDir: tsEnv.NodeDir}}
	for name, files := range map[string]map[string]string{
		"single root": {"a.go": "package a", "b.ts": "", "c.py": "", "d.rs": ""},
		"nested":      {"web/package.json": "{}", "web/b.ts": "", "api/pyproject.toml": "", "api/c.py": ""},
	} {
		regs, err := DiscoverLocalLive(goplsRoot(t, files), env)
		if err != nil {
			t.Fatal(name, err)
		}
		var ids []string
		for _, r := range regs {
			ids = append(ids, r.ProviderID)
		}
		if !slices.Equal(ids, LiveProviderIDs()) {
			t.Errorf("%s: %v", name, ids)
		}
	}
}

func TestDisabledLanguagesLeaveDiscovery(t *testing.T) {
	skipUnlessProduction(t)
	bin, goEnv := goplsHost(t, "v0.24.0", goplsModulePath)
	tsPath, tsEnv := tsHost(t)
	env := LocalLiveEnvironment{GoPath: []string{bin}, Path: tsPath, Go: goEnv, TS: tsEnv, Ruff: RuffEnvironment{Home: tsEnv.Home}, Pyright: PyrightEnvironment{Home: tsEnv.Home, NodeDir: tsEnv.NodeDir},
		Disabled: NewDisabledLanguages([]string{"python", "rust", "nonsense"})}
	if len(env.Disabled) != 2 || !env.Disabled.Disables(ruffProviderID) || !env.Disabled.Disables("cargo-check") || env.Disabled.Disables(goplsProviderID) || env.Disabled.Disables("lsp") {
		t.Fatal(env.Disabled)
	}
	for name, files := range map[string]map[string]string{
		"single root": {"go.mod": "module example.test/a\n\ngo 1.26\n", "a.go": "package a", "b.py": ""},
		"nested":      {"web/package.json": "{}", "web/b.ts": "", "api/pyproject.toml": "", "api/c.py": ""},
	} {
		root := goplsRoot(t, files)
		regs, err := DiscoverLocalLive(root, env)
		if err != nil {
			t.Fatal(name, err)
		}
		var ids []string
		for _, r := range regs {
			ids = append(ids, r.ProviderID)
		}
		if !slices.Equal(ids, []string{goplsProviderID, tsProviderID, svelteLSProviderID}) {
			t.Errorf("%s: %v", name, ids)
		}
		// Python has sources, Rust none: only Python is reported, as disabled
		// and with nothing to approve or start.
		var disabled []string
		for _, r := range DisabledRegistrations(root, env.Disabled) {
			if r.Availability != "disabled" || r.Adapter != nil || r.Prepare != nil || (r.Live != nil && (r.Live.State.Notice != "Off in Settings" || r.Live.Server.Executable != "" || r.Live.Workspaces != nil)) || (r.Live == nil && r.Notice != "Off in Settings") {
				t.Fatalf("%s: %+v", name, r)
			}
			disabled = append(disabled, r.ProviderID)
		}
		if !slices.Equal(disabled, []string{ruffProviderID, pyrightProviderID}) {
			t.Errorf("%s: disabled %v", name, disabled)
		}
		if _, err := DiscoveryDocuments(root, append(regs, DisabledRegistrations(root, env.Disabled)...), nil); err != nil {
			t.Fatal(name, err)
		}
	}
	root := goplsRoot(t, map[string]string{"a.go": "package a"})
	if regs := DisabledRegistrations(root, env.Disabled); len(regs) != 0 {
		t.Fatal("a project without Python or Rust reported", regs)
	}
	if regs := DisabledRegistrations(root, nil); regs != nil {
		t.Fatal(regs)
	}
	all := []Registration{{ProviderID: goplsProviderID}, {ProviderID: ruffProviderID}, {ProviderID: "cargo-check"}}
	if got := env.Disabled.Without(all); len(got) != 1 || got[0].ProviderID != goplsProviderID {
		t.Fatal(got)
	}
}

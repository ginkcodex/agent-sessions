package codediagnostics

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// gitInit makes root/rel a repository of its own; nothing is committed.
func gitInit(t *testing.T, root AuthorizedRoot, rel string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	cmd := exec.Command("git", "-C", filepath.Join(root.Project().Root, filepath.FromSlash(rel)), "init", "-q")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v: %s", rel, err, out)
	}
}

// fillDir writes n small files in root/rel, enough to spend a walk's budget.
func fillDir(t *testing.T, root AuthorizedRoot, rel string, n int) {
	t.Helper()
	dir := filepath.Join(root.Project().Root, filepath.FromSlash(rel))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d.txt", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func openTestRoot(t *testing.T, root AuthorizedRoot) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(root.Project().Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// ignoreTree is an outer repository that ignores dist/ and web/; web/ is a
// repository of its own that ignores build/; build/x is a repository inside
// an ignored folder that is not one.
func ignoreTree(t *testing.T) AuthorizedRoot {
	t.Helper()
	root := goplsRoot(t, map[string]string{
		".gitignore":          "dist/\nweb/\nbuild/\n",
		"go.mod":              "module example.test/root\n\ngo 1.26\n",
		"dist/go.mod":         "module example.test/dist\n\ngo 1.26\n",
		"web/.gitignore":      "build/\n",
		"web/go.mod":          "module example.test/web\n\ngo 1.26\n",
		"web/build/go.mod":    "module example.test/webbuild\n\ngo 1.26\n",
		"web/src/go.mod":      "module example.test/websrc\n\ngo 1.26\n",
		"build/x/go.mod":      "module example.test/x\n\ngo 1.26\n",
		"tools/lint/go.mod":   "module example.test/lint\n\ngo 1.26\n",
		"tools/lint/gen/a.go": "package gen\n",
	})
	gitInit(t, root, ".")
	gitInit(t, root, "web")
	gitInit(t, root, "build/x")
	return root
}

func TestGitScopeSkipsIgnoredFoldersAndEntersNestedRepositories(t *testing.T) {
	root := ignoreTree(t)
	r := openTestRoot(t, root)
	s := newGitScope(root)
	// In walk order: a walk asks about a folder before the folders inside it.
	for _, c := range []struct {
		rel  string
		want bool
	}{
		{"build", true}, {"dist", true}, // ignored, no marker
		{"tools", false},
		{"web", false},      // ignored, but a repository of its own
		{"web/build", true}, // ignored by web's own rules
		{"web/src", false},
	} {
		if got := s.skip(r, c.rel); got != c.want {
			t.Errorf("skip(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	if s.calls != 2 {
		t.Fatalf("Git calls = %d, want 2 (root and web)", s.calls)
	}
}

func TestGitScopeWithoutGitSkipsNothing(t *testing.T) {
	root := goplsRoot(t, map[string]string{".gitignore": "dist/\n", "dist/a.go": "package a\n"})
	r := openTestRoot(t, root)
	if newGitScope(root).skip(r, "dist") {
		t.Fatal("a folder was skipped without Git")
	}
}

func TestGitScopeRootIgnoredByOuterRepository(t *testing.T) {
	outer := goplsRoot(t, map[string]string{".gitignore": "*\n!.gitignore\n", "proj/dist/a.go": "package a\n", "proj/.gitignore": "dist/\n"})
	gitInit(t, outer, ".")
	dir := filepath.Join(outer.Project().Root, "proj")
	root, err := AuthorizeRoot("local:test", dir, func(s string) bool { return s == dir })
	if err != nil {
		t.Fatal(err)
	}
	if newGitScope(root).skip(openTestRoot(t, root), "dist") {
		t.Fatal("the outer repository's rules were used for a root it ignores")
	}
}

func TestGitScopeCallBudget(t *testing.T) {
	files := map[string]string{".gitignore": "out/\n"}
	for i := range maxGitScopeCalls + 2 {
		files[fmt.Sprintf("r%02d/.gitignore", i)] = "out/\n"
	}
	root := goplsRoot(t, files)
	gitInit(t, root, ".")
	r := openTestRoot(t, root)
	s := newGitScope(root)
	for i := range maxGitScopeCalls + 2 {
		rel := fmt.Sprintf("r%02d", i)
		gitInit(t, root, rel)
		s.skip(r, rel)
	}
	if s.calls != maxGitScopeCalls {
		t.Fatalf("Git calls = %d, want the budget %d", s.calls, maxGitScopeCalls)
	}
}

func TestFilesFirstOrder(t *testing.T) {
	root := goplsRoot(t, map[string]string{"b.txt": "", "a/x": "", "c/x": "", "go.mod": "", "Z": ""})
	r := openTestRoot(t, root)
	f, err := r.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := f.ReadDir(-1)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(ds, filesFirst)
	var got []string
	for _, d := range ds {
		got = append(got, d.Name())
	}
	if want := []string{"Z", "b.txt", "go.mod", "a", "c"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestGoDiscoveryUsesGitIgnoreRules(t *testing.T) {
	root := ignoreTree(t)
	want := []string{"", "tools/lint", "web", "web/src"}
	w, err := discoverGoWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.modules, want) {
		t.Fatalf("gopls modules = %q, want %q", w.modules, want)
	}
	vet := &GoVetPlan{root: root}
	if err := vet.discover(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(vet.modules, want) {
		t.Fatalf("go vet modules = %q, want %q", vet.modules, want)
	}
}

func TestGoDiscoveryWithoutGitIsUnchanged(t *testing.T) {
	root := goplsRoot(t, map[string]string{
		".gitignore":  "dist/\n",
		"go.mod":      "module example.test/root\n\ngo 1.26\n",
		"dist/go.mod": "module example.test/dist\n\ngo 1.26\n",
	})
	w, err := discoverGoWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"", "dist"}; !slices.Equal(w.modules, want) {
		t.Fatalf("modules = %q, want %q", w.modules, want)
	}
}

// The bug this plan fixes: folders sorting before go.mod spend the budget.
func TestGoDiscoveryFindsRootModuleBehindLargeFolders(t *testing.T) {
	root := goplsRoot(t, map[string]string{"go.mod": "module example.test/root\n\ngo 1.26\n", "zz/go.mod": "module example.test/zz\n\ngo 1.26\n"})
	fillDir(t, root, "frontend/public", maxGoplsEntries+10)
	w, err := discoverGoWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.modules, []string{""}) || !w.partial {
		t.Fatalf("gopls: modules %q partial %v; want [\"\"] and partial", w.modules, w.partial)
	}
	vet := &GoVetPlan{root: root}
	if err := vet.discover(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(vet.modules, []string{""}) || !vet.partial {
		t.Fatalf("go vet: modules %q partial %v; want [\"\"] and partial", vet.modules, vet.partial)
	}
}

// With Git, the large ignored folder is not entered at all, so nothing is
// lost to the budget.
func TestGoDiscoveryIgnoredLargeFolderKeepsFullCoverage(t *testing.T) {
	root := goplsRoot(t, map[string]string{".gitignore": "frontend/dist/\n", "go.mod": "module example.test/root\n\ngo 1.26\n", "zz/go.mod": "module example.test/zz\n\ngo 1.26\n"})
	fillDir(t, root, "frontend/dist", maxGoplsEntries+10)
	gitInit(t, root, ".")
	w, err := discoverGoWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.modules, []string{"", "zz"}) {
		t.Fatalf("modules = %q, want both", w.modules)
	}
}

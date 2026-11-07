package codediagnostics

import (
	"slices"
	"testing"
)

// webTree is an outer repository ignoring dist/ and web/; web/ is its own
// repository ignoring build/. marker is written at the root, in dist, web,
// web/build and web/src.
func webTree(t *testing.T, marker, body string, extra map[string]string) AuthorizedRoot {
	t.Helper()
	files := map[string]string{
		".gitignore":          "dist/\nweb/\n",
		"web/.gitignore":      "build/\n",
		marker:                body,
		"dist/" + marker:      body,
		"web/" + marker:       body,
		"web/build/" + marker: body,
		"web/src/" + marker:   body,
	}
	for k, v := range extra {
		files[k] = v
	}
	root := goplsRoot(t, files)
	gitInit(t, root, ".")
	gitInit(t, root, "web")
	return root
}

// largeTree has marker at the root and more entries than any walk's budget
// in a folder that sorts before it.
func largeTree(t *testing.T, marker, body string) AuthorizedRoot {
	t.Helper()
	root := goplsRoot(t, map[string]string{marker: body, "zz/" + marker: body})
	fillDir(t, root, "aaa", 4106)
	return root
}

var webPackages = []string{"", "web", "web/src"}

func TestProjectDirsUseGitIgnoreRules(t *testing.T) {
	w, err := discoverProjectDirs(webTree(t, "package.json", "{}", nil), tsProjectMarkers)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.packages, webPackages) {
		t.Fatalf("packages = %q, want %q", w.packages, webPackages)
	}
	// Ruff, Pyright and RuboCop share the walk.
	w, err = discoverProjectDirs(webTree(t, "pyproject.toml", "", nil), ruffProjectMarkers)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.packages, webPackages) {
		t.Fatalf("Ruff roots = %q, want %q", w.packages, webPackages)
	}
}

func TestProjectDirsFindRootBehindLargeFolder(t *testing.T) {
	w, err := discoverProjectDirs(largeTree(t, "package.json", "{}"), tsProjectMarkers)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.packages, []string{""}) || !w.partial {
		t.Fatalf("packages %q partial %v; want [\"\"] and partial", w.packages, w.partial)
	}
}

func TestLiveWorkspacesUseGitIgnoreRules(t *testing.T) {
	root := webTree(t, "package.json", "{}", map[string]string{
		// Git ignores .venv; the name rule still reads its pyvenv.cfg.
		".gitignore":           "dist/\nweb/\n.venv/\n",
		"api/.venv/pyvenv.cfg": "home = /usr/bin\n",
		"api/main.py":          "print()\n",
	})
	scan, err := scanLiveWorkspaces(root)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	python := false
	for _, d := range scan.dirs {
		dirs = append(dirs, d.rel)
		python = python || d.rel == "api" && d.python
	}
	slices.Sort(dirs)
	if want := []string{"", "api", "web", "web/src"}; !slices.Equal(dirs, want) {
		t.Fatalf("scanned %q, want %q", dirs, want)
	}
	if !python {
		t.Fatal("api's ignored .venv was not recognized")
	}
}

func TestSvelteCheckWalkUsesGitIgnoreRules(t *testing.T) {
	root := webTree(t, "App.svelte", "<p></p>\n", nil)
	var seen []string
	partial := false
	if err := walkContained(root, func(rel string) error {
		seen = append(seen, rel)
		return nil
	}, &partial); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(seen, "App.svelte") || !slices.Contains(seen, "web/App.svelte") || !slices.Contains(seen, "web/src/App.svelte") {
		t.Fatalf("visited %q; root, web and web/src files missing", seen)
	}
	if slices.Contains(seen, "dist/App.svelte") || slices.Contains(seen, "web/build/App.svelte") {
		t.Fatalf("visited %q; ignored folders entered", seen)
	}
}

func TestSveltePackagesUseGitIgnoreRules(t *testing.T) {
	w, err := discoverSveltePackages(webTree(t, "svelte.config.js", "export default {}\n", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.packages, webPackages) {
		t.Fatalf("packages = %q, want %q", w.packages, webPackages)
	}
	w, err = discoverSveltePackages(largeTree(t, "svelte.config.js", "export default {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(w.packages, []string{""}) || !w.partial {
		t.Fatalf("large: packages %q partial %v; want [\"\"] and partial", w.packages, w.partial)
	}
}

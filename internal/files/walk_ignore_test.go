package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// A plain root holding repositories: each is listed by its own ignore rules
// (docs/plan/nested-repo-gitignore.md).
func TestWalkFollowsEachRepositoryIgnoreRules(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	write(t, root, "notes.md", "plain\n")
	write(t, root, "node_modules/dep/index.js", "x\n") // outside any repository: skipped by name
	write(t, root, "logs/run.log", "x\n")              // outside any repository: no Git rules

	app := nestedRepo(t, root, "app")
	write(t, root, "app/.gitignore", "devops/staging/\n*.log\n.ruff_cache/\nweb/\nconfig.yaml\n")
	write(t, root, "app/devops/staging/init.sql", "select 1;\n")
	write(t, root, "app/devops/README.md", "kept\n")
	write(t, root, "app/debug.log", "x\n")
	write(t, root, "app/.ruff_cache/x", "x\n")
	write(t, root, "app/vendor/lib/lib.go", "package lib\n")
	write(t, root, "app/.venv/pyvenv.cfg", "home = /usr\n") // not ignored: Git decides, not names
	// Tracked, although an ignore rule matches it.
	write(t, root, "app/config.yaml", "k: v\n")
	app("add", "-f", "config.yaml", "vendor/lib/lib.go")
	app("commit", "-q", "-m", "tracked")
	// A repository the outer one ignores is still source, by its own rules.
	nestedRepo(t, root, "app/web")
	write(t, root, "app/web/.gitignore", "dist/\n")
	write(t, root, "app/web/dist/bundle.js", "x\n")
	write(t, root, "app/web/app.log", "not ignored here\n")
	// A repository inside an ignored folder that is not one itself is not found.
	nestedRepo(t, root, "app/devops/staging/clone")

	got, err := walk(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"notes.md", "logs/run.log", "app/main.go", "app/.gitignore", "app/devops/README.md",
		"app/vendor/lib/lib.go", "app/.venv/pyvenv.cfg", "app/config.yaml",
		"app/web/main.go", "app/web/.gitignore", "app/web/app.log"} {
		if !slices.Contains(got.Files, f) {
			t.Errorf("Files lacks %s: %q", f, got.Files)
		}
	}
	for _, f := range []string{"app/debug.log"} {
		if !slices.Contains(got.Ignored, f) || slices.Contains(got.Files, f) {
			t.Errorf("%s: want in Ignored only; files %q, ignored %q", f, got.Files, got.Ignored)
		}
	}
	for _, d := range []string{"node_modules", "app/devops/staging", "app/.ruff_cache", "app/web/dist"} {
		if !slices.Contains(got.IgnoredDirectories, d) {
			t.Errorf("IgnoredDirectories lacks %s: %q", d, got.IgnoredDirectories)
		}
	}
	for _, f := range append(got.Files, got.Ignored...) {
		for _, d := range got.IgnoredDirectories {
			if len(f) > len(d) && f[:len(d)+1] == d+"/" {
				t.Errorf("%s listed inside the ignored folder %s", f, d)
			}
		}
	}
	if want := []string{"app", "app/web"}; !slices.Equal(got.Repositories, want) {
		t.Errorf("Repositories = %q, want %q", got.Repositories, want)
	}
	if slices.Contains(got.IgnoredDirectories, "app/web") {
		t.Error("an ignored repository folder was left out")
	}
	if !slices.Contains(got.Directories, "app/web") || slices.Contains(got.Directories, "app/devops/staging") {
		t.Errorf("Directories = %q", got.Directories)
	}
}

// When a repository's Git cannot tell, that repository keeps the name rules
// and the others still use Git; the listing never fails.
func TestWalkFallsBackToNameRulesPerRepository(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	nestedRepo(t, root, "good")
	write(t, root, "good/.gitignore", "*.log\n")
	write(t, root, "good/a.log", "x\n")
	write(t, root, "bad/.git/HEAD", "not a repository\n")
	write(t, root, "bad/b.log", "x\n")
	write(t, root, "bad/node_modules/m.js", "x\n")
	write(t, root, "bad/src/s.go", "package s\n")

	got, err := walk(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Ignored, "good/a.log") {
		t.Errorf("good/a.log not ignored: %q", got.Ignored)
	}
	for _, f := range []string{"bad/b.log", "bad/src/s.go"} {
		if !slices.Contains(got.Files, f) {
			t.Errorf("Files lacks %s: %q", f, got.Files)
		}
	}
	if !slices.Contains(got.IgnoredDirectories, "bad/node_modules") {
		t.Errorf("bad/node_modules not skipped by name: %q", got.IgnoredDirectories)
	}
	if !slices.Equal(got.Repositories, []string{"bad", "good"}) {
		t.Errorf("Repositories = %q", got.Repositories)
	}
}

func TestWalkCountsIgnoredFilesTowardTheCap(t *testing.T) {
	requireGit(t)
	saved := fileCap
	t.Cleanup(func() { fileCap = saved })
	root := t.TempDir()
	nestedRepo(t, root, "r") // r, r/main.go
	write(t, root, "r/.gitignore", "*.log\n")
	for _, n := range []string{"a", "b", "c", "d"} {
		write(t, root, "r/"+n+".log", "x\n")
	}
	fileCap = 5
	got, err := walk(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Files) + len(got.Directories) + len(got.Ignored); !got.Truncated || n != fileCap {
		t.Errorf("truncated = %v, entries = %d (files %q, dirs %q, ignored %q)", got.Truncated, n, got.Files, got.Directories, got.Ignored)
	}
}

// A symlink named .git does not make a repository.
func TestWalkIgnoresSymlinkedGitMarker(t *testing.T) {
	requireGit(t)
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	root := t.TempDir()
	nestedRepo(t, root, "real")
	write(t, root, "fake/x.go", "package x\n")
	if err := os.Symlink(filepath.Join(root, "real", ".git"), filepath.Join(root, "fake", ".git")); err != nil {
		t.Fatal(err)
	}
	got, err := walk(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Repositories, []string{"real"}) {
		t.Errorf("Repositories = %q", got.Repositories)
	}
}

func TestSearchPlainRootSkipsIgnoredFiles(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	nestedRepo(t, root, "app")
	write(t, root, "app/.gitignore", "gen/\n*.log\n")
	write(t, root, "app/src.txt", "needle\n")
	write(t, root, "app/gen/out.txt", "needle\n")
	write(t, root, "app/run.log", "needle\n")
	write(t, root, "top.txt", "needle\n")

	got, err := Search(context.Background(), root, SearchQuery{Pattern: "needle"})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range got.Files {
		paths = append(paths, f.Path)
	}
	slices.Sort(paths)
	if want := []string{"app/src.txt", "top.txt"}; !slices.Equal(paths, want) {
		t.Errorf("search found %q, want %q", paths, want)
	}
}

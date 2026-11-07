package files

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestRepositoryIgnoredFollowsGitRules(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	git := nestedRepo(t, root, "repo")
	write(t, root, "repo/.gitignore", "dist/\n*.log\nkeep/\n")
	write(t, root, "repo/dist/a.js", "a\n")
	write(t, root, "repo/dist/sub/b.js", "b\n")
	write(t, root, "repo/debug.log", "x\n")
	write(t, root, "repo/logs/1.log", "x\n")
	// keep/ is ignored but holds a tracked file: never reported whole.
	write(t, root, "repo/keep/tracked.txt", "t\n")
	git("add", "-f", "keep/tracked.txt")
	git("commit", "-q", "-m", "keep")
	write(t, root, "repo/keep/new.txt", "n\n")
	write(t, root, "repo/pkg/.gitignore", "gen/\n")
	write(t, root, "repo/pkg/gen/x.go", "package gen\n")
	write(t, root, "repo/.git/info/exclude", "secret.env\n")
	write(t, root, "repo/secret.env", "k=v\n")
	write(t, root, "repo/src/ok.go", "package src\n")

	files, dirs, ok := repositoryIgnored(context.Background(), filepath.Join(root, "repo"))
	if !ok {
		t.Fatal("ok = false")
	}
	slices.Sort(files)
	slices.Sort(dirs)
	// Git reports logs/ (only ignored files inside) and its file both.
	if want := []string{"debug.log", "keep/new.txt", "logs/1.log", "secret.env"}; !reflect.DeepEqual(files, want) {
		t.Errorf("files = %q, want %q", files, want)
	}
	if want := []string{"dist", "logs", "pkg/gen"}; !reflect.DeepEqual(dirs, want) {
		t.Errorf("dirs = %q, want %q", dirs, want)
	}
}

func TestRepositoryIgnoredNeverAnswersForAnOuterRepository(t *testing.T) {
	requireGit(t)
	outer := t.TempDir()
	git := nestedRepo(t, outer, ".")
	write(t, outer, ".gitignore", "*.tmp\n")
	// The outer repository tracks a file in bad, so without the ceiling its
	// Git would answer for bad (x.tmp) rather than fail.
	write(t, outer, "bad/keep.txt", "k\n")
	git("add", "bad/keep.txt")
	write(t, outer, "bad/.git/HEAD", "not a repository\n")
	write(t, outer, "bad/x.tmp", "x\n")
	if files, dirs, ok := repositoryIgnored(context.Background(), filepath.Join(outer, "bad")); ok {
		t.Errorf("invalid .git answered with the outer repository: files %q, dirs %q", files, dirs)
	}
}

func TestRepositoryIgnoredUnknownWithoutGit(t *testing.T) {
	root := t.TempDir()
	write(t, root, "repo/.git/HEAD", "ref: refs/heads/main\n")
	t.Setenv("PATH", "")
	if _, _, ok := repositoryIgnored(context.Background(), filepath.Join(root, "repo")); ok {
		t.Error("ok = true without git")
	}
}

func TestParseIgnored(t *testing.T) {
	files, dirs, ok := parseIgnored([]byte("a.log\x00dist/\x00dist/sub/\x00"))
	if !ok || !reflect.DeepEqual(files, []string{"a.log"}) || !reflect.DeepEqual(dirs, []string{"dist", "dist/sub"}) {
		t.Errorf("got %q %q %v", files, dirs, ok)
	}
	if _, _, ok := parseIgnored([]byte("./\x00")); ok {
		t.Error(`"./" answer: ok = true`)
	}
	if files, dirs, ok := parseIgnored(nil); !ok || files != nil || dirs != nil {
		t.Errorf("empty answer: %q %q %v", files, dirs, ok)
	}
}

func TestWalkScopesInnermostRepository(t *testing.T) {
	saved := walkScopeQuery
	t.Cleanup(func() { walkScopeQuery = saved })
	walkScopeQuery = func(_ context.Context, dir string) ([]string, []string, bool) {
		switch filepath.Base(dir) {
		case "outer":
			return []string{"x.log"}, []string{"dist"}, true
		case "inner":
			return []string{"y.log"}, nil, true
		}
		return nil, nil, false
	}
	w := newWalkScopes(context.Background(), "/root")
	if w.of("top.txt") != nil || w.of("a/b.txt") != nil {
		t.Error("a path outside every repository has ignores")
	}
	w.enter("outer")
	w.enter("outer/inner")
	w.enter("outer/broken")
	for _, c := range []struct {
		path       string
		file, dir  string
		ignoredDir bool
	}{
		{path: "outer/x.log", file: "outer/x.log"},
		{path: "outer/src/a.go", dir: "outer/dist", ignoredDir: true},
		{path: "outer/inner/y.log", file: "outer/inner/y.log"},
		{path: "outer/inner/deep/z.go", file: "outer/inner/y.log"},
	} {
		ig := w.of(c.path)
		if ig == nil {
			t.Fatalf("%s: no ignores", c.path)
		}
		if c.file != "" && !ig.files[c.file] {
			t.Errorf("%s: %s not ignored (%v)", c.path, c.file, ig.files)
		}
		if c.ignoredDir && !ig.dirs[c.dir] {
			t.Errorf("%s: %s not ignored (%v)", c.path, c.dir, ig.dirs)
		}
	}
	if ig := w.of("outer/inner/x.log"); ig.files["outer/x.log"] || ig.files["outer/inner/x.log"] {
		t.Error("the inner repository took the outer repository's rules")
	}
	if w.of("outer/broken/a.log") != nil {
		t.Error("a repository Git could not answer for has ignores")
	}
	if w.of("outer/dist") == nil || !w.of("outer/dist").dirs["outer/dist"] {
		t.Error("outer/dist is not asked of its parent repository")
	}
}

func TestWalkScopesBudget(t *testing.T) {
	saved := walkScopeQuery
	t.Cleanup(func() { walkScopeQuery = saved })
	calls := 0
	walkScopeQuery = func(context.Context, string) ([]string, []string, bool) {
		calls++
		return nil, nil, true
	}
	w := newWalkScopes(context.Background(), t.TempDir())
	for i := range maxWalkScopes + 5 {
		w.enter(filepath.ToSlash(filepath.Join("r", string(rune('a'+i%26)), string(rune('a'+i/26)))))
	}
	w.enter("r/a/a") // asked already
	if calls != maxWalkScopes {
		t.Errorf("calls = %d, want %d", calls, maxWalkScopes)
	}
	if w.of("r/z/c/file") != nil {
		t.Error("a repository past the call budget has ignores")
	}

	calls = 0
	w = newWalkScopes(context.Background(), "/root")
	w.deadline = time.Now().Add(-time.Second)
	w.enter("late")
	if calls != 0 || w.of("late/file") != nil {
		t.Errorf("past the time budget: calls = %d", calls)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w = newWalkScopes(ctx, "/root")
	w.enter("cancelled")
	if calls != 0 {
		t.Errorf("cancelled walk: calls = %d", calls)
	}
}

func TestWalkScopesQueryDeadline(t *testing.T) {
	saved := walkScopeQuery
	t.Cleanup(func() { walkScopeQuery = saved })
	var deadline time.Time
	walkScopeQuery = func(ctx context.Context, _ string) ([]string, []string, bool) {
		deadline, _ = ctx.Deadline()
		return nil, nil, true
	}
	w := newWalkScopes(context.Background(), "/root")
	w.enter("repo")
	if !deadline.Equal(w.deadline) {
		t.Errorf("query deadline = %v, want the walk's budget %v", deadline, w.deadline)
	}
}

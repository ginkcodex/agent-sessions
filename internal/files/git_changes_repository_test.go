package files

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGitChangesRepositoryPaths(t *testing.T) {
	own, err := ResolveGitChangesRepository(context.Background(), "/project", "")
	if err != nil || own.Root != "/project" {
		t.Fatalf("own: %+v %v", own, err)
	}
	if got, _ := own.Paths([]string{"a", "b/c"}); !reflect.DeepEqual(got, []string{"a", "b/c"}) {
		t.Fatalf("own paths: %v", got)
	}
	r := GitChangesRepository{Root: "/project/child", prefix: "child/"}
	got, err := r.Paths([]string{"child/a", "child/b/c"})
	if err != nil || !reflect.DeepEqual(got, []string{"a", "b/c"}) {
		t.Fatalf("paths: %v %v", got, err)
	}
	for _, p := range []string{"child", "child/", "childish/a", "other/a", "../child/a"} {
		if _, err := r.Paths([]string{"child/a", p}); !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("%q: %v", p, err)
		}
	}
	if got := r.Drafts([]string{"child/a", "other/a", "child"}); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("drafts: %v", got)
	}
	status := r.Status(RepoStatus{OK: true, Entries: []GitChange{{Path: "new", OriginalPath: "old"}, {Path: "added"}}})
	if want := []GitChange{{Path: "child/new", OriginalPath: "child/old"}, {Path: "child/added"}}; !reflect.DeepEqual(status.Entries, want) {
		t.Fatalf("status: %+v", status.Entries)
	}
	if r.Path("") != "" || r.Path("x") != "child/x" {
		t.Fatal("path")
	}
}

// A repository the project's own ignores is reviewed as itself, not the outer one.
func TestGitChangesRepositoryInsideGitProject(t *testing.T) {
	requireGit(t)
	parent := t.TempDir()
	nestedRepo(t, parent, "outer")
	root, _ := filepath.EvalSymlinks(filepath.Join(parent, "outer"))
	write(t, root, ".gitignore", "/web/\n")
	nestedRepo(t, root, "web")
	write(t, root, "web/main.go", "package main\n\nfunc main() {}\n")
	r, err := ResolveGitChangesRepository(context.Background(), root, "web")
	if err != nil || r.Root != filepath.Join(root, "web") {
		t.Fatalf("resolve: %+v %v", r, err)
	}
	status, err := GitRepoStatus(context.Background(), r.Root)
	status = r.Status(status)
	if err != nil || !status.OK || len(status.Entries) != 1 || status.Entries[0].Path != "web/main.go" {
		t.Fatalf("status: %+v %v", status, err)
	}
}

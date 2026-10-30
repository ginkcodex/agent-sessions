package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestGitHistoryRepositoriesRootAndScope(t *testing.T) {
	requireGit(t)
	parent := t.TempDir()
	nestedRepo(t, parent, "repo")
	root, _ := filepath.EvalSymlinks(filepath.Join(parent, "repo"))
	write(t, root, "src/deep/file", "x")
	// A nested repository is deliberately not another choice for Git workspaces.
	nestedRepo(t, root, "nested")
	for _, scope := range []string{"", "src/deep"} {
		dir := filepath.Join(root, filepath.FromSlash(scope))
		got, err := GitHistoryRepositories(context.Background(), dir)
		want := GitHistoryRepositoryList{Repositories: []GitHistoryRepository{{Path: ".", Name: "repo", Scope: scope}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%q: %+v, %v", scope, got, err)
		}
		target, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: dir, Repo: "."})
		if err != nil || target.Workspace != dir || target.Root != root || target.Scope != scope || target.Target.Dir != dir {
			t.Fatalf("resolved: %+v, %v", target, err)
		}
	}
}

func TestGitHistoryRepositoriesChildrenAndGitFiles(t *testing.T) {
	requireGit(t)
	parent := t.TempDir()
	git := nestedRepo(t, parent, "z/deep")
	nestedRepo(t, parent, "a")
	git("worktree", "add", "-q", "-b", "linked", filepath.Join(parent, "linked"))
	// A local submodule exercises a second real .git-file layout, without network.
	git("-c", "protocol.file.allow=always", "submodule", "add", "-q", filepath.Join(parent, "a"), "module")
	got, err := GitHistoryRepositories(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, repo := range got.Repositories {
		paths = append(paths, repo.Path)
	}
	if want := []string{"a", "linked", "z/deep", "z/deep/module"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths %v", paths)
	}
	target, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: parent, Repo: "z/deep/module"})
	canonical, _ := filepath.EvalSymlinks(parent)
	if err != nil || target.Workspace != canonical || target.Root != filepath.Join(canonical, "z", "deep", "module") || target.Scope != "" || target.Target.Dir != parent {
		t.Fatalf("target %+v, %v", target, err)
	}
	if _, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: parent, Repo: "z/deep/module/nope"}); err == nil {
		t.Fatal("accepted missing target")
	}
	write(t, parent, "a/sub/file", "x")
	if _, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: parent, Repo: "a/sub"}); err == nil {
		t.Fatal("accepted non-root child")
	}
}

func TestGitHistoryRepositoriesEmptyAndFailures(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	got, err := GitHistoryRepositories(context.Background(), root)
	if err != nil || got.Truncated || len(got.Repositories) != 0 || got.Repositories == nil {
		t.Fatalf("empty %+v, %v", got, err)
	}
	_, err = resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: root, Repo: "."})
	if !errors.Is(err, errGitHistoryNotRepository) {
		t.Fatalf("not repository: %v", err)
	}
	if _, err := GitHistoryRepositories(context.Background(), filepath.Join(root, "missing")); err == nil || errors.Is(err, errGitHistoryNotRepository) {
		t.Fatalf("missing dir: %v", err)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	if _, err := GitHistoryRepositories(expired, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitHistoryRepositories(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	// A marker causes Git launch errors to be preserved, not treated as no repos.
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := GitHistoryRepositories(context.Background(), root); err == nil || errors.Is(err, errGitHistoryNotRepository) {
		t.Fatalf("missing executable: %v", err)
	}
}

func TestGitHistoryInvalidMarkersDoNotFallThrough(t *testing.T) {
	requireGit(t)
	for _, marker := range []string{"directory", "file"} {
		t.Run(marker, func(t *testing.T) {
			parent := t.TempDir()
			nestedRepo(t, parent, "outer")
			dir := filepath.Join(parent, "outer", "broken")
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if marker == "directory" {
				if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
					t.Fatal(err)
				}
			} else {
				write(t, dir, ".git", "gitdir: missing\n")
			}
			write(t, dir, "sub/file", "x")
			for _, selected := range []string{dir, filepath.Join(dir, "sub")} {
				if _, err := GitHistoryRepositories(context.Background(), selected); err == nil || errors.Is(err, errGitHistoryNotRepository) {
					t.Fatalf("invalid marker accepted: %v", err)
				}
			}
			if _, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: filepath.Join(parent, "outer"), Repo: "broken"}); err == nil {
				t.Fatal("accepted broken child")
			}
		})
	}
	root := t.TempDir()
	write(t, root, "broken/.git", "invalid")
	nestedRepo(t, root, "healthy")
	got, err := GitHistoryRepositories(context.Background(), root)
	if err != nil || !got.Truncated || len(got.Repositories) != 1 || got.Repositories[0].Path != "healthy" {
		t.Fatalf("invalid sibling hid healthy repo or incomplete notice: %+v, %v", got, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := GitHistoryRepositories(context.Background(), root); err == nil {
		t.Fatal("discovery swallowed missing Git executable")
	}
}

func TestGitHistoryTargetRejectsEscapes(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	nestedRepo(t, root, "repo")
	outside := t.TempDir()
	nestedRepo(t, outside, "external")
	if err := os.Symlink(filepath.Join(outside, "external"), filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(root, filepath.Join(root, "self")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"", "..", "repo/../repo", "/tmp", "C:/repo", `repo\sub`, "repo/.git", "repo/.GIT/objects", "escape", "self", "repo\x00bad"} {
		if _, err := resolveGitHistoryTarget(context.Background(), GitHistoryTarget{Dir: root, Repo: rel}); err == nil {
			t.Errorf("accepted %q", rel)
		}
	}
	// Discoveries never authorize traversal, and links are not walked.
	got, err := GitHistoryRepositories(context.Background(), root)
	if err != nil || len(got.Repositories) != 1 || got.Repositories[0].Path != "repo" {
		t.Fatalf("discovery %+v, %v", got, err)
	}
}

func TestGitHistoryRepositoriesTruncationAndSkips(t *testing.T) {
	requireGit(t)
	source := t.TempDir()
	nestedRepo(t, source, "source")
	gitdir := filepath.Join(source, "source", ".git")
	root := t.TempDir()
	// Git-file repositories share object storage for a small, fast cap fixture.
	for i := 0; i <= MaxGitHistoryRepositories; i++ {
		write(t, root, fmt.Sprintf("repo-%03d/.git", i), "gitdir: "+gitdir+"\n")
	}
	write(t, root, "node_modules/hidden/.git", "invalid")
	got, err := GitHistoryRepositories(context.Background(), root)
	if err != nil || !got.Truncated || len(got.Repositories) != MaxGitHistoryRepositories {
		t.Fatalf("cap: %d, %v, %v", len(got.Repositories), got.Truncated, err)
	}
	if got.Repositories[0].Path != "repo-000" || got.Repositories[len(got.Repositories)-1].Path != "repo-499" {
		t.Fatal("not sorted")
	}
}

func TestGitHistoryRepositoriesWalkTruncated(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	for i := 0; i <= MaxFiles; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("dir-%05d", i)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := GitHistoryRepositories(context.Background(), root)
	if err != nil || !got.Truncated || len(got.Repositories) != 0 {
		t.Fatalf("walk cap: %+v, %v", got, err)
	}
}

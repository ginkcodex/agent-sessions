package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// A project that is in no repository works on one of those inside it, with
// paths that stay project-relative on both sides.
func TestGitChangesRepositoryInsideProject(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"child", "other"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		git := func(args ...string) {
			t.Helper()
			out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %s: %v", args, out, err)
			}
		}
		git("init")
		git("config", "core.autocrlf", "false")
		git("config", "commit.gpgsign", "false")
		git("config", "user.name", "Test")
		git("config", "user.email", "test@example.com")
		if err := os.WriteFile(filepath.Join(dir, "file"), []byte("base\n"), 0600); err != nil {
			t.Fatal(err)
		}
		git("add", "file")
		git("commit", "-m", "base")
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := editEngine(root)
	ctx := context.Background()
	status := func() files.RepoStatus {
		t.Helper()
		got, err := e.GitStatus(ctx, root, "child")
		if err != nil || !got.OK {
			t.Fatalf("status: %+v %v", got, err)
		}
		return got
	}
	if got, err := e.GitStatus(ctx, root, ""); err != nil || got.OK {
		t.Fatalf("project itself: %+v %v", got, err)
	}
	write("child/file", "changed\n")
	write("other/file", "changed\n")
	got := status()
	if len(got.Entries) != 1 || got.Entries[0].Path != "child/file" || got.Entries[0].Worktree != "M" {
		t.Fatalf("entries: %+v", got.Entries)
	}
	diff, err := e.ReadGitDiff(ctx, root, "child", "child/file", files.GitDiffIndex)
	if err != nil || diff.Content.Path != "child/file" || len(diff.Content.Changes) == 0 {
		t.Fatalf("diff: %+v %v", diff, err)
	}
	if err := e.GitStage(ctx, root, "child", []string{"child/file"}); err != nil {
		t.Fatal(err)
	}
	if got := status(); got.Entries[0].Index != "M" || got.Entries[0].Worktree != "" {
		t.Fatalf("staged: %+v", got.Entries)
	}
	if err := e.GitUnstage(ctx, root, "child", nil); err != nil {
		t.Fatal(err)
	}
	if err := e.GitDiscard(ctx, root, "child", files.GitDiscardRequest{Paths: []string{"child/file"}, DraftPaths: []string{"child/file", "other/file"}}); err == nil || !strings.Contains(err.Error(), "unsaved draft") {
		t.Fatalf("draft inside the repository: %v", err)
	}
	if err := e.GitDiscard(ctx, root, "child", files.GitDiscardRequest{Paths: []string{"child/file"}, DraftPaths: []string{"other/file"}}); err != nil {
		t.Fatal(err)
	}
	if got := status(); len(got.Entries) != 0 {
		t.Fatalf("discarded: %+v", got.Entries)
	}
	// Paths of the project outside the repository are refused, and left alone.
	for _, p := range []string{"other/file", "child", "childish/file"} {
		if err := e.GitStage(ctx, root, "child", []string{p}); !errors.Is(err, files.ErrOutsideRoot) {
			t.Errorf("stage %q: %v", p, err)
		}
		if _, err := e.ReadGitDiff(ctx, root, "child", p, files.GitDiffIndex); !errors.Is(err, files.ErrOutsideRoot) {
			t.Errorf("diff %q: %v", p, err)
		}
	}
	if text, _ := os.ReadFile(filepath.Join(root, "other", "file")); string(text) != "changed\n" {
		t.Fatalf("other repository touched: %q", text)
	}
	write("child/new", "new\n")
	if err := e.GitCommit(ctx, root, "child", files.GitCommitRequest{Message: "add new", StageAll: true}); err != nil {
		t.Fatal(err)
	}
	if got := status(); len(got.Entries) != 0 || !got.HasCommits {
		t.Fatalf("committed: %+v", got)
	}
	if err := e.GitSwitch(ctx, root, "child", files.GitSwitchRequest{Name: "feature", Create: true}); err != nil {
		t.Fatal(err)
	}
	branches, err := e.GitBranches(ctx, root, "child")
	current := ""
	for _, b := range branches.Branches {
		if b.Current {
			current = b.Name
		}
	}
	if err != nil || current != "feature" {
		t.Fatalf("branches: %+v %v", branches, err)
	}
	for _, repo := range []string{"..", "../x", "missing", "child/../other", root} {
		if _, err := e.GitStatus(ctx, root, repo); err == nil {
			t.Errorf("repository %q accepted", repo)
		}
	}
	if _, err := e.GitStatus(ctx, filepath.Join(root, "child"), "other"); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("unknown project: %v", err)
	}
}

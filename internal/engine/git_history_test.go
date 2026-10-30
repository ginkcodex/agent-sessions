package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
)

func historyCalls(e *Engine, dir, repo string) map[string]func() error {
	ctx := context.Background()
	target := files.GitHistoryTarget{Dir: dir, Repo: repo}
	commit := files.GitHistoryCommitRequest{GitHistoryTarget: target}
	return map[string]func() error{
		"repositories": func() error { _, err := e.GitHistoryRepositories(ctx, dir); return err },
		"branches":     func() error { _, err := e.GitHistoryBranches(ctx, target); return err },
		"list": func() error {
			_, err := e.GitHistoryList(ctx, files.GitHistoryListRequest{GitHistoryTarget: target, Ref: "HEAD"})
			return err
		},
		"commit": func() error { _, err := e.GitHistoryCommit(ctx, commit); return err },
		"diff": func() error {
			_, err := e.ReadGitHistoryDiff(ctx, files.GitHistoryDiffRequest{GitHistoryCommitRequest: commit, Path: "file"})
			return err
		},
	}
}

func TestGitHistoryExactAuthorization(t *testing.T) {
	root := t.TempDir()
	e := editEngine(root)
	for _, dir := range []string{"", t.TempDir(), filepath.Join(root, "child"), filepath.Dir(root)} {
		for name, call := range historyCalls(e, dir, ".") {
			t.Run(name+dir, func(t *testing.T) {
				if err := call(); !errors.Is(err, ErrUnknownDirectory) {
					t.Fatalf("got %v", err)
				}
			})
		}
	}
}

func TestGitHistoryAuthorizedChild(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", child}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init")
	git("config", "core.autocrlf", "false")
	git("config", "commit.gpgsign", "false")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(child, "file"), []byte("content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "file")
	git("commit", "-m", "first")
	e := editEngine(root)
	ctx := context.Background()
	target := files.GitHistoryTarget{Dir: root, Repo: "child"}
	repos, err := e.GitHistoryRepositories(ctx, root)
	if err != nil || len(repos.Repositories) != 1 {
		t.Fatalf("%+v %v", repos, err)
	}
	refs, err := e.GitHistoryBranches(ctx, target)
	if err != nil || refs.Head == "" {
		t.Fatalf("%+v %v", refs, err)
	}
	page, err := e.GitHistoryList(ctx, files.GitHistoryListRequest{GitHistoryTarget: target, Ref: "HEAD"})
	if err != nil || len(page.Commits) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	req := files.GitHistoryCommitRequest{GitHistoryTarget: target, Tip: page.Tip, Commit: page.Commits[0].OID}
	detail, err := e.GitHistoryCommit(ctx, req)
	if err != nil || len(detail.Changes) != 1 {
		t.Fatalf("%+v %v", detail, err)
	}
	diff, err := e.ReadGitHistoryDiff(ctx, files.GitHistoryDiffRequest{GitHistoryCommitRequest: req, Path: detail.Changes[0].Path})
	if err != nil || diff.Content.Text != "content\n" {
		t.Fatalf("%+v %v", diff, err)
	}
	for name, call := range historyCalls(e, root, "../outside") {
		if name != "repositories" {
			if err := call(); err == nil {
				t.Fatalf("%s accepted outside repo", name)
			}
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.GitHistoryRepositories(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

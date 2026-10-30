package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadGitHistoryDiff(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "dir/file.txt", "old\n")
	git("add", ".")
	git("commit", "-m", "root")
	first := git("rev-parse", "HEAD")
	target := GitHistoryTarget{Dir: root, Repo: "."}
	request := func(commit, path string) GitHistoryDiffRequest {
		return GitHistoryDiffRequest{GitHistoryCommitRequest: GitHistoryCommitRequest{GitHistoryTarget: target, Tip: git("rev-parse", "HEAD"), Commit: commit}, Path: path}
	}
	got, err := ReadGitHistoryDiff(context.Background(), request(first, "dir/file.txt"))
	if err != nil || got.Content.Text != "old\n" || len(got.Content.Changes) != 1 || got.Content.Changes[0].Kind != "added" {
		t.Fatalf("root: %+v %v", got, err)
	}
	statusWrite(t, root, "dir/file.txt", "new\n")
	git("add", ".")
	git("commit", "-m", "modify")
	second := git("rev-parse", "HEAD")
	statusWrite(t, root, "dir/file.txt", "dirty\n")
	got, err = ReadGitHistoryDiff(context.Background(), request(second, "dir/file.txt"))
	if err != nil || got.Content.Text != "new\n" || got.Content.Revision != "" || len(got.Content.Changes) != 1 || got.Content.Changes[0].Removed[0] != "old" {
		t.Fatalf("modified: %+v %v", got, err)
	}
	git("rm", "-f", "dir/file.txt")
	git("commit", "-m", "delete")
	if err := os.RemoveAll(filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	got, err = ReadGitHistoryDiff(context.Background(), request(git("rev-parse", "HEAD"), "dir/file.txt"))
	if err != nil || !got.Deleted || len(got.Content.Changes) != 1 || got.Content.Changes[0].Removed[0] != "new" {
		t.Fatalf("deleted: %+v %v", got, err)
	}
	if _, err = ReadGitHistoryDiff(context.Background(), request(second, "../file")); err == nil {
		t.Fatal("forged change accepted")
	}
}

func TestHistoryDiffLargeOldSideAndExternalTools(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "large", strings.Repeat("a", MaxReadBytes+1))
	statusWrite(t, root, "file", "old\n")
	git("add", ".")
	git("commit", "-m", "large base")
	statusWrite(t, root, "large", "small\n")
	statusWrite(t, root, "file", "new\n")
	git("add", ".")
	git("commit", "-m", "small replacement")
	// If either tool runs the diff fails, without requiring platform shell scripts.
	git("config", "diff.external", "git-history-external-must-not-run")
	git("config", "diff.history.textconv", "git-history-textconv-must-not-run")
	git("config", "diff.history.command", "git-history-driver-must-not-run")
	statusWrite(t, root, ".gitattributes", "file diff=history\n")
	tip := git("rev-parse", "HEAD")
	req := GitHistoryDiffRequest{GitHistoryCommitRequest: GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: tip, Commit: tip}, Path: "large"}
	got, err := ReadGitHistoryDiff(context.Background(), req)
	if err != nil || got.Kind != "tooLarge" || got.Content.Text != "" {
		t.Fatalf("old size limit: %+v %v", got, err)
	}
	req.Path = "file"
	got, err = ReadGitHistoryDiff(context.Background(), req)
	if err != nil || got.Content.Text != "new\n" || len(got.Content.Changes) != 1 {
		t.Fatalf("external driver: %+v %v", got, err)
	}
}

func TestHistoryBinaryAndRenameDiff(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "old.txt", "same\n")
	statusWrite(t, root, "binary.bin", "a\x00b")
	git("add", ".")
	git("commit", "-m", "base")
	git("mv", "old.txt", "new.txt")
	git("commit", "-m", "rename")
	commit := git("rev-parse", "HEAD")
	req := GitHistoryDiffRequest{GitHistoryCommitRequest: GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: commit, Commit: commit}, Path: "new.txt"}
	got, err := ReadGitHistoryDiff(context.Background(), req)
	if err != nil || got.Content.Text != "same\n" || got.Change.OldPath != "old.txt" || len(got.Content.Changes) != 0 {
		t.Fatalf("rename: %+v %v", got, err)
	}
	req.Commit = git("rev-parse", "HEAD^")
	req.Path = "binary.bin"
	got, err = ReadGitHistoryDiff(context.Background(), req)
	if err != nil || got.Kind != "binary" || got.Content.Text != "" {
		t.Fatalf("binary: %+v %v", got, err)
	}
}

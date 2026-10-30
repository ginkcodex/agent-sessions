package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGitHistoryCompareRefusesSymlinkParentEscape(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "dir/file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	outside := t.TempDir()
	statusWrite(t, outside, "file", "secret\n")
	if err := os.RemoveAll(filepath.Join(root, "dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "dir")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	// A listing may safely omit the old descendants as deletions, or fail
	// closed; it must never return/read the target outside the workspace.
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		return
	}
	for _, change := range list.Changes {
		if change.Path != "dir/file" {
			continue
		}
		got, err := ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req, Token: list.Token, Path: change.Path})
		if err == nil && got.Content.Text == "secret\n" {
			t.Fatal("followed symlink outside workspace")
		}
	}
}

func TestGitHistoryCompareIgnoredRecreatedDeletion(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "tracked", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	git("rm", "tracked")
	statusWrite(t, root, ".gitignore", "tracked\n")
	statusWrite(t, root, "tracked", "saved\n")
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	got := compareRead(t, req, list, "tracked")
	if got.Deleted || got.Content.Text != "saved\n" {
		t.Fatalf("ignored recreated %+v", got)
	}
}

func TestGitHistoryCompareNewUntrackedInvalidatesToken(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	statusWrite(t, root, "file", "saved\n")
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	statusWrite(t, root, "new", "new\n")
	_, err = ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req, Token: list.Token, Path: "file"})
	if !errors.Is(err, ErrGitHistoryCompareStale) {
		t.Fatalf("new path did not invalidate token: %v", err)
	}
}

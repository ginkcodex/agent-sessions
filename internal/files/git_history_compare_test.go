package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func compareRequest(t *testing.T, root string, git func(...string) string) GitHistoryCompareRequest {
	t.Helper()
	return GitHistoryCompareRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Ref: git("symbolic-ref", "HEAD"), ExpectedOID: git("rev-parse", "HEAD")}
}

func compareRead(t *testing.T, req GitHistoryCompareRequest, list GitHistoryComparison, path string) GitHistoryCompareDiff {
	t.Helper()
	got, err := ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req, Token: list.Token, Path: path})
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return got
}

func TestGitHistoryCompareSavedDirectionAndStaleness(t *testing.T) {
	root, git := blameRepo(t)
	for _, name := range []string{"file", "delete", "recreated", "rename"} {
		text := "old\n"
		if name == "delete" {
			text = "deleted original\n"
		}
		if name == "rename" {
			text = "rename original\n"
		}
		statusWrite(t, root, name, text)
	}
	statusWrite(t, root, ".gitignore", "ignored\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	statusWrite(t, root, "file", "staged\n")
	git("add", "file")
	statusWrite(t, root, "file", "saved\n")
	git("rm", "delete", "recreated")
	statusWrite(t, root, "recreated", "back\n")
	git("mv", "rename", "renamed")
	statusWrite(t, root, "new", "untracked\n")
	statusWrite(t, root, "ignored", "hidden\n")
	index := git("ls-files", "--stage")
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Changes) != 5 || list.Commit != req.ExpectedOID || len(list.Token) != 64 {
		t.Fatalf("list %+v", list)
	}
	got := compareRead(t, req, list, "file")
	if got.Content.Text != "saved\n" || got.Content.Revision != "" || got.Content.Edit.Reason != EditReadOnly || len(got.Content.Changes) != 1 || got.Content.Changes[0].Removed[0] != "old" {
		t.Fatalf("direction %+v", got)
	}
	got = compareRead(t, req, list, "new")
	if got.Content.Changes[0].Kind != "added" || got.Content.Changes[0].Count != 1 {
		t.Fatalf("untracked %+v", got)
	}
	got = compareRead(t, req, list, "delete")
	if !got.Deleted || got.Content.Text != "" || got.Content.Changes[0].Removed[0] != "deleted original" {
		t.Fatalf("delete %+v", got)
	}
	got = compareRead(t, req, list, "recreated")
	if got.Deleted || got.Content.Text != "back\n" || got.Content.Changes[0].Removed[0] != "old" {
		t.Fatalf("recreated %+v", got)
	}
	got = compareRead(t, req, list, "renamed")
	if got.Change.OldPath != "rename" || len(got.Content.Changes) != 0 {
		t.Fatalf("rename %+v", got)
	}
	if git("ls-files", "--stage") != index {
		t.Fatal("comparison modified index")
	}
	statusWrite(t, root, "file", "later\n")
	_, err = ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req, Token: list.Token, Path: "file"})
	if !errors.Is(err, ErrGitHistoryCompareStale) {
		t.Fatalf("stale: %v", err)
	}
}

func TestGitHistoryCompareTagIdentityValidationAndTools(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	git("tag", "-a", "selected", "-m", "annotated")
	req := compareRequest(t, root, git)
	req.Ref = "refs/tags/selected"
	req.ExpectedOID = git("rev-parse", req.Ref)
	git("config", "diff.external", "history-compare-must-not-run")
	git("config", "diff.history.textconv", "history-textconv-must-not-run")
	git("config", "diff.history.command", "history-driver-must-not-run")
	statusWrite(t, root, ".gitattributes", "file diff=history\n")
	statusWrite(t, root, "file", "new\n")
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if list.Commit == list.OID || list.Commit != git("rev-parse", "HEAD") {
		t.Fatalf("tag %+v", list)
	}
	if got := compareRead(t, req, list, "file"); len(got.Content.Changes) != 1 {
		t.Fatalf("tools %+v", got)
	}
	for _, ref := range []string{"", "HEAD", "selected", "--all", "refs/tags/selected^{commit}", "refs/heads/../bad"} {
		bad := req
		bad.Ref = ref
		if _, err := GitHistoryCompare(context.Background(), bad); err == nil {
			t.Errorf("accepted ref %q", ref)
		}
	}
	bad := req
	bad.ExpectedOID = list.Commit
	if _, err := GitHistoryCompare(context.Background(), bad); !errors.Is(err, ErrGitHistoryCompareStale) {
		t.Fatalf("direct tag oid: %v", err)
	}
	for _, path := range []string{"../file", ".git/config", "not-listed", "file\x00"} {
		if _, err := ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: req, Token: list.Token, Path: path}); err == nil {
			t.Errorf("accepted path %q", path)
		}
	}
	git("tag", "-f", "selected", "HEAD")
	if _, err := GitHistoryCompare(context.Background(), req); !errors.Is(err, ErrGitHistoryCompareStale) {
		t.Fatalf("moved tag: %v", err)
	}
}

func TestGitHistoryCompareScopedProtectedAndSpecialFiles(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "inside/file", "base\n")
	statusWrite(t, root, "outside", "outside\n")
	statusWrite(t, root, "inside/binary", "a\x00b")
	statusWrite(t, root, "inside/large", strings.Repeat("x", MaxReadBytes+1))
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	req.Dir = filepath.Join(root, "inside")
	statusWrite(t, root, "inside/file", "new\n")
	statusWrite(t, root, "outside", "changed\n")
	statusWrite(t, root, "inside/.claude/private", "secret")
	statusWrite(t, root, "inside/binary", "tiny\n")
	statusWrite(t, root, "inside/large", "small\n")
	if err := os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "inside", "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Changes) != 4 || !list.Truncated {
		t.Fatalf("scope %+v", list)
	}
	for _, test := range []struct{ path, kind string }{{"file", "text"}, {"binary", "binary"}, {"large", "tooLarge"}, {"link", "symlink"}} {
		got := compareRead(t, req, list, test.path)
		if got.Kind != test.kind || (test.kind != "text" && got.Content.Text != "") {
			t.Fatalf("%s %+v", test.path, got)
		}
	}
	// Tokens cannot be transplanted to a different authorized display scope.
	full := req
	full.Dir = root
	_, err = ReadGitHistoryCompareDiff(context.Background(), GitHistoryCompareDiffRequest{GitHistoryCompareRequest: full, Token: list.Token, Path: "inside/file"})
	if !errors.Is(err, ErrGitHistoryCompareStale) {
		t.Fatalf("scope token: %v", err)
	}
}

func TestGitHistoryCompareRenameBoundaryAndSubmodule(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "inside/from", "move out\n")
	statusWrite(t, root, "outside", "move in\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	req.Dir = filepath.Join(root, "inside")
	git("mv", "inside/from", "moved-out")
	git("mv", "outside", "inside/moved-in")
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Changes) != 2 {
		t.Fatalf("scope rename %+v", list)
	}
	for _, change := range list.Changes {
		if change.OldPath != "" {
			t.Fatalf("outside path leak %+v", change)
		}
		got := compareRead(t, req, list, change.Path)
		if change.Path == "from" && (!got.Deleted || change.Status != "D") {
			t.Fatalf("out %+v", got)
		}
		if change.Path == "moved-in" && (got.Content.Text != "move in\n" || change.Status != "A") {
			t.Fatalf("in %+v", got)
		}
	}
	// A gitlink is metadata, never traversed or read as a directory's content.
	oid := git("rev-parse", "HEAD")
	git("update-index", "--add", "--cacheinfo", "160000,"+oid+",inside/sub")
	git("commit", "-m", "gitlink")
	req = compareRequest(t, root, git)
	if err := os.MkdirAll(filepath.Join(root, "inside/sub"), 0755); err != nil {
		t.Fatal(err)
	}
	git("update-index", "--force-remove", "inside/sub")
	list, err = GitHistoryCompare(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	got := compareRead(t, req, list, "inside/sub")
	if got.Kind != "submodule" {
		t.Fatalf("gitlink %+v", got)
	}
}

func TestGitHistoryCompareCancellationAndTruncation(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "base", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	req := compareRequest(t, root, git)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitHistoryCompare(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel %v", err)
	}
	for i := 0; i < MaxGitChanges+1; i++ {
		statusWrite(t, root, filepath.Join("new", fmt.Sprintf("%06d", i)), "x")
	}
	list, err := GitHistoryCompare(context.Background(), req)
	if err != nil || !list.Truncated || len(list.Changes) != MaxGitChanges {
		t.Fatalf("truncated %d %+v %v", len(list.Changes), list.Truncated, err)
	}
}

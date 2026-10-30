package files

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHistoryCommitDetails(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "old.txt", "one\ntwo\n")
	git("add", ".")
	git("commit", "-m", "root\n\nfull body")
	first := git("rev-parse", "HEAD")
	req := GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: first, Commit: first}
	detail, err := GitHistoryCommit(context.Background(), req)
	if err != nil || detail.Base != "" || len(detail.Changes) != 1 || detail.Changes[0].Status != "A" || !strings.Contains(detail.Message, "full body") {
		t.Fatalf("root: %+v %v", detail, err)
	}
	git("mv", "old.txt", "new.txt")
	git("commit", "-m", "rename")
	req.Tip = git("rev-parse", "HEAD")
	req.Commit = req.Tip
	detail, err = GitHistoryCommit(context.Background(), req)
	if err != nil || detail.Base != first || len(detail.Changes) != 1 || detail.Changes[0].OldPath != "old.txt" || detail.Changes[0].Path != "new.txt" {
		t.Fatalf("rename: %+v %v", detail, err)
	}
	git("rm", "new.txt")
	git("commit", "-m", "delete")
	req.Tip = git("rev-parse", "HEAD")
	req.Commit = req.Tip
	detail, err = GitHistoryCommit(context.Background(), req)
	if err != nil || len(detail.Changes) != 1 || detail.Changes[0].Status != "D" {
		t.Fatalf("delete: %+v %v", detail, err)
	}
	req.Tip = first
	if _, err = GitHistoryCommit(context.Background(), req); err == nil {
		t.Fatal("nonmember accepted")
	}
	req.Commit = "HEAD~1"
	if _, err = GitHistoryCommit(context.Background(), req); err == nil {
		t.Fatal("revision expression accepted")
	}
}

func TestHistoryCommitMergeAndScopedRename(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "src/old", "same\n")
	git("add", ".")
	git("commit", "-m", "base")
	branch := git("branch", "--show-current")
	git("checkout", "-b", "side")
	name := awkwardName("side")
	statusWrite(t, root, "src/"+name, "side\n")
	git("add", ".")
	git("commit", "-m", "side")
	git("checkout", branch)
	statusWrite(t, root, "outside", "main\n")
	git("add", ".")
	git("commit", "-m", "main")
	base := git("rev-parse", "HEAD")
	git("merge", "--no-ff", "side", "-m", "merge")
	tip := git("rev-parse", "HEAD")
	req := GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: filepath.Join(root, "src"), Repo: "."}, Tip: tip, Commit: tip}
	detail, err := GitHistoryCommit(context.Background(), req)
	if err != nil || detail.Base != base || len(detail.Parents) != 2 || len(detail.Changes) != 1 || detail.Changes[0].Path != name {
		t.Fatalf("merge: %+v %v", detail, err)
	}
	git("mv", "src/old", "moved")
	git("commit", "-m", "move outside scope")
	req.Tip = git("rev-parse", "HEAD")
	req.Commit = req.Tip
	detail, err = GitHistoryCommit(context.Background(), req)
	if err != nil || len(detail.Changes) != 1 || detail.Changes[0].Status != "D" || detail.Changes[0].Path != "old" || detail.Changes[0].OldPath != "" {
		t.Fatalf("scoped rename: %+v %v", detail, err)
	}
}

func TestHistoryChangesLimit(t *testing.T) {
	oid := strings.Repeat("b", 40)
	var raw strings.Builder
	for i := range MaxGitChanges + 1 {
		fmt.Fprintf(&raw, ":000000 100644 %s %s A\x00file-%d\x00", strings.Repeat("0", 40), oid, i)
	}
	changes, truncated, err := parseHistoryChanges([]byte(raw.String()), "", 40)
	if err != nil || !truncated || len(changes) != MaxGitChanges {
		t.Fatalf("limit: %d %v %v", len(changes), truncated, err)
	}
}

func TestParseHistoryChangesScope(t *testing.T) {
	oid := strings.Repeat("a", 40)
	raw := func(old, new string) []byte {
		return []byte(":100644 100644 " + oid + " " + oid + " R100\x00" + old + "\x00" + new + "\x00")
	}
	for _, test := range []struct{ old, new, status, path, origin string }{{"src/old", "src/new", "R", "new", "old"}, {"outside", "src/new", "A", "new", ""}, {"src/old", "outside", "D", "old", ""}} {
		changes, truncated, err := parseHistoryChanges(raw(test.old, test.new), "src", 40)
		if err != nil || truncated || len(changes) != 1 || changes[0].Status != test.status || changes[0].Path != test.path || changes[0].OldPath != test.origin {
			t.Fatalf("%+v: %+v %v", test, changes, err)
		}
	}
	if changes, _, err := parseHistoryChanges(raw("outside/a", "outside/b"), "src", 40); err != nil || len(changes) != 0 {
		t.Fatalf("outside: %+v %v", changes, err)
	}
	if _, _, err := parseHistoryChanges(raw("../bad", "new"), "", 40); err == nil {
		t.Fatal("unsafe tree path accepted")
	}
}

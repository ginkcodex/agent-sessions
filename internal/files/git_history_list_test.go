package files

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHistoryListPagesPinnedAndSearch(t *testing.T) {
	root, git := blameRepo(t)
	target := GitHistoryTarget{Dir: root, Repo: "."}
	req := GitHistoryListRequest{GitHistoryTarget: target, Limit: 2}
	empty, err := GitHistoryList(context.Background(), req)
	if err != nil || empty.Tip != "" || len(empty.Commits) != 0 {
		t.Fatalf("unborn: %+v %v", empty, err)
	}
	messages := []string{"initial", "literal [a-z].*\n\nBody NeEdLe", "third", "fourth", "fifth"}
	for i, message := range messages {
		statusWrite(t, root, "file", strings.Repeat("x", i+1))
		git("add", ".")
		git("commit", "-m", message)
	}
	original := git("rev-list", "--topo-order", "HEAD")
	before := git("status", "--porcelain=v1")
	page, err := GitHistoryList(context.Background(), req)
	if err != nil || len(page.Commits) != 2 || page.Cursor == "" {
		t.Fatalf("first: %+v %v", page, err)
	}
	tip := page.Tip
	git("commit", "--allow-empty", "-m", "moved")
	got := []string{page.Commits[0].OID, page.Commits[1].OID}
	for page.Cursor != "" {
		req.Cursor = page.Cursor
		page, err = GitHistoryList(context.Background(), req)
		if err != nil || page.Tip != tip {
			t.Fatalf("continuation: %+v %v", page, err)
		}
		for _, c := range page.Commits {
			got = append(got, c.OID)
		}
	}
	if strings.Join(got, "\n") != original {
		t.Fatalf("pagination: %v != %s", got, original)
	}
	for _, query := range []string{"[a-z].*", "needle", git("config", "user.email"), got[0][:8]} {
		page, err = GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: target, Query: query})
		if err != nil || len(page.Commits) == 0 {
			t.Fatalf("query %q: %+v %v", query, page, err)
		}
	}
	page, err = GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: target, Query: "[nonexistent].*"})
	if err != nil || len(page.Commits) != 0 {
		t.Fatalf("literal: %+v %v", page, err)
	}
	if git("status", "--porcelain=v1") != before {
		t.Fatal("modified worktree")
	}
	git("update-ref", "refs/remotes/origin/test", "HEAD")
	for _, ref := range []string{"HEAD", "refs/heads/" + git("branch", "--show-current"), "refs/remotes/origin/test"} {
		if _, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: target, Ref: ref}); err != nil {
			t.Fatalf("ref %s: %v", ref, err)
		}
	}
}

func TestGitHistoryListValidation(t *testing.T) {
	root, git := blameRepo(t)
	git("commit", "--allow-empty", "-m", "one")
	git("commit", "--allow-empty", "-m", "two")
	req := GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Limit: 1}
	page, err := GitHistoryList(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"-x", "--all", "main", "HEAD~1", "refs/heads/main^{commit}", "refs/heads/a..b", "refs/heads/a@{b", ":(glob)*", "refs/tags/v1", "refs/heads/missing"} {
		bad := req
		bad.Ref = ref
		if _, err := GitHistoryList(context.Background(), bad); err == nil {
			t.Fatalf("accepted ref %q", ref)
		}
	}
	for _, limit := range []int{-1, MaxGitHistoryPageSize + 1} {
		bad := req
		bad.Limit = limit
		if _, err := GitHistoryList(context.Background(), bad); err == nil {
			t.Fatal("accepted limit")
		}
	}
	for _, cursor := range []string{"!", strings.Repeat("a", 4097), base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"extra":1}`))} {
		bad := req
		bad.Cursor = cursor
		if _, err := GitHistoryList(context.Background(), bad); err == nil {
			t.Fatalf("accepted cursor %q", cursor)
		}
	}
	data, _ := base64.RawURLEncoding.DecodeString(page.Cursor)
	var cursor gitHistoryCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*gitHistoryCursor){
		func(c *gitHistoryCursor) { c.Offset = -1 }, func(c *gitHistoryCursor) { c.Offset = MaxGitHistoryScan }, func(c *gitHistoryCursor) { c.Offset = 999 },
		func(c *gitHistoryCursor) { c.Query = "other" }, func(c *gitHistoryCursor) { c.Scope = "other" }, func(c *gitHistoryCursor) { c.Tip = strings.Repeat("a", 40) }, func(c *gitHistoryCursor) { c.Limit = 2 },
	} {
		badCursor := cursor
		mutate(&badCursor)
		data, _ := json.Marshal(badCursor)
		bad := req
		bad.Cursor = base64.RawURLEncoding.EncodeToString(data)
		if _, err := GitHistoryList(context.Background(), bad); err == nil {
			t.Fatalf("accepted cursor %+v", badCursor)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GitHistoryList(ctx, req); err == nil {
		t.Fatal("accepted canceled request")
	}
}

func TestGitHistoryListScopedMergeAndRoot(t *testing.T) {
	root, git := blameRepo(t)
	scope := filepath.Join(root, "sub")
	if err := os.Mkdir(scope, 0755); err != nil {
		t.Fatal(err)
	}
	statusWrite(t, root, "sub/file", "base")
	git("add", ".")
	git("commit", "-m", "root")
	rootOID := git("rev-parse", "HEAD")
	branch := git("branch", "--show-current")
	git("checkout", "-b", "side")
	statusWrite(t, root, "sub/file", "side")
	git("add", ".")
	git("commit", "-m", "side change")
	side := git("rev-parse", "HEAD")
	git("checkout", branch)
	statusWrite(t, root, "outside", "outside")
	git("add", ".")
	git("commit", "-m", "outside only")
	git("merge", "--no-ff", "side", "-m", "merge relevant")
	merge := git("rev-parse", "HEAD")
	page, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: scope, Repo: "."}})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, c := range page.Commits {
		got = append(got, c.OID)
	}
	if strings.Join(got, ",") != strings.Join([]string{merge, side, rootOID}, ",") {
		t.Fatalf("scoped merge/root: %v", got)
	}
	// A merge identical to its first parent must not appear, even if it differs
	// from a second parent; the outside commit must remain excluded too.
	git("checkout", "-b", "other", rootOID)
	statusWrite(t, root, "another", "x")
	git("add", ".")
	git("commit", "-m", "other outside")
	git("checkout", branch)
	git("merge", "--no-ff", "other", "-m", "irrelevant merge")
	again, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: scope, Repo: "."}, Limit: 1})
	if err != nil || len(again.Commits) != 1 || again.Commits[0].OID != merge {
		t.Fatalf("first-parent semantics: %+v %v", again, err)
	}
}

func TestHistoryMetadataAndMembership(t *testing.T) {
	root, git := blameRepo(t)
	git("commit", "--allow-empty", "-m", "subject\n\nfull body\x1e delimiter")
	tip := git("rev-parse", "HEAD")
	err := withGitHistory(context.Background(), GitHistoryTarget{Dir: root, Repo: "."}, func(w gitWriter, _ gitHistoryResolvedTarget) error {
		commits, err := readHistoryMetadata(w, []string{tip}, 40)
		if err != nil {
			return err
		}
		if len(commits) != 1 || commits[0].Subject != "subject" || !strings.Contains(commits[0].Message, "full body\x1e delimiter") || len(commits[0].Parents) != 0 {
			t.Fatalf("metadata %+v", commits)
		}
		return validateHistoryMembership(w, tip, tip, 40)
	})
	if err != nil {
		t.Fatal(err)
	}
	git("checkout", "--orphan", "unrelated")
	git("commit", "--allow-empty", "-m", "other")
	other := git("rev-parse", "HEAD")
	err = withGitHistory(context.Background(), GitHistoryTarget{Dir: root, Repo: "."}, func(w gitWriter, _ gitHistoryResolvedTarget) error {
		return validateHistoryMembership(w, tip, other, 40)
	})
	if err == nil {
		t.Fatal("accepted unrelated commit")
	}
}

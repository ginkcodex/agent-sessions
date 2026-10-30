package files

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Isolate both fixture Git commands and the production read path from host config.
func historyRepo(t *testing.T) (string, func(...string) string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	return blameRepo(t)
}

type historyFileState struct {
	Data     string
	Mode     os.FileMode
	Modified time.Time
}

func historySnapshot(t *testing.T, root string) map[string]historyFileState {
	t.Helper()
	result := map[string]historyFileState{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = historyFileState{string(data), info.Mode(), info.ModTime()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestHistoryReadOnlyDiscoveryThroughDiff(t *testing.T) {
	root, git := historyRepo(t)
	statusWrite(t, root, "file", "old\n")
	git("add", ".")
	git("commit", "-m", "base")
	statusWrite(t, root, "file", "new\n")
	git("add", ".")
	git("commit", "-m", "tip")
	// Include a dirty index, dirty worktree and untracked content in the snapshot.
	statusWrite(t, root, "file", "staged\n")
	git("add", "file")
	statusWrite(t, root, "file", "dirty\n")
	statusWrite(t, root, "untracked", "untouched\n")
	before := historySnapshot(t, root)
	ctx := context.Background()
	repos, err := GitHistoryRepositories(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	target := GitHistoryTarget{Dir: root, Repo: repos.Repositories[0].Path}
	refs, err := GitHistoryBranches(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	page, err := GitHistoryList(ctx, GitHistoryListRequest{GitHistoryTarget: target, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Tip != refs.Head || page.Cursor == "" {
		t.Fatalf("page: %+v", page)
	}
	if _, err := GitHistoryList(ctx, GitHistoryListRequest{GitHistoryTarget: target, Limit: 1, Cursor: page.Cursor}); err != nil {
		t.Fatal(err)
	}
	req := GitHistoryCommitRequest{GitHistoryTarget: target, Tip: page.Tip, Commit: page.Commits[0].OID}
	detail, err := GitHistoryCommit(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range detail.Changes {
		diff, err := ReadGitHistoryDiff(ctx, GitHistoryDiffRequest{GitHistoryCommitRequest: req, Path: change.Path})
		if err != nil || diff.Content.Text != "new\n" || diff.Content.Revision != "" || diff.Content.Edit == nil || diff.Content.Edit.Reason != EditReadOnly {
			t.Fatalf("diff: %+v %v", diff, err)
		}
	}
	// This includes .git/HEAD, refs, index, object files and all working files,
	// checking content, mode, mtime and new/deleted entries (not just git status).
	if after := historySnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("history changed repository or worktree content/mtime")
	}
}

func TestHistoryUnusualPathsRoundTrip(t *testing.T) {
	root, git := historyRepo(t)
	names := []string{"-leading-dash", "[literal]", "space ü", "nested/file"}
	if runtime.GOOS == "windows" {
		t.Log("Windows forbids quote/control/colon names and cannot represent non-UTF-8 names; testing legal names")
	} else {
		names = append(names, "line\nbreak", "quote\"name", ":(glob)*", "invalid-�")
		if runtime.GOOS == "linux" {
			names = append(names, "invalid-\xff")
		} else {
			t.Log("This filesystem rejects non-UTF-8 names; raw Git path bytes are covered by parser tests")
		}
	}
	for _, name := range names {
		statusWrite(t, root, name, "old\n")
	}
	git("add", ".")
	git("commit", "-m", "base")
	for _, name := range names {
		statusWrite(t, root, name, "new\n")
	}
	git("add", ".")
	git("commit", "-m", "new")
	tip := git("rev-parse", "HEAD")
	req := GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: tip, Commit: tip}
	detail, err := GitHistoryCommit(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the JSON boundary: invalid byte paths must not alias U+FFFD.
	data, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &detail); err != nil {
		t.Fatal(err)
	}
	expected := len(names)
	if runtime.GOOS == "linux" {
		expected--
		if !detail.Truncated {
			t.Fatal("unrepresentable path omitted without truncation")
		}
	}
	if len(detail.Changes) != expected {
		t.Fatalf("changes: %+v", detail)
	}
	for _, change := range detail.Changes {
		diff, err := ReadGitHistoryDiff(context.Background(), GitHistoryDiffRequest{GitHistoryCommitRequest: req, Path: change.Path})
		if err != nil || diff.Content.Text != "new\n" || len(diff.Content.Changes) != 1 || diff.Content.Changes[0].Removed[0] != "old" {
			t.Fatalf("path %q: %+v %v", change.Path, diff, err)
		}
	}
}

func TestHistoryNonUTF8ChangesDoNotAlias(t *testing.T) {
	oid := strings.Repeat("a", 40)
	raw := ":100644 100644 " + oid + " " + oid + " M\x00invalid-\xff\x00"
	raw += ":100644 100644 " + oid + " " + oid + " M\x00invalid-�\x00"
	changes, truncated, err := parseHistoryChanges([]byte(raw), "", 40)
	if err != nil || !truncated || len(changes) != 1 || changes[0].Path != "invalid-�" {
		t.Fatalf("changes: %+v %v %v", changes, truncated, err)
	}
}

func TestHistoryDiffIgnoresPresentationConfig(t *testing.T) {
	root, git := historyRepo(t)
	statusWrite(t, root, "file", "old\n")
	git("add", ".")
	git("commit", "-m", "base")
	statusWrite(t, root, "file", "new\n")
	git("add", ".")
	git("commit", "-m", "tip")
	git("config", "diff.outputIndicatorOld", "!")
	git("config", "diff.outputIndicatorNew", "?")
	tip := git("rev-parse", "HEAD")
	diff, err := ReadGitHistoryDiff(context.Background(), GitHistoryDiffRequest{GitHistoryCommitRequest: GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: tip, Commit: tip}, Path: "file"})
	if err != nil || len(diff.Content.Changes) != 1 || !reflect.DeepEqual(diff.Content.Changes[0].Removed, []string{"old"}) {
		t.Fatalf("diff: %+v %v", diff, err)
	}
}

func TestHistoryMissingPromisorBlobNeverFetches(t *testing.T) {
	root, git := historyRepo(t)
	statusWrite(t, root, "file", "blob\n")
	git("add", ".")
	git("commit", "-m", "tip")
	tip := git("rev-parse", "HEAD")
	blob := git("rev-parse", "HEAD:file")
	git("config", "remote.origin.url", "ext::git-history-network-must-not-run")
	git("config", "remote.origin.promisor", "true")
	git("config", "extensions.partialClone", "origin")
	// Even if the user explicitly permits an external transport, history denies it.
	t.Setenv("GIT_ALLOW_PROTOCOL", "ext")
	if err := os.Remove(filepath.Join(root, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	before := historySnapshot(t, root)
	req := GitHistoryDiffRequest{GitHistoryCommitRequest: GitHistoryCommitRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Tip: tip, Commit: tip}, Path: "file"}
	if _, err := ReadGitHistoryDiff(context.Background(), req); err == nil {
		t.Fatal("missing promisor blob succeeded")
	}
	if !reflect.DeepEqual(before, historySnapshot(t, root)) {
		t.Fatal("missing-object read wrote repository state")
	}
	if err := withGitHistory(context.Background(), req.GitHistoryTarget, func(w gitWriter, _ gitHistoryResolvedTarget) error {
		_, err := w.run("", "fetch", "origin")
		if err == nil || !strings.Contains(err.Error(), "transport 'ext' not allowed") {
			t.Fatalf("transport not denied: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryShallowMissingParent(t *testing.T) {
	root, git := historyRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	parent := git("rev-parse", "HEAD")
	statusWrite(t, root, "file", "tip\n")
	git("add", ".")
	git("commit", "-m", "tip")
	tip := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, ".git", "shallow"), []byte(tip+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".git", "objects", parent[:2], parent[2:])); err != nil {
		t.Fatal(err)
	}
	target := GitHistoryTarget{Dir: root, Repo: "."}
	page, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: target})
	if err != nil || !page.Shallow || len(page.Commits) != 1 || len(page.Commits[0].Parents) != 1 {
		t.Fatalf("page: %+v %v", page, err)
	}
	if _, err := GitHistoryCommit(context.Background(), GitHistoryCommitRequest{GitHistoryTarget: target, Tip: tip, Commit: tip}); err == nil {
		t.Fatal("missing parent fabricated root diff")
	}
}

func TestHistoryMetadataOutputLimit(t *testing.T) {
	root, git := historyRepo(t)
	message := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(message, []byte(strings.Repeat("x", 17<<20)), 0600); err != nil {
		t.Fatal(err)
	}
	git("commit", "--allow-empty", "-F", message)
	if _, err := GitHistoryList(context.Background(), GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}}); err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("oversized metadata: %v", err)
	}
}

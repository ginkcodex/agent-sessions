package files

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func getGitDiff(t *testing.T, root, path string, basis GitDiffBasis) GitDiffContent {
	t.Helper()
	out, err := ReadGitDiff(context.Background(), EditPolicy{}, root, path, basis)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Exercise the public readers with real patches: user presentation settings must
// not change the line-oriented protocol consumed by parseHunks. blameRepo keeps
// the fixtures LF-only even when the host's Git defaults enable CRLF conversion.
func TestGitDiffPresentationSettings(t *testing.T) {
	for _, setting := range []string{"default", "plain", "color", "indicators", "combined"} {
		t.Run(setting, func(t *testing.T) {
			for _, deletion := range []bool{true, false} {
				name := "replacement"
				if deletion {
					name = "partial-deletion"
				}
				t.Run(name, func(t *testing.T) {
					root, git := blameRepo(t)
					if setting == "plain" || setting == "color" {
						git("config", "diff.wordDiff", setting)
					}
					if setting == "combined" {
						git("config", "diff.wordDiff", "plain")
					}
					if setting == "indicators" || setting == "combined" {
						git("config", "diff.outputIndicatorOld", "<")
						git("config", "diff.outputIndicatorNew", ">")
						git("config", "diff.outputIndicatorContext", "=")
					}
					statusWrite(t, root, "file", "a\nb\nc\nd\ne\n")
					git("add", "file")
					git("commit", "-m", "base")
					index, working := "a\nc\nd\ne\n", "a\nc\ne\n"
					stagedWant := []LineChange{{Kind: "deleted", Start: 1, Removed: []string{"b"}}}
					indexWant := []LineChange{{Kind: "deleted", Start: 2, Removed: []string{"d"}}}
					if !deletion {
						index, working = "a\nB\nc\nd\ne\n", "a\nB\nc\nD\ne\n"
						stagedWant = []LineChange{{Kind: "modified", Start: 2, Count: 1, Removed: []string{"b"}}}
						indexWant = []LineChange{{Kind: "modified", Start: 4, Count: 1, Removed: []string{"d"}}}
					}
					statusWrite(t, root, "file", index)
					git("add", "file")
					statusWrite(t, root, "file", working)
					headWant := append(append([]LineChange{}, stagedWant...), indexWant...)
					ordinary, err := Read(context.Background(), root, "file")
					if err != nil {
						t.Fatal(err)
					}
					if ordinary.Text != working || !reflect.DeepEqual(ordinary.Changes, headWant) {
						t.Fatalf("Read = %+v, want text %q and changes %+v", ordinary, working, headWant)
					}
					for _, tc := range []struct {
						basis GitDiffBasis
						text  string
						want  []LineChange
					}{{GitDiffHead, working, headWant}, {GitDiffIndex, working, indexWant}, {GitDiffStaged, index, stagedWant}} {
						got := getGitDiff(t, root, "file", tc.basis)
						if got.Deleted || got.Content.Text != tc.text || !reflect.DeepEqual(got.Content.Changes, tc.want) {
							t.Errorf("%s = %+v, want text %q and changes %+v", tc.basis, got, tc.text, tc.want)
						}
					}
				})
			}
		})
	}
}

func TestGitDiffModifiedBases(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "head\n")
	git("add", ".")
	git("commit", "-m", "base")
	statusWrite(t, root, "file", "index\n")
	git("add", "file")
	statusWrite(t, root, "file", "working\n")
	for _, tc := range []struct {
		basis         GitDiffBasis
		text, removed string
		readonly      bool
	}{{GitDiffHead, "working\n", "head", false}, {GitDiffIndex, "working\n", "index", false}, {GitDiffStaged, "index\n", "head", true}} {
		got := getGitDiff(t, root, "file", tc.basis)
		if got.Content.Text != tc.text || got.ReadOnly != tc.readonly || got.Deleted || len(got.Content.Changes) != 1 || strings.Join(got.Content.Changes[0].Removed, "\n") != tc.removed {
			t.Fatalf("%s: %+v", tc.basis, got)
		}
		if tc.readonly && (got.Content.Revision != "" || got.Content.Edit == nil || got.Content.Edit.Eligible) {
			t.Fatal("staged snapshot editable")
		}
		if !tc.readonly && (got.Content.Revision == "" || got.Content.Edit == nil || !got.Content.Edit.Eligible) {
			t.Fatalf("working content not editable: %+v", got.Content)
		}
	}
}
func TestGitDiffAddedDeletedAndUnborn(t *testing.T) {
	for _, unborn := range []bool{false, true} {
		t.Run(map[bool]string{true: "unborn", false: "committed"}[unborn], func(t *testing.T) {
			root, git := blameRepo(t)
			if !unborn {
				statusWrite(t, root, "base", "base\n")
				git("add", ".")
				git("commit", "-m", "base")
			}
			statusWrite(t, root, "added", "added\n")
			git("add", "added")
			for _, basis := range []GitDiffBasis{GitDiffHead, GitDiffIndex, GitDiffStaged} {
				got := getGitDiff(t, root, "added", basis)
				if got.Content.Text != "added\n" || got.Deleted {
					t.Fatalf("addition %s %+v", basis, got)
				}
				if basis == GitDiffIndex {
					if len(got.Content.Changes) != 0 {
						t.Fatal("index diff should be clean")
					}
				} else if len(got.Content.Changes) != 1 || got.Content.Changes[0].Kind != "added" {
					t.Fatalf("addition %s: %+v", basis, got)
				}
			}
			statusWrite(t, root, "untracked", "untracked\n")
			for _, basis := range []GitDiffBasis{GitDiffHead, GitDiffIndex} {
				got := getGitDiff(t, root, "untracked", basis)
				if len(got.Content.Changes) != 1 || got.Content.Changes[0].Kind != "added" {
					t.Fatal(got)
				}
			}
			if _, err := ReadGitDiff(context.Background(), EditPolicy{}, root, "untracked", GitDiffStaged); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("untracked staged: %v", err)
			}
		})
	}
	root, git := blameRepo(t)
	statusWrite(t, root, "deleted", "old\n")
	statusWrite(t, root, "empty", "")
	git("add", ".")
	git("commit", "-m", "base")
	gitOK(t, os.Remove(filepath.Join(root, "deleted")))
	for _, basis := range []GitDiffBasis{GitDiffHead, GitDiffIndex} {
		got := getGitDiff(t, root, "deleted", basis)
		if !got.Deleted || !got.ReadOnly || got.Content.Text != "" || len(got.Content.Changes) != 1 || got.Content.Changes[0].Kind != "deleted" || got.Content.Changes[0].Removed[0] != "old" {
			t.Fatalf("deletion %s %+v", basis, got)
		}
	}
	git("add", "-A")
	got := getGitDiff(t, root, "deleted", GitDiffStaged)
	if !got.Deleted || len(got.Content.Changes) != 1 {
		t.Fatalf("staged delete %+v", got)
	}
	git("rm", "empty")
	got = getGitDiff(t, root, "empty", GitDiffStaged)
	if !got.Deleted || got.Content.Text != "" {
		t.Fatal(got)
	}
}
func TestGitDiffSubdirLiteralAndProtection(t *testing.T) {
	root, git := blameRepo(t)
	for _, p := range append([]string{"sub/literal[1]", "outside", "auth.json", "store/data"}, colonNames("sub/:colon")...) {
		statusWrite(t, root, p, "base\n")
	}
	git("add", ".")
	git("commit", "-m", "base")
	statusWrite(t, root, "sub/literal[1]", "staged\n")
	for _, p := range colonNames("sub/:colon") {
		statusWrite(t, root, p, "colon\n")
	}
	git("add", ".")
	for _, p := range append([]string{"literal[1]"}, colonNames(":colon")...) {
		got := getGitDiff(t, filepath.Join(root, "sub"), p, GitDiffStaged)
		if got.Content.Path != p || len(got.Content.Changes) != 1 {
			t.Fatalf("literal/subdir %+v", got)
		}
	}
	for _, p := range []string{"../outside", "/absolute", "", ".git/config", "nested/../../outside", "auth.json", "store/data"} {
		_, err := ReadGitDiff(context.Background(), EditPolicy{ProtectedRoots: []string{filepath.Join(root, "store")}}, root, p, GitDiffStaged)
		if err == nil {
			t.Errorf("accepted protected/outside %q", p)
		}
	}
	if _, err := ReadGitDiff(context.Background(), EditPolicy{}, root, "outside", "invalid"); err == nil {
		t.Fatal("invalid basis")
	}
	if runtime.GOOS != "windows" {
		gitOK(t, os.Symlink(filepath.Join(root, "outside"), filepath.Join(root, "link")))
		git("add", "link")
		if _, err := ReadGitDiff(context.Background(), EditPolicy{}, root, "link", GitDiffStaged); err == nil {
			t.Fatal("link read")
		}
	}
}
func TestGitDiffBinaryLargeAndMissing(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "binary", "a\x00b")
	statusWrite(t, root, "large", strings.Repeat("x", MaxReadBytes+1))
	git("add", ".")
	git("commit", "-m", "base")
	statusWrite(t, root, "binary", "changed\x00binary")
	statusWrite(t, root, "large", strings.Repeat("y", MaxReadBytes+1))
	git("add", ".")
	for _, basis := range []GitDiffBasis{GitDiffHead, GitDiffIndex, GitDiffStaged} {
		binary := getGitDiff(t, root, "binary", basis)
		if !binary.Content.Binary || binary.Content.Text != "" || len(binary.Content.Changes) != 0 {
			t.Fatalf("binary %s %+v", basis, binary)
		}
		large := getGitDiff(t, root, "large", basis)
		if !large.Content.TooLarge || large.Content.Text != "" || len(large.Content.Changes) != 0 {
			t.Fatalf("large %s %+v", basis, large)
		}
	}
	git("rm", "-f", "binary", "large")
	for _, basis := range []GitDiffBasis{GitDiffHead, GitDiffStaged} {
		if !getGitDiff(t, root, "large", basis).Content.TooLarge || !getGitDiff(t, root, "binary", basis).Content.Binary {
			t.Fatal("deleted old blob not bounded/classified")
		}
	}
	if _, err := ReadGitDiff(context.Background(), EditPolicy{}, root, "missing", GitDiffHead); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: %v", err)
	}
}

func TestGitDiffStagedDeletionRecreatedWorkingAndEmptyDeleted(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "old\n")
	statusWrite(t, root, "empty", "")
	git("add", ".")
	git("commit", "-m", "base")
	git("rm", "file", "empty")
	statusWrite(t, root, "file", "new\n")
	head := getGitDiff(t, root, "file", GitDiffHead)
	if len(head.Content.Changes) != 1 || head.Content.Changes[0].Kind != "modified" || head.Content.Changes[0].Removed[0] != "old" {
		t.Fatalf("head recreated: %+v", head)
	}
	index := getGitDiff(t, root, "file", GitDiffIndex)
	if len(index.Content.Changes) != 1 || index.Content.Changes[0].Kind != "added" {
		t.Fatalf("index recreated: %+v", index)
	}
	empty := getGitDiff(t, root, "empty", GitDiffHead)
	if !empty.Deleted || !empty.ReadOnly {
		t.Fatalf("empty deletion: %+v", empty)
	}
}
func TestGitDiffCancellationAndUnmerged(t *testing.T) {
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	branch := git("branch", "--show-current")
	git("checkout", "-b", "other")
	statusWrite(t, root, "file", "other\n")
	git("commit", "-am", "other")
	git("checkout", branch)
	statusWrite(t, root, "file", "ours\n")
	git("commit", "-am", "ours")
	cmd := exec.Command("git", "-C", root, "merge", "other")
	if err := cmd.Run(); err == nil {
		t.Fatal("expected conflict")
	}
	if _, err := ReadGitDiff(context.Background(), EditPolicy{}, root, "file", GitDiffStaged); err == nil || !strings.Contains(err.Error(), "unmerged") {
		t.Fatalf("unmerged staged: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadGitDiff(ctx, EditPolicy{}, root, "file", GitDiffHead); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestGitSubmoduleStatusDiffAndDiscard(t *testing.T) {
	ctx := context.Background()
	root, git := blameRepo(t)
	ui := func(args ...string) string { return git(append([]string{"-C", "ui"}, args...)...) }
	statusWrite(t, root, "ui/file", "one\n")
	ui("init")
	ui("config", "user.name", "UI Author")
	ui("config", "user.email", "ui@example.test")
	ui("config", "commit.gpgsign", "false")
	ui("add", ".")
	ui("commit", "-m", "one")
	old := "Subproject commit " + ui("rev-parse", "HEAD")
	statusWrite(t, root, "top", "top\n")
	git("add", "top", "ui") // the embedded repository is recorded as a gitlink
	git("commit", "-m", "add ui")
	statusWrite(t, root, "ui/file", "two\n")
	ui("commit", "-am", "two")
	now := "Subproject commit " + ui("rev-parse", "HEAD")
	statusWrite(t, root, "ui/file", "dirty\n")

	status := repoStatus(t, root)
	if len(status.Entries) != 1 || status.Entries[0].Path != "ui" || status.Entries[0].Worktree != "M" ||
		!reflect.DeepEqual(status.Entries[0].Submodule, &GitSubmoduleChange{Commit: true, Modified: true}) {
		t.Fatalf("status: %+v", status.Entries)
	}
	for _, tc := range []struct {
		basis   GitDiffBasis
		text    string
		removed []string
	}{{GitDiffIndex, now + "-dirty\n", []string{old}}, {GitDiffHead, now + "-dirty\n", []string{old}}, {GitDiffStaged, old + "\n", nil}} {
		got := getGitDiff(t, root, "ui", tc.basis)
		var removed []string
		if len(got.Content.Changes) == 1 {
			removed = got.Content.Changes[0].Removed
		}
		if !got.Submodule || !got.ReadOnly || got.Deleted || got.Content.Text != tc.text || !reflect.DeepEqual(removed, tc.removed) ||
			(tc.removed == nil) != (len(got.Content.Changes) == 0) || got.Content.Edit == nil || got.Content.Edit.Eligible {
			t.Fatalf("%s: %+v", tc.basis, got)
		}
	}
	if got := getGitDiff(t, root, "top", GitDiffIndex); got.Submodule {
		t.Fatalf("a file read as a submodule: %+v", got)
	}

	gitOK(t, GitStage(ctx, root, []string{"ui"}))
	if got := getGitDiff(t, root, "ui", GitDiffStaged); got.Content.Text != now+"\n" || len(got.Content.Changes) != 1 || !reflect.DeepEqual(got.Content.Changes[0].Removed, []string{old}) {
		t.Fatalf("staged: %+v", got)
	}
	if got := repoStatus(t, root).Entries; len(got) != 1 || got[0].Index != "M" || !got[0].Submodule.Modified || got[0].Submodule.Commit {
		t.Fatalf("staged status: %+v", got)
	}
	err := GitDiscard(ctx, EditPolicy{}, root, GitDiscardRequest{Paths: []string{"ui"}})
	if err == nil || !strings.Contains(err.Error(), "cannot discard ui: it is a submodule") {
		t.Fatalf("discard: %v", err)
	}
	if text, _ := os.ReadFile(filepath.Join(root, "ui", "file")); string(text) != "dirty\n" {
		t.Fatalf("submodule changed: %q", text)
	}
}

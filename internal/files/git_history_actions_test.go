package files

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func actionFixture(t *testing.T) (string, func(...string) string) {
	t.Helper()
	root, git := blameRepo(t)
	statusWrite(t, root, "file", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	return root, git
}
func actionRequest(t *testing.T, root, action, ref string) GitHistoryRefActionRequest {
	t.Helper()
	req := GitHistoryRefActionRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Action: action, Ref: ref}
	gitOK(t, withGitWrite(context.Background(), root, func(w gitWriter) error {
		var err error
		req.ExpectedOID, err = historyActionRef(w, ref)
		if err != nil {
			return err
		}
		req.ExpectedHead, err = historyActionHead(w)
		return err
	}))
	return req
}
func runAction(req GitHistoryRefActionRequest) error {
	_, err := GitHistoryRefAction(context.Background(), EditPolicy{}, req)
	return err
}
func TestHistoryActionCheckoutAndIdentities(t *testing.T) {
	root, git := actionFixture(t)
	git("branch", "other")
	req := actionRequest(t, root, "checkout", "refs/heads/other")
	bad := req
	bad.ExpectedOID = strings.Repeat("0", 40)
	if runAction(bad) == nil {
		t.Fatal("stale selected ref accepted")
	}
	bad = req
	bad.ExpectedHead.SymbolicRef = "refs/heads/wrong"
	if runAction(bad) == nil {
		t.Fatal("stale symbolic HEAD accepted")
	}
	bad = req
	bad.ExpectedHead.OID = strings.Repeat("1", 40)
	if runAction(bad) == nil {
		t.Fatal("stale HEAD OID accepted")
	}
	bad = req
	bad.DraftPaths = []string{"new-unsaved"}
	if runAction(bad) == nil {
		t.Fatal("draft allowed")
	}
	gitOK(t, runAction(req))
	if git("symbolic-ref", "HEAD") != "refs/heads/other" {
		t.Fatal("did not switch")
	}
	if runAction(req) == nil {
		t.Fatal("stale HEAD allowed")
	}
	if runAction(actionRequest(t, root, "checkout", "refs/heads/other")) == nil {
		t.Fatal("current checkout allowed")
	}
	git("tag", "-a", "v1", "-m", "tag")
	req = actionRequest(t, root, "checkout", "refs/tags/v1")
	if req.ExpectedOID == git("rev-parse", "HEAD") {
		t.Fatal("fixture not annotated")
	}
	gitOK(t, runAction(req))
	if got := repoStatus(t, root); !got.Detached || !strings.HasPrefix(git("rev-parse", "v1^{commit}"), got.Head) {
		t.Fatalf("not detached tag: %+v", got)
	}
}
func TestHistoryActionCheckoutDirtyAndOperations(t *testing.T) {
	root, git := actionFixture(t)
	git("checkout", "-b", "other")
	statusWrite(t, root, "file", "other\n")
	git("commit", "-am", "other")
	git("checkout", "-")
	statusWrite(t, root, "file", "dirty\n")
	req := actionRequest(t, root, "checkout", "refs/heads/other")
	if runAction(req) == nil {
		t.Fatal("overwriting dirty checkout succeeded")
	}
	data, _ := os.ReadFile(filepath.Join(root, "file"))
	if string(data) != "dirty\n" {
		t.Fatal("dirty contents lost")
	}
	git("restore", "file")
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG"} {
		gitOK(t, os.WriteFile(filepath.Join(root, ".git", marker), []byte("state"), 0600))
		if runAction(req) == nil {
			t.Fatalf("operation %s allowed", marker)
		}
		gitOK(t, os.Remove(filepath.Join(root, ".git", marker)))
	}
}
func TestHistoryActionRemoteCheckout(t *testing.T) {
	root, git := actionFixture(t)
	git("remote", "add", "origin", "https://invalid.invalid/repo")
	git("update-ref", "refs/remotes/origin/topic", "HEAD")
	req := actionRequest(t, root, "checkout", "refs/remotes/origin/topic")
	gitOK(t, runAction(req))
	if git("symbolic-ref", "HEAD") != "refs/heads/topic" || git("rev-parse", "--symbolic-full-name", "@{upstream}") != req.Ref {
		t.Fatal("not tracking selected branch")
	}
	git("checkout", "-")
	req = actionRequest(t, root, "checkout", req.Ref)
	gitOK(t, runAction(req))
	git("checkout", "-")
	git("config", "branch.topic.merge", "refs/heads/wrong")
	if runAction(actionRequest(t, root, "checkout", req.Ref)) == nil {
		t.Fatal("conflicting upstream reused")
	}
}
func TestHistoryActionScopePolicyAndTargets(t *testing.T) {
	root, git := actionFixture(t)
	statusWrite(t, root, "sub/file", "base")
	git("add", ".")
	git("commit", "-m", "sub")
	git("branch", "other")
	req := actionRequest(t, root, "checkout", "refs/heads/other")
	req.Dir = filepath.Join(root, "sub")
	if runAction(req) == nil {
		t.Fatal("scope expansion unacknowledged")
	}
	req.AcknowledgeWholeRepository = true
	policy := EditPolicy{ProtectedRoots: []string{filepath.Join(root, "secret-store")}}
	if _, err := GitHistoryRefAction(context.Background(), policy, req); !errors.Is(err, ErrEditProtected) {
		t.Fatalf("policy bypass: %v", err)
	}
	gitOK(t, runAction(req))
	req = actionRequest(t, root, "checkout", "refs/heads/"+git("branch", "--show-current"))
	for _, repo := range []string{"../", ".git", "sub"} {
		req.Repo = repo
		if runAction(req) == nil {
			t.Fatalf("invalid repository %q allowed", repo)
		}
	}
}
func TestHistoryActionProtectedAffectedPath(t *testing.T) {
	root, git := actionFixture(t)
	git("checkout", "-b", "unsafe")
	statusWrite(t, root, ".credentials-test", "secret")
	git("add", ".")
	git("commit", "-m", "secret")
	git("checkout", "-")
	req := actionRequest(t, root, "checkout", "refs/heads/unsafe")
	if err := runAction(req); !errors.Is(err, ErrEditProtected) {
		t.Fatalf("protected checkout: %v", err)
	}
	req.Action = "merge"
	if err := runAction(req); !errors.Is(err, ErrEditProtected) {
		t.Fatalf("protected merge: %v", err)
	}
}
func TestHistoryActionMergeAndConflictPreservation(t *testing.T) {
	root, git := actionFixture(t)
	main := git("branch", "--show-current")
	git("checkout", "-b", "topic")
	statusWrite(t, root, "topic", "new")
	git("add", ".")
	git("commit", "-m", "topic")
	git("checkout", main)
	req := actionRequest(t, root, "merge", "refs/heads/topic")
	statusWrite(t, root, "untracked", "draft saved")
	if runAction(req) == nil {
		t.Fatal("dirty merge succeeded")
	}
	gitOK(t, os.Remove(filepath.Join(root, "untracked")))
	gitOK(t, runAction(req))
	if git("rev-parse", "HEAD") != req.ExpectedOID {
		t.Fatal("merge failed")
	}
	git("checkout", "topic")
	statusWrite(t, root, "file", "topic\n")
	git("commit", "-am", "topic change")
	git("checkout", main)
	statusWrite(t, root, "file", "main\n")
	git("commit", "-am", "main change")
	req = actionRequest(t, root, "merge", "refs/heads/topic")
	err := runAction(req)
	if err == nil || !strings.Contains(err.Error(), "preserved") {
		t.Fatalf("conflict not reported: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "MERGE_HEAD")); err != nil {
		t.Fatal("conflict auto-aborted")
	}
	if git("ls-files", "-u") == "" {
		t.Fatal("conflict not retained")
	}
}
func TestHistoryActionDeleteSafeAndLocalOnly(t *testing.T) {
	root, git := actionFixture(t)
	git("branch", "merged")
	gitOK(t, runAction(actionRequest(t, root, "delete", "refs/heads/merged")))
	if runAction(actionRequest(t, root, "delete", "refs/heads/"+git("branch", "--show-current"))) == nil {
		t.Fatal("deleted current branch")
	}
	git("checkout", "-b", "unmerged")
	statusWrite(t, root, "new", "new")
	git("add", ".")
	git("commit", "-m", "unmerged")
	git("checkout", "-")
	if runAction(actionRequest(t, root, "delete", "refs/heads/unmerged")) == nil {
		t.Fatal("deleted unmerged branch")
	}
	for _, ref := range []string{"refs/tags/v1", "refs/remotes/origin/topic"} {
		git("update-ref", ref, "HEAD")
		req := actionRequest(t, root, "delete", ref)
		git("update-ref", ref, git("rev-parse", "unmerged"))
		if runAction(req) == nil {
			t.Fatal("stale delete allowed")
		}
		gitOK(t, runAction(actionRequest(t, root, "delete", ref)))
	}
	git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/topic")
	// Dangling aliases must never be treated as an actionable direct ref.
	if _, err := GitHistoryRefAction(context.Background(), EditPolicy{}, GitHistoryRefActionRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Action: "delete", Ref: "refs/remotes/origin/HEAD", ExpectedOID: git("rev-parse", "HEAD")}); err == nil {
		t.Fatal("symbolic alias accepted")
	}
}
func TestHistoryActionLinkedWorktreeDelete(t *testing.T) {
	root, git := actionFixture(t)
	git("branch", "linked")
	linked := filepath.Join(t.TempDir(), "linked")
	git("worktree", "add", linked, "linked")
	if runAction(actionRequest(t, root, "delete", "refs/heads/linked")) == nil {
		t.Fatal("deleted linked-worktree branch")
	}
}
func TestHistoryActionPushSelectedOnly(t *testing.T) {
	root, git := actionFixture(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %s: %v", out, err)
	}
	git("remote", "add", "origin", bare)
	git("branch", "selected")
	git("branch", "unrelated")
	git("tag", "-a", "selected-tag", "-m", "tag")
	git("tag", "unrelated-tag")
	// Force-prone configuration must not add refs or change the requested scope.
	git("config", "remote.origin.mirror", "true")
	git("config", "push.followTags", "true")
	git("config", "remote.origin.push", "+refs/heads/*:refs/heads/*")
	for _, ref := range []string{"refs/heads/selected", "refs/tags/selected-tag"} {
		gitOK(t, runAction(actionRequest(t, root, "push", ref)))
	}
	cmd = exec.Command("git", "-C", bare, "for-each-ref", "--format=%(refname)")
	out, err := cmd.Output()
	gitOK(t, err)
	if string(out) != "refs/heads/selected\nrefs/tags/selected-tag\n" {
		t.Fatalf("unrelated push: %s", out)
	}
	if got := git("config", "--default", "", "--get", "branch.selected.remote"); got != "" {
		t.Fatal("push changed upstream")
	}
}
func TestHistoryActionCancellationAndSharedLock(t *testing.T) {
	root, git := actionFixture(t)
	git("tag", "v1")
	req := actionRequest(t, root, "delete", "refs/tags/v1")
	common, err := filepath.EvalSymlinks(filepath.Join(root, ".git"))
	gitOK(t, err)
	unlock, err := lockGitWrite(context.Background(), common)
	gitOK(t, err)
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := GitHistoryRefAction(ctx, EditPolicy{}, req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock did not observe cancellation: %v", err)
	}
	if git("rev-parse", "refs/tags/v1") != req.ExpectedOID {
		t.Fatal("canceled delete mutated ref")
	}
}

func TestHistoryActionPushRejectionCancellationAndAuth(t *testing.T) {
	root, git := actionFixture(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	out, err := exec.Command("git", "init", "--bare", bare).CombinedOutput()
	if err != nil {
		t.Fatalf("init: %s: %v", out, err)
	}
	git("remote", "add", "origin", bare)
	git("branch", "selected")
	ref := "refs/heads/selected"
	gitOK(t, runAction(actionRequest(t, root, "push", ref)))
	base := git("rev-parse", "HEAD")
	statusWrite(t, root, "file", "new\n")
	git("commit", "-am", "new")
	git("branch", "-f", "selected", "HEAD")
	gitOK(t, runAction(actionRequest(t, root, "push", ref)))
	git("branch", "-f", "selected", base)
	err = runAction(actionRequest(t, root, "push", ref))
	var syncErr *GitSyncError
	if !errors.As(err, &syncErr) || syncErr.Code != GitSyncNonFastForward {
		t.Fatalf("nonforce rejection: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell hooks require Unix")
	}
	hook := filepath.Join(bare, "hooks", "pre-receive")
	gitOK(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\nprintf 'authentication failed\\n' >&2\nexit 1\n"), 0700))
	git("branch", "auth", "HEAD")
	err = runAction(actionRequest(t, root, "push", "refs/heads/auth"))
	if !errors.As(err, &syncErr) || syncErr.Code != GitSyncAuthRequired {
		t.Fatalf("auth mapping: %v", err)
	}
	marker := filepath.Join(t.TempDir(), "started")
	gitOK(t, os.WriteFile(hook, []byte("#!/bin/sh\ncat >/dev/null\ntouch '"+marker+"'\nsleep 30\n"), 0700))
	req := actionRequest(t, root, "push", "refs/heads/auth")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(marker); err == nil {
				cancel()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
	}()
	start := time.Now()
	_, err = GitHistoryRefAction(ctx, EditPolicy{}, req)
	if !errors.As(err, &syncErr) || syncErr.Code != GitSyncCanceled || !errors.Is(err, context.Canceled) || time.Since(start) > 10*time.Second {
		t.Fatalf("cancel: %v after %s", err, time.Since(start))
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("push never reached local transport")
	}
}

func TestHistoryActionNonCommitTagAndDetachedMerge(t *testing.T) {
	root, git := actionFixture(t)
	blob := git("rev-parse", "HEAD:file")
	git("update-ref", "refs/tags/blob", blob)
	req := actionRequest(t, root, "checkout", "refs/tags/blob")
	if runAction(req) == nil {
		t.Fatal("noncommit checkout allowed")
	}
	gitOK(t, runAction(actionRequest(t, root, "delete", "refs/tags/blob")))
	git("tag", "lightweight")
	gitOK(t, runAction(actionRequest(t, root, "checkout", "refs/tags/lightweight")))
	if runAction(actionRequest(t, root, "merge", "refs/tags/lightweight")) == nil {
		t.Fatal("detached merge allowed")
	}
}
func TestHistoryActionChildRepository(t *testing.T) {
	root, git := actionFixture(t)
	parent := filepath.Dir(root)
	git("branch", "child-action")
	req := actionRequest(t, root, "checkout", "refs/heads/child-action")
	req.Dir = parent
	req.Repo = filepath.Base(root)
	gitOK(t, runAction(req))
	if git("symbolic-ref", "HEAD") != req.Ref {
		t.Fatal("child target ignored")
	}
}

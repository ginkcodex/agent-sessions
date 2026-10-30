package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GitHistoryHeadIdentity distinguishes an attached branch, detached HEAD and an
// unborn branch. OID is the direct HEAD object; SymbolicRef is empty if detached.
type GitHistoryHeadIdentity struct {
	SymbolicRef string `json:"symbolicRef"`
	OID         string `json:"oid"`
}

// GitHistoryRefActionRequest must be built from the confirmed host/workspace
// snapshot. ExpectedOID is the raw ref object, NOT an annotated tag's commit.
// Host routing, original Dir authorization and GitWrite/SaveFile (plus GitSync
// for push) permission checks belong to the engine/transport, not discovery.
type GitHistoryRefActionRequest struct {
	GitHistoryTarget
	Action                     string                 `json:"action"`
	Ref                        string                 `json:"ref"`
	ExpectedOID                string                 `json:"expectedOID"`
	ExpectedHead               GitHistoryHeadIdentity `json:"expectedHead"`
	DraftPaths                 []string               `json:"draftPaths,omitempty"`
	AcknowledgeWholeRepository bool                   `json:"acknowledgeWholeRepository,omitempty"`
}

type GitHistoryRefActionResult struct {
	Action  string `json:"action"`
	Ref     string `json:"ref"`
	Summary string `json:"summary"`
}

// GitHistoryRefAction never force-switches, force-pushes or force-deletes. Errors
// (including cancellation) can have partial effects: callers must refresh even
// on failure. In particular a failed merge deliberately preserves conflicts.
func GitHistoryRefAction(ctx context.Context, policy EditPolicy, req GitHistoryRefActionRequest) (GitHistoryRefActionResult, error) {
	result := GitHistoryRefActionResult{Action: req.Action, Ref: req.Ref}
	switch req.Action {
	case "checkout", "merge", "push", "delete":
	default:
		return result, errors.New("invalid Git History action")
	}
	resolved, err := resolveGitHistoryTarget(ctx, req.GitHistoryTarget)
	if err != nil {
		return result, err
	}
	timeout := gitWriteTimeout
	if req.Action == "push" {
		timeout = gitSyncTimeout
	}
	err = withGitRepo(ctx, resolved.Root, timeout, func(w gitWriter) error {
		// Re-resolve after waiting for the common-dir lock. Discovery is not an
		// authorization grant and a changed target must not redirect a confirmed write.
		current, err := resolveGitHistoryTarget(w.ctx, req.GitHistoryTarget)
		if err != nil {
			return err
		}
		if current != resolved {
			return errors.New("Git repository target changed; refresh and confirm again")
		}
		w.env = gitHistoryEnv()
		length, err := historyOIDLength(w)
		if err != nil {
			return err
		}
		if !validHistoryOID(req.ExpectedOID, length) || (req.ExpectedHead.OID != "" && !validHistoryOID(req.ExpectedHead.OID, length)) {
			return errors.New("invalid expected Git object identity")
		}
		kind := ""
		for _, k := range []string{"heads", "remotes", "tags"} {
			if strings.HasPrefix(req.Ref, "refs/"+k+"/") {
				kind = k
			}
		}
		if kind == "" {
			return errors.New("action requires an exact branch or tag ref, not HEAD or a revision")
		}
		if _, err := w.run("", "check-ref-format", req.Ref); err != nil {
			return err
		}
		oid, err := historyActionRef(w, req.Ref)
		if err != nil {
			return err
		}
		if oid != req.ExpectedOID {
			return errors.New("selected ref changed; refresh and confirm again")
		}
		head, err := historyActionHead(w)
		if err != nil {
			return err
		}
		if head != req.ExpectedHead {
			return errors.New("HEAD changed; refresh and confirm again")
		}
		if err := policy.refuse(false, req.Dir, resolved.Workspace, resolved.Root); err != nil {
			return err
		}
		switch req.Action {
		case "checkout", "merge":
			if len(req.DraftPaths) > 0 {
				return errors.New("save or discard unsaved drafts before changing the working tree")
			}
			if resolved.Scope != "" && !req.AcknowledgeWholeRepository {
				return errors.New("this action affects the whole repository; explicit acknowledgement is required")
			}
			// Even acknowledged scope expansion must not bypass host-local protection.
			if err := policy.refuse(true, resolved.Root); err != nil {
				return err
			}
			if err := historyActionState(w, req.Action == "merge"); err != nil {
				return err
			}
			commit, err := w.run("", "rev-parse", "--verify", req.ExpectedOID+"^{commit}")
			if err != nil {
				return fmt.Errorf("selected ref does not resolve to a commit: %w", err)
			}
			tip := strings.TrimSpace(string(commit))
			if !validHistoryOID(tip, length) {
				return errors.New("invalid selected commit")
			}
			if err := historyActionPaths(w, policy, head.OID, tip); err != nil {
				return err
			}
			if req.Action == "merge" {
				if head.SymbolicRef == "" || head.OID == "" {
					return errors.New("merge requires an attached, born HEAD")
				}
				// Ignore merge.ff=only and automatic stash configuration; do not discard
				// changes, rebase or auto-abort a conflicting merge.
				_, err = w.run("", "-c", "merge.autoStash=false", "merge", "--ff", "--no-edit", "--no-autostash", "--", tip)
				if err != nil {
					return fmt.Errorf("merge failed; conflicts or partial effects are preserved. Finish the merge or run git merge --abort in a terminal: %w", err)
				}
			} else {
				err = historyActionCheckout(w, req.Ref, kind, tip, head)
			}
			if err != nil {
				return err
			}
		case "delete":
			switch kind {
			case "heads":
				// Native -d retains mergedness and linked-worktree protections. Unlike
				// update-ref this command does not offer compare-and-delete semantics.
				_, err = w.run("", "branch", "-d", "--", strings.TrimPrefix(req.Ref, "refs/heads/"))
			default:
				_, err = w.run("", "update-ref", "--no-deref", "-d", req.Ref, req.ExpectedOID)
			}
			if err != nil {
				return err
			}
		case "push":
			if kind == "remotes" {
				return errors.New("remote-tracking refs cannot be pushed")
			}
			remotes, err := w.run("", "remote")
			if err != nil {
				return err
			}
			found := false
			for r := range strings.SplitSeq(string(remotes), "\n") {
				found = found || r == "origin"
			}
			if !found {
				return syncRefusal(GitSyncNoRemote, "This repository has no origin remote.")
			}
			env, err := gitSyncEnv(w)
			if err != nil {
				return err
			}
			w.env = append([]string{"GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1"}, env...)
			w.session = true
			// Pin the source object, not a moving ref. Disable mirror/followTags so
			// repository configuration cannot add unrelated refs or force updates.
			_, err = w.run("", "-c", "remote.origin.mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "origin", req.ExpectedOID+":"+req.Ref)
			if err != nil {
				return err
			}
		}
		result.Summary = fmt.Sprintf("%s completed for %s.", req.Action, req.Ref)
		if req.Action == "delete" && kind == "remotes" {
			result.Summary = "Deleted local remote-tracking ref only; the remote server was not changed."
		}
		return nil
	})
	if err != nil && req.Action == "push" {
		err = gitSyncFailure(GitSyncPush, err)
	}
	return result, err
}

func historyActionRef(w gitWriter, ref string) (string, error) {
	out, err := w.run("", "for-each-ref", "--format=%(refname)%00%(objectname)%00%(symref)", ref)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) == 3 && fields[0] == ref {
			if fields[2] != "" {
				return "", errors.New("symbolic refs are not actionable")
			}
			return fields[1], nil
		}
	}
	return "", errors.New("selected ref no longer exists")
}

func historyActionHead(w gitWriter) (GitHistoryHeadIdentity, error) {
	var head GitHistoryHeadIdentity
	symbolic, err := w.run("", "symbolic-ref", "-q", "HEAD")
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return head, err
		}
	} else {
		head.SymbolicRef = strings.TrimSuffix(string(symbolic), "\n")
	}
	oid, err := w.run("", "rev-parse", "--verify", "HEAD")
	if err != nil {
		unborn, e := historyHeadUnborn(w)
		if e != nil {
			return head, e
		}
		if !unborn {
			return head, err
		}
	} else {
		head.OID = strings.TrimSpace(string(oid))
	}
	return head, nil
}

func historyActionState(w gitWriter, clean bool) error {
	gitdir, err := w.run("", "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}
	for _, marker := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "sequencer", "BISECT_LOG"} {
		_, err := os.Stat(filepath.Join(strings.TrimSuffix(string(gitdir), "\n"), marker))
		if err == nil {
			return fmt.Errorf("repository operation in progress (%s); finish or abort it first", marker)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	conflicts, err := w.run("", "ls-files", "-u", "-z")
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return errors.New("resolve repository conflicts first")
	}
	if clean {
		out, err := w.run("", "status", "--porcelain=v1", "-z", "--untracked-files=all")
		if err != nil {
			return err
		}
		if len(out) > 0 {
			return errors.New("merge requires a clean index and working tree")
		}
	}
	return nil
}

func historyActionPaths(w gitWriter, policy EditPolicy, head, tip string) error {
	var out []byte
	var err error
	if head == "" {
		out, err = w.run("", "ls-tree", "-r", "--name-only", "-z", tip)
	} else {
		out, err = w.run("", "diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-only", "-z", head, tip, "--")
	}
	if err != nil {
		return err
	}
	paths := strings.Split(string(out), "\x00")
	if len(paths) > MaxGitChanges+1 {
		return errors.New("too many affected paths for safe action")
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if err := gitDiscardPreflight(policy, w.root, path); err != nil {
			return fmt.Errorf("cannot change %s: %w", path, err)
		}
	}
	return nil
}

func historyActionCheckout(w gitWriter, ref, kind, tip string, head GitHistoryHeadIdentity) error {
	switch kind {
	case "tags":
		_, err := w.run("", "switch", "--detach", "--no-guess", tip)
		return err
	case "heads":
		if ref == head.SymbolicRef {
			return errors.New("selected branch is already current")
		}
		_, err := w.run("", "switch", "--no-guess", "--", strings.TrimPrefix(ref, "refs/heads/"))
		return err
	default:
		remoteAndBranch := strings.TrimPrefix(ref, "refs/remotes/")
		_, local, ok := strings.Cut(remoteAndBranch, "/")
		if !ok || !validSyncName(local) {
			return errors.New("invalid remote branch name")
		}
		if _, err := w.run("", "check-ref-format", "--branch", local); err != nil {
			return err
		}
		existing, err := w.run("", "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)", "refs/heads/"+local)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(existing), "\n") {
			fields := strings.Split(line, "\x00")
			if len(fields) == 3 && fields[0] == "refs/heads/"+local {
				if fields[1] != tip || fields[2] != ref {
					return errors.New("local branch name or upstream conflicts with selected remote branch")
				}
				if fields[0] == head.SymbolicRef {
					return errors.New("compatible local branch is already current")
				}
				_, err := w.run("", "switch", "--no-guess", "--", local)
				return err
			}
		}
		_, err = w.run("", "switch", "--no-guess", "--track", "-c", local, ref)
		return err
	}
}

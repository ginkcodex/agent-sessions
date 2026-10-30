package files

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// Deny every transport as a second line of defense on Git versions which do not
// honor GIT_NO_LAZY_FETCH. History must fail on missing promisor objects offline.
func gitHistoryEnv() []string {
	return []string{"GIT_OPTIONAL_LOCKS=0", "GIT_NO_LAZY_FETCH=1", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ALLOW_PROTOCOL="}
}

// historyHeadUnborn reports whether HEAD names a branch with no ref yet. A
// detached or unreadable HEAD is not unborn; callers keep their original error.
// show-ref --exists needs Git 2.43, so list the exact ref for older LTS hosts.
// Patterns also match below a slash, so compare whole lines.
func historyHeadUnborn(w gitWriter) (bool, error) {
	symbolic, err := w.run("", "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return false, nil
	}
	ref := strings.TrimSuffix(string(symbolic), "\n")
	listed, err := w.run("", "for-each-ref", "--format=%(refname)", ref)
	if err != nil {
		return false, err
	}
	for line := range strings.SplitSeq(string(listed), "\n") {
		if line == ref {
			return false, nil
		}
	}
	return true, nil
}

// withGitHistory pins the resolved target for one bounded, coordinated read.
func withGitHistory(ctx context.Context, target GitHistoryTarget, action func(gitWriter, gitHistoryResolvedTarget) error) error {
	ctx, cancel := context.WithTimeout(ctx, GitHistoryTimeout)
	defer cancel()
	resolved, err := resolveGitHistoryTarget(ctx, target)
	if err != nil {
		return err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	w := gitWriter{ctx: ctx, root: resolved.Root, git: git, env: gitHistoryEnv()}
	common, err := w.run("", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	key, err := filepath.EvalSymlinks(strings.TrimSuffix(string(common), "\n"))
	if err != nil {
		return err
	}
	unlock, err := lockGitWrite(ctx, key)
	if err != nil {
		return err
	}
	defer unlock()
	return action(w, resolved)
}

func historyOIDLength(w gitWriter) (int, error) {
	format, err := w.run("", "rev-parse", "--show-object-format")
	if err != nil {
		return 0, err
	}
	switch strings.TrimSpace(string(format)) {
	case "sha1":
		return 40, nil
	case "sha256":
		return 64, nil
	default:
		return 0, errors.New("unsupported Git object format")
	}
}

func GitHistoryBranches(ctx context.Context, target GitHistoryTarget) (GitHistoryRefList, error) {
	result := GitHistoryRefList{Refs: []GitHistoryRef{}}
	err := withGitHistory(ctx, target, func(w gitWriter, _ gitHistoryResolvedTarget) error {
		length, err := historyOIDLength(w)
		if err != nil {
			return err
		}
		data, err := w.run("", "for-each-ref", "--sort=refname", "--format=%(refname)%00%(objectname)%00%(HEAD)%00%(symref)", "refs/heads/", "refs/remotes/", "refs/tags/")
		if err != nil {
			return err
		}
		result, err = parseHistoryRefs(data, length)
		if err != nil {
			return err
		}
		for i := range result.Refs {
			ref := &result.Refs[i]
			if ref.Kind != "tag" {
				continue
			}
			// Enumeration is capped and all peeling shares the overall read deadline.
			// ^{commit} recursively peels annotated tags, including nested tags.
			commit, peelErr := w.run("", "rev-parse", "--verify", ref.OID+"^{commit}")
			if peelErr != nil {
				if w.ctx.Err() != nil {
					return w.ctx.Err()
				}
				kind, typeErr := w.run("", "cat-file", "-t", ref.OID+"^{}")
				if typeErr != nil {
					return typeErr
				}
				if string(kind) == "blob\n" || string(kind) == "tree\n" {
					continue
				}
				return peelErr
			}
			ref.Commit = strings.TrimSpace(string(commit))
			if !validHistoryOID(ref.Commit, length) {
				return errors.New("invalid peeled tag commit")
			}
		}
		shallow, err := w.run("", "rev-parse", "--is-shallow-repository")
		if err != nil {
			return err
		}
		result.Shallow = strings.TrimSpace(string(shallow)) == "true"
		symbolic, symbolicErr := w.run("", "symbolic-ref", "-q", "HEAD")
		if symbolicErr == nil {
			result.HeadRef = strings.TrimSuffix(string(symbolic), "\n")
		} else {
			var exit *exec.ExitError
			if !errors.As(symbolicErr, &exit) || exit.ExitCode() != 1 {
				return symbolicErr
			}
		}
		head, err := w.run("", "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			// A symbolic HEAD pointing to an absent branch is unborn. Other failures
			// (corrupt refs, cancellation, missing objects) must not look empty.
			unborn, unbornErr := historyHeadUnborn(w)
			if unbornErr != nil {
				return unbornErr
			}
			if !unborn {
				return err
			}
			result.Unborn = true
			return nil
		}
		result.Head = strings.TrimSpace(string(head))
		if !validHistoryOID(result.Head, length) {
			return errors.New("invalid HEAD object")
		}
		_, err = w.run("", "symbolic-ref", "-q", "HEAD")
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 1 {
				return err
			}
			result.Detached = true
		}
		return nil
	})
	return result, err
}

func validHistoryOID(oid string, length int) bool {
	if (length != 40 && length != 64) || len(oid) != length {
		return false
	}
	for _, c := range oid {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func parseHistoryRefs(data []byte, oidLength int) (GitHistoryRefList, error) {
	out := GitHistoryRefList{Refs: []GitHistoryRef{}}
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		fields := strings.Split(string(line), "\x00")
		if len(fields) != 4 {
			return out, errors.New("invalid Git history ref record")
		}
		if fields[3] != "" {
			continue
		}
		ref := GitHistoryRef{Ref: fields[0], OID: fields[1], Current: fields[2] == "*"}
		switch {
		case strings.HasPrefix(ref.Ref, "refs/heads/"):
			ref.Name = strings.TrimPrefix(ref.Ref, "refs/heads/")
			ref.Kind, ref.Commit = "local", ref.OID
		case strings.HasPrefix(ref.Ref, "refs/remotes/"):
			ref.Name = strings.TrimPrefix(ref.Ref, "refs/remotes/")
			ref.Remote = true
			ref.Kind, ref.Commit = "remote", ref.OID
		case strings.HasPrefix(ref.Ref, "refs/tags/"):
			ref.Name = strings.TrimPrefix(ref.Ref, "refs/tags/")
			ref.Kind = "tag"
		default:
			return out, errors.New("invalid Git history ref namespace")
		}
		if !validHistoryOID(ref.OID, oidLength) {
			return out, errors.New("invalid Git history ref object")
		}
		if len(out.Refs) >= MaxGitBranches {
			out.Truncated = true
			continue
		}
		out.Refs = append(out.Refs, ref)
	}
	return out, nil
}

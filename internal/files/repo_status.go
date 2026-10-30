package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const MaxGitChanges = 5000

// RepoStatus keeps index and working-tree changes separate. Empty states mean
// unchanged; untracked entries have worktree state "?". Paths are project-relative.
type RepoStatus struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
	// RepoRoot is metadata only; actionable entry paths remain project-relative.
	RepoRoot   string `json:"repoRoot,omitempty"`
	Branch     string `json:"branch"`
	Head       string `json:"head"`
	Detached   bool   `json:"detached"`
	HasCommits bool   `json:"hasCommits"`
	Upstream   string `json:"upstream"`
	Ahead      int    `json:"ahead"`
	Behind     int    `json:"behind"`
	// HeadPublished says the last commit is on a remote-tracking branch, so
	// amending it would rewrite pushed history. Omitted by older servers.
	HeadPublished bool        `json:"headPublished"`
	Operation     string      `json:"operation,omitempty"`
	Entries       []GitChange `json:"entries"`
	Truncated     bool        `json:"truncated"`
}

type GitChange struct {
	Path         string `json:"path"`
	OriginalPath string `json:"originalPath,omitempty"`
	Index        string `json:"index"`
	Worktree     string `json:"worktree"`
	Conflict     bool   `json:"conflict"`
	// Submodule is set for a submodule: Git records the commit it is at.
	Submodule *GitSubmoduleChange `json:"submodule,omitempty"`
}

// GitSubmoduleChange says how a submodule differs: another commit checked
// out, changed tracked files, or untracked files.
type GitSubmoduleChange struct {
	Commit    bool `json:"commit"`
	Modified  bool `json:"modified"`
	Untracked bool `json:"untracked"`
}

// repoStatusError keeps Git's own message apart from the command that failed.
type repoStatusError struct {
	command string
	err     error
	stderr  string
}

func (e *repoStatusError) Error() string {
	return fmt.Sprintf("git %s: %v: %s", e.command, e.err, e.stderr)
}

func (e *repoStatusError) Unwrap() error { return e.err }

// repositoryUnavailable explains why Git found no repository for the project:
// usually there is none; otherwise Git's message says why, such as a
// repository owned by another user, with its suggested fix.
func repositoryUnavailable(stderr string) string {
	if strings.Contains(stderr, "not a git repository") {
		return "This project is not in a Git repository."
	}
	message := strings.Join(strings.Fields(strings.TrimPrefix(stderr, "fatal: ")), " ")
	if message == "" {
		return "Git cannot read this project's repository."
	}
	return "Git cannot read this project's repository: " + message
}

// headNotOnRemotes prints HEAD unless a remote-tracking branch contains it.
var headNotOnRemotes = []string{"rev-list", "--max-count=1", "HEAD", "--not", "--remotes", "--"}

// GitRepoStatus reports the whole project's changes, including deleted files.
// Missing git and non-repositories are normal unavailable results, not errors.
func GitRepoStatus(ctx context.Context, root string) (RepoStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	unavailable := RepoStatus{Entries: []GitChange{}}
	git, err := exec.LookPath("git")
	if err != nil {
		unavailable.Reason = "Git is not installed or not on PATH"
		return unavailable, nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return RepoStatus{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return RepoStatus{}, err
	}
	run := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, git, append([]string{"--no-optional-locks", "-C", root, "-c", "core.quotepath=off", "-c", "status.relativePaths=false"}, args...)...)
		// Git's messages in English, so a missing repository can be told apart.
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		hideConsole(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			return nil, &repoStatusError{command: args[0], err: err, stderr: strings.TrimSpace(stderr.String())}
		}
		return out, nil
	}
	top, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		var failed *repoStatusError
		var exit *exec.ExitError
		if errors.As(err, &failed) && errors.As(err, &exit) {
			unavailable.Reason = repositoryUnavailable(failed.stderr)
			return unavailable, nil
		}
		return RepoStatus{}, err
	}
	repo := strings.TrimSuffix(string(top), "\n")
	out, err := run("status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return RepoStatus{}, err
	}
	result, err := parseRepoStatus(out, repo, root)
	if err != nil {
		return RepoStatus{}, err
	}
	if result.HasCommits {
		unpushed, err := run(headNotOnRemotes...)
		if err != nil {
			return RepoStatus{}, err
		}
		result.HeadPublished = len(bytes.TrimSpace(unpushed)) == 0
	}
	gitdir, err := run("rev-parse", "--absolute-git-dir")
	if err != nil {
		return RepoStatus{}, err
	}
	// Per-worktree state lives in the worktree's gitdir, not the common dir.
	for _, op := range []struct{ marker, name string }{
		{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"},
		{"MERGE_HEAD", "merge"}, {"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"},
		{"sequencer", "sequencer"},
	} {
		_, err := os.Stat(filepath.Join(strings.TrimSuffix(string(gitdir), "\n"), op.marker))
		if err == nil {
			result.Operation = op.name
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return RepoStatus{}, err
		}
	}
	return result, nil
}

func parseRepoStatus(out []byte, repo, root string) (RepoStatus, error) {
	result := RepoStatus{OK: true, RepoRoot: repo, Entries: []GitChange{}}
	records := bytes.Split(out, []byte{0})
	relative := func(path string) string {
		rel, _ := filepath.Rel(root, filepath.Join(repo, filepath.FromSlash(path)))
		return filepath.ToSlash(rel)
	}
	for i := 0; i < len(records); i++ {
		record := string(records[i])
		if record == "" {
			continue
		}
		if strings.HasPrefix(record, "# ") {
			key, value, _ := strings.Cut(record[2:], " ")
			switch key {
			case "branch.oid":
				result.HasCommits = value != "(initial)"
				if result.HasCommits {
					result.Head = value
					if len(result.Head) > 7 {
						result.Head = result.Head[:7]
					}
				}
			case "branch.head":
				result.Detached = value == "(detached)"
				if !result.Detached {
					result.Branch = value
				}
			case "branch.upstream":
				result.Upstream = value
			case "branch.ab":
				counts := strings.Fields(value)
				if len(counts) != 2 {
					return RepoStatus{}, fmt.Errorf("invalid git branch counts")
				}
				var err error
				result.Ahead, err = strconv.Atoi(strings.TrimPrefix(counts[0], "+"))
				if err != nil {
					return RepoStatus{}, err
				}
				result.Behind, err = strconv.Atoi(strings.TrimPrefix(counts[1], "-"))
				if err != nil {
					return RepoStatus{}, err
				}
			}
			continue
		}
		entry := GitChange{}
		fields := 0
		switch record[0] {
		case '1':
			fields = 9
		case '2':
			fields = 10
		case 'u':
			fields = 11
			entry.Conflict = true
		case '?':
			entry.Path = relative(record[2:])
			entry.Worktree = "?"
		case '!':
			continue
		default:
			return RepoStatus{}, fmt.Errorf("unknown git status record")
		}
		if fields > 0 {
			parts := strings.SplitN(record, " ", fields)
			if len(parts) != fields || len(parts[1]) != 2 {
				return RepoStatus{}, fmt.Errorf("invalid git status record")
			}
			entry.Path = relative(parts[fields-1])
			entry.Index = strings.Trim(parts[1][:1], ".")
			entry.Worktree = strings.Trim(parts[1][1:], ".")
			// <sub> is N... for a file, else S<c><m><u>.
			if sub := parts[2]; len(sub) == 4 && sub[0] == 'S' {
				entry.Submodule = &GitSubmoduleChange{Commit: sub[1] == 'C', Modified: sub[2] == 'M', Untracked: sub[3] == 'U'}
			}
			if record[0] == '2' {
				i++
				if i >= len(records) || len(records[i]) == 0 {
					return RepoStatus{}, fmt.Errorf("missing git rename origin")
				}
				entry.OriginalPath = relative(string(records[i]))
				if entry.OriginalPath == ".." || strings.HasPrefix(entry.OriginalPath, "../") {
					entry.OriginalPath = ""
				}
			}
		}
		if entry.Path == ".." || strings.HasPrefix(entry.Path, "../") {
			continue
		}
		if len(result.Entries) == MaxGitChanges {
			result.Truncated = true
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}

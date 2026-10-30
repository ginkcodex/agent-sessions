package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var errGitHistoryNotRepository = errors.New("not a Git worktree")
var errGitHistoryInvalidRepository = errors.New("invalid Git worktree")

// gitHistoryResolvedTarget keeps authorization separate from repository identity.
// Workspace is the canonical authorized directory; Root may be its ancestor
// only for Repo == ".". Scope is a slash-separated prefix within Root, never a
// replacement authorization root. Target retains the caller's original workspace.
type gitHistoryResolvedTarget struct {
	Target    GitHistoryTarget
	Workspace string
	Root      string
	Repo      string
	Scope     string
}

// GitHistoryRepositories discovers choices, not authorization grants. Every later
// read must resolve its target again. A Git-backed workspace needs only ".";
// otherwise the existing bounded source-tree walk supplies child candidates.
func GitHistoryRepositories(ctx context.Context, dir string) (GitHistoryRepositoryList, error) {
	ctx, cancel := context.WithTimeout(ctx, GitHistoryTimeout)
	defer cancel()
	result := GitHistoryRepositoryList{Repositories: []GitHistoryRepository{}}
	target, err := resolveGitHistoryTarget(ctx, GitHistoryTarget{Dir: dir, Repo: "."})
	if err == nil {
		result.Repositories = append(result.Repositories, gitHistoryRepository(target))
		return result, nil
	}
	if !errors.Is(err, errGitHistoryNotRepository) {
		return result, err
	}
	paths, err := ListPaths(ctx, target.Workspace)
	if err != nil {
		return result, err
	}
	result.Truncated = paths.Truncated
	for _, rel := range paths.Repositories {
		if len(result.Repositories) == MaxGitHistoryRepositories {
			result.Truncated = true
			break
		}
		child, err := resolveGitHistoryTarget(ctx, GitHistoryTarget{Dir: dir, Repo: rel})
		if err != nil {
			if errors.Is(err, errGitHistoryInvalidRepository) || errors.Is(err, errGitHistoryNotRepository) || errors.Is(err, ErrOutsideRoot) || errors.Is(err, ErrEditProtected) || errors.Is(err, os.ErrNotExist) {
				// A stale/broken candidate must not hide its healthy siblings.
				// Truncated also signals choices omitted during validation.
				result.Truncated = true
				continue
			}
			return result, err
		}
		result.Repositories = append(result.Repositories, gitHistoryRepository(child))
	}
	sort.Slice(result.Repositories, func(i, j int) bool { return result.Repositories[i].Path < result.Repositories[j].Path })
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func gitHistoryRepository(t gitHistoryResolvedTarget) GitHistoryRepository {
	return GitHistoryRepository{Path: t.Repo, Name: filepath.Base(t.Root), Scope: t.Scope}
}

// resolveGitHistoryTarget accepts only literal relative worktree selectors. A
// child must itself be a worktree root, not merely a directory inside one. The
// not-repository sentinel is reserved for an absent marker; broken markers and
// Git failures are errors and must never trigger discovery/fallback.
func resolveGitHistoryTarget(ctx context.Context, target GitHistoryTarget) (gitHistoryResolvedTarget, error) {
	ctx, cancel := context.WithTimeout(ctx, GitHistoryTimeout)
	defer cancel()
	result := gitHistoryResolvedTarget{Target: target}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	rel := target.Repo
	if rel == "" || strings.ContainsAny(rel, "\\\x00:") || !filepath.IsLocal(rel) || editHasParentComponent(rel) {
		return result, ErrOutsideRoot
	}
	for component := range strings.SplitSeq(filepath.ToSlash(rel), "/") {
		if strings.EqualFold(component, ".git") {
			return result, ErrEditProtected
		}
	}
	workspace, err := filepath.Abs(target.Dir)
	if err != nil {
		return result, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return result, err
	}
	info, err := os.Stat(workspace)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("%s is not a directory", target.Dir)
	}
	result.Workspace = workspace
	selected, err := Resolve(workspace, rel)
	if err != nil {
		return result, err
	}
	info, err = os.Stat(selected)
	if err != nil {
		return result, err
	}
	if !info.IsDir() {
		return result, fmt.Errorf("repository target is not a directory")
	}
	result.Repo = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	// Resolving symlinks must not turn a child selector into the containing repo.
	if result.Repo != "." && selected == workspace {
		return result, ErrOutsideRoot
	}
	markerRoot, err := gitHistoryMarkerRoot(ctx, selected)
	if err != nil {
		return result, err
	}
	if result.Repo != "." && markerRoot != selected {
		return result, fmt.Errorf("%w: target is not a worktree root", errGitHistoryInvalidRepository)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return result, err
	}
	w := gitWriter{ctx: ctx, root: selected, git: git, env: gitHistoryEnv()}
	out, err := w.run("", "rev-parse", "--show-toplevel")
	if err != nil {
		var commandErr *gitCommandError
		if errors.As(err, &commandErr) {
			return result, fmt.Errorf("%w: %w", errGitHistoryInvalidRepository, err)
		}
		return result, err
	}
	root, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
	if err != nil {
		return result, err
	}
	if root != markerRoot {
		return result, fmt.Errorf("%w: marker does not identify the selected worktree", errGitHistoryInvalidRepository)
	}
	result.Root = root
	scope, err := filepath.Rel(root, selected)
	if err != nil || !editWithin(root, selected) {
		return result, ErrOutsideRoot
	}
	if scope != "." {
		result.Scope = filepath.ToSlash(scope)
	}
	return result, ctx.Err()
}

// Locate the nearest marker before asking Git. Git may ignore an invalid .git
// directory and discover an outer repository; that must not widen the target.
func gitHistoryMarkerRoot(ctx context.Context, dir string) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		info, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
				return "", fmt.Errorf("%w: invalid marker in %s", errGitHistoryInvalidRepository, dir)
			}
			return dir, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errGitHistoryNotRepository
		}
		dir = parent
	}
}

package files

import (
	"context"
	"path/filepath"
	"strings"
)

// GitChangesRepository is the repository the Changes pane works on: the
// project's own, or one of those inside a project that is in none, as
// GitHistoryRepositories offers them. Paths on the wire stay project-relative,
// so tabs, drafts and diffs need not know which; Root and the paths given to
// Git are the repository's.
type GitChangesRepository struct {
	Root string
	// "repo/" for a repository inside the project, else empty.
	prefix string
}

// ResolveGitChangesRepository authorizes repo inside dir as History does: only
// a literal relative worktree root, resolved again on every call. An empty
// repo or "." is dir itself, Git finding its repository as before.
func ResolveGitChangesRepository(ctx context.Context, dir, repo string) (GitChangesRepository, error) {
	if repo == "" || repo == "." {
		return GitChangesRepository{Root: dir}, nil
	}
	target, err := resolveGitHistoryTarget(ctx, GitHistoryTarget{Dir: dir, Repo: repo})
	if err != nil {
		return GitChangesRepository{}, err
	}
	if target.Repo == "." {
		return GitChangesRepository{Root: dir}, nil
	}
	return GitChangesRepository{Root: target.Root, prefix: target.Repo + "/"}, nil
}

// Paths maps project-relative paths to the repository's, refusing any outside it.
func (r GitChangesRepository) Paths(paths []string) ([]string, error) {
	if r.prefix == "" {
		return paths, nil
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, ok := strings.CutPrefix(filepath.ToSlash(p), r.prefix)
		if !ok || rel == "" {
			return nil, ErrOutsideRoot
		}
		out = append(out, rel)
	}
	return out, nil
}

// Drafts keeps the unsaved drafts inside the repository, as its paths: Git
// there cannot touch the others.
func (r GitChangesRepository) Drafts(paths []string) []string {
	if r.prefix == "" {
		return paths
	}
	var out []string
	for _, p := range paths {
		if rel, ok := strings.CutPrefix(filepath.ToSlash(p), r.prefix); ok && rel != "" {
			out = append(out, rel)
		}
	}
	return out
}

// Path maps a repository path back to the project.
func (r GitChangesRepository) Path(p string) string {
	if r.prefix == "" || p == "" {
		return p
	}
	return r.prefix + p
}

// Status maps a status's entries back to the project.
func (r GitChangesRepository) Status(s RepoStatus) RepoStatus {
	if r.prefix == "" {
		return s
	}
	entries := make([]GitChange, len(s.Entries))
	for i, e := range s.Entries {
		e.Path, e.OriginalPath = r.Path(e.Path), r.Path(e.OriginalPath)
		entries[i] = e
	}
	s.Entries = entries
	return s
}

package codediagnostics

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// Git ignore rules for discovery walks (docs/plan/code-diagnostics-ignore-scope.md).
//
// A walk leaves out the folders the project's Git ignores (build output,
// dependencies), as the Files tree does. A folder with a .git marker of its
// own is a repository, so it is source even when the outer repository
// ignores it: it is walked, with its own Git's rules. Single ignored files
// are not left out. When Git cannot tell (no git, no worktree, the root
// ignored by an outer repository), nothing is skipped and the walk keeps
// only its own name rules.

const (
	maxGitScopeCalls = 16
	gitScopeBudget   = 5 * time.Second
)

// gitScope serves one discovery walk. It is not safe for concurrent use.
type gitScope struct {
	root     string // canonical project root
	ignored  map[string]bool
	asked    map[string]bool
	calls    int
	deadline time.Time
}

// newGitScope asks the Git that answers for the root, once.
func newGitScope(root AuthorizedRoot) *gitScope {
	s := &gitScope{root: root.Project().Root, ignored: map[string]bool{}, asked: map[string]bool{}, deadline: time.Now().Add(gitScopeBudget)}
	s.ask("")
	return s
}

// ask adds the ignored folders of rel's Git, under rel. Calls past the
// budget are not made; the walk then has fewer rules, never more.
func (s *gitScope) ask(rel string) {
	if s.asked[rel] || s.calls >= maxGitScopeCalls || !time.Now().Before(s.deadline) {
		return
	}
	s.asked[rel] = true
	s.calls++
	ctx, cancel := context.WithDeadline(context.Background(), s.deadline)
	defer cancel()
	dirs, ok := files.IgnoredDirectories(ctx, filepath.Join(s.root, filepath.FromSlash(rel)))
	if !ok {
		return
	}
	for _, d := range dirs {
		s.ignored[path.Join(rel, d)] = true
	}
}

// skip reports whether the walk leaves out folder rel (slash-separated,
// relative to the root), one it would otherwise enter. Folders must be asked
// in walk order, a folder before the folders inside it: a nested
// repository's rules are only known once its own folder was asked. Skipping
// is not partial coverage: the folder is out of the project's source on
// purpose.
func (s *gitScope) skip(r *os.Root, rel string) bool {
	if hasGitMarker(r, rel) {
		s.ask(rel)
		return false
	}
	return s.ignored[rel]
}

// hasGitMarker reports whether folder rel has a .git directory or file of
// its own (a repository, worktree or submodule). A symlinked marker is not
// one, as in internal/files.
func hasGitMarker(r *os.Root, rel string) bool {
	info, err := r.Lstat(filepath.Join(filepath.FromSlash(rel), ".git"))
	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// filesFirst orders one folder's entries for discovery: files, then
// folders, each by name. A folder's own markers (go.mod, package.json) are
// then read before the entry budget can be spent in its subfolders.
func filesFirst(a, b os.DirEntry) int {
	if a.IsDir() != b.IsDir() {
		if a.IsDir() {
			return 1
		}
		return -1
	}
	return strings.Compare(a.Name(), b.Name())
}

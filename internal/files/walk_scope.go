package files

import (
	"context"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Git ignore rules for the repositories a walked listing finds
// (docs/plan/nested-repo-gitignore.md). A project root that is in no work
// tree is walked; each repository below it is listed with its own Git's
// ignore rules, as it would be if it were the project. When Git cannot tell,
// that repository keeps the walk's name rules.

const (
	// maxWalkScopes bounds the ignore queries of one walk.
	maxWalkScopes = 64
	// walkScopeBudget bounds their total time; later repositories keep the
	// name rules.
	walkScopeBudget = 5 * time.Second
)

// repositoryIgnores is what one repository's Git ignores: files, and folders
// it ignores whole (never read). Paths are relative to the listing root.
type repositoryIgnores struct {
	files map[string]bool
	dirs  map[string]bool
}

// walkScopeQuery asks the Git of the work tree dir what it ignores. Tests
// replace it.
var walkScopeQuery = repositoryIgnored

// walkScopes serves one walk. It is not safe for concurrent use.
type walkScopes struct {
	ctx      context.Context
	root     string
	calls    int
	deadline time.Time
	// repos maps each repository found, by path relative to the root, to its
	// ignores; nil when Git could not tell.
	repos map[string]*repositoryIgnores
	// The last folder looked up and its answer: a walk asks about one
	// folder's entries in a row.
	lastDir   string
	lastRepo  string
	lastFound bool
}

func newWalkScopes(ctx context.Context, root string) *walkScopes {
	return &walkScopes{ctx: ctx, root: root, deadline: time.Now().Add(walkScopeBudget), repos: map[string]*repositoryIgnores{}}
}

// enter records rel, a folder with a .git marker of its own, as a
// repository and asks its Git, within the budget. Folders must be entered in
// walk order, a repository before the folders inside it.
func (w *walkScopes) enter(rel string) {
	w.lastDir, w.lastRepo, w.lastFound = "", "", false
	if _, ok := w.repos[rel]; ok {
		return
	}
	w.repos[rel] = nil
	if w.calls >= maxWalkScopes || !time.Now().Before(w.deadline) || w.ctx.Err() != nil {
		return
	}
	w.calls++
	ctx, cancel := context.WithDeadline(w.ctx, w.deadline)
	defer cancel()
	files, dirs, ok := walkScopeQuery(ctx, filepath.Join(w.root, filepath.FromSlash(rel)))
	if !ok {
		return
	}
	ig := &repositoryIgnores{files: make(map[string]bool, len(files)), dirs: make(map[string]bool, len(dirs))}
	for _, f := range files {
		ig.files[path.Join(rel, f)] = true
	}
	for _, d := range dirs {
		ig.dirs[path.Join(rel, d)] = true
	}
	w.repos[rel] = ig
}

// of returns the ignores of the innermost repository holding rel (a path
// relative to the root, not a repository itself), or nil when rel is in no
// repository or its Git could not tell.
func (w *walkScopes) of(rel string) *repositoryIgnores {
	dir := path.Dir(rel)
	if dir != w.lastDir || w.lastDir == "" {
		w.lastDir, w.lastRepo, w.lastFound = dir, "", false
		for d := dir; d != "." && d != "/" && d != ""; d = path.Dir(d) {
			if _, ok := w.repos[d]; ok {
				w.lastRepo, w.lastFound = d, true
				break
			}
		}
	}
	if !w.lastFound {
		return nil
	}
	return w.repos[w.lastRepo]
}

// repositoryIgnored lists what the Git of the work tree dir ignores: single
// files, and folders it ignores whole, as `git ls-files --others --ignored
// --directory` reports them. A folder that holds tracked files is never
// reported whole. Paths are slash-separated and relative to dir.
//
// Discovery may not go above dir, so an invalid .git never answers with an
// outer repository's paths. ok is false when Git cannot tell: no git, a
// failure or the context ending, or dir ignored by an outer repository (Git
// then answers "./").
func repositoryIgnored(ctx context.Context, dir string) (files, dirs []string, ok bool) {
	env := append(gitHistoryEnv(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	out, err := gitOutputEnv(ctx, dir, nil, env, []string{"--literal-pathspecs"}, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil || ctx.Err() != nil {
		return nil, nil, false
	}
	return parseIgnored(out)
}

// parseIgnored reads `git ls-files -z --others --ignored --directory`
// output into files and folders (without the trailing slash). ok is false
// for a "./" answer: the whole folder is ignored.
func parseIgnored(out []byte) (files, dirs []string, ok bool) {
	for name := range strings.SplitSeq(string(out), "\x00") {
		if name == "" {
			continue
		}
		rel, isDir := strings.CutSuffix(name, "/")
		if rel == "" || rel == "." {
			return nil, nil, false
		}
		if isDir {
			dirs = append(dirs, rel)
		} else {
			files = append(files, rel)
		}
	}
	return files, dirs, true
}

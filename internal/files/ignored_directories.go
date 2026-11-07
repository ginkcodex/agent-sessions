package files

import (
	"context"
	"slices"
	"strings"
)

// IgnoredDirectories lists the folders below dir that dir's Git ignores, as
// `git ls-files --others --ignored --directory` reports them: each wholly
// ignored folder once, never what is inside it. Paths are slash-separated,
// relative to dir and sorted, without the trailing slash. Ignored single
// files are left out.
//
// ok is false when Git cannot tell: no git, dir in no worktree, a failure or
// the context ending, or dir itself ignored by an outer repository (Git then
// answers "./"). Callers then decide without Git's rules.
//
// It may run before the project is approved, so a repository's own config
// must not start a program: gitOutputEnv turns core.fsmonitor off and
// History's environment keeps Git from writing locks or fetching.
func IgnoredDirectories(ctx context.Context, dir string) ([]string, bool) {
	out, err := gitOutputEnv(ctx, dir, nil, gitHistoryEnv(), []string{"--literal-pathspecs"}, "ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory")
	if err != nil || ctx.Err() != nil {
		return nil, false
	}
	dirs := []string{}
	for name := range strings.SplitSeq(string(out), "\x00") {
		rel, isDir := strings.CutSuffix(name, "/")
		if !isDir {
			continue
		}
		if rel == "" || rel == "." {
			return nil, false
		}
		dirs = append(dirs, rel)
	}
	slices.Sort(dirs)
	return dirs, true
}

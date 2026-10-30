package engine

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/files"
)

type GitDiffBackend interface {
	ReadGitDiff(context.Context, string, string, string, files.GitDiffBasis) (files.GitDiffContent, error)
}

func (e *Engine) ReadGitDiff(ctx context.Context, dir, repo, path string, basis files.GitDiffBasis) (files.GitDiffContent, error) {
	r, paths, err := e.gitChangesPaths(ctx, dir, repo, []string{path})
	if err != nil {
		return files.GitDiffContent{}, err
	}
	out, err := files.ReadGitDiff(ctx, e.editPolicy(), r.Root, paths[0], basis)
	out.Content.Path = r.Path(out.Content.Path)
	return out, err
}

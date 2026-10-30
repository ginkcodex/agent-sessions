package engine

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitWriteBackend is an optional extension, advertised only on writable hosts.
// Each takes the session directory, then the repository chosen in it ("" for
// its own); paths are relative to the directory.
type GitWriteBackend interface {
	GitStage(context.Context, string, string, []string) error
	GitUnstage(context.Context, string, string, []string) error
	GitDiscard(context.Context, string, string, files.GitDiscardRequest) error
	GitCommit(context.Context, string, string, files.GitCommitRequest) error
}

func (e *Engine) GitStage(ctx context.Context, dir, repo string, paths []string) error {
	r, paths, err := e.gitChangesPaths(ctx, dir, repo, paths)
	if err != nil {
		return err
	}
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitStage(ctx, r.Root, paths)
}
func (e *Engine) GitUnstage(ctx context.Context, dir, repo string, paths []string) error {
	r, paths, err := e.gitChangesPaths(ctx, dir, repo, paths)
	if err != nil {
		return err
	}
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitUnstage(ctx, r.Root, paths)
}
func (e *Engine) GitDiscard(ctx context.Context, dir, repo string, req files.GitDiscardRequest) error {
	r, paths, err := e.gitChangesPaths(ctx, dir, repo, req.Paths)
	if err != nil {
		return err
	}
	req.Paths, req.DraftPaths = paths, r.Drafts(req.DraftPaths)
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitDiscard(ctx, e.editPolicy(), r.Root, req)
}
func (e *Engine) GitCommit(ctx context.Context, dir, repo string, req files.GitCommitRequest) error {
	r, err := e.svc.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return err
	}
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitCommit(ctx, r.Root, req)
}

func (e *Engine) gitChangesPaths(ctx context.Context, dir, repo string, paths []string) (files.GitChangesRepository, []string, error) {
	r, err := e.svc.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return r, nil, err
	}
	paths, err = r.Paths(paths)
	return r, paths, err
}

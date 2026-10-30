package engine

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitBranchesBackend is optional so older host backends remain compatible.
type GitBranchesBackend interface {
	GitBranches(context.Context, string, string) (files.GitBranchList, error)
	GitSwitch(context.Context, string, string, files.GitSwitchRequest) error
}

func (e *Engine) GitBranches(ctx context.Context, dir, repo string) (files.GitBranchList, error) {
	r, err := e.svc.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return files.GitBranchList{}, err
	}
	return files.GitBranches(ctx, r.Root)
}
func (e *Engine) GitSwitch(ctx context.Context, dir, repo string, req files.GitSwitchRequest) error {
	r, err := e.svc.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return err
	}
	req.DraftPaths = r.Drafts(req.DraftPaths)
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitSwitch(ctx, r.Root, req)
}

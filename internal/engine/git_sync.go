package engine

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitSyncBackend is optional so older host backends remain compatible.
type GitSyncBackend interface {
	GitSync(context.Context, string, string, files.GitSyncRequest) (files.GitSyncResult, error)
}

func (e *Engine) GitSync(ctx context.Context, dir, repo string, req files.GitSyncRequest) (files.GitSyncResult, error) {
	r, err := e.svc.gitChangesRepository(ctx, dir, repo)
	if err != nil {
		return files.GitSyncResult{Op: req.Op}, err
	}
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	result, err := files.GitSync(ctx, r.Root, req)
	result.Status = r.Status(result.Status)
	return result, err
}

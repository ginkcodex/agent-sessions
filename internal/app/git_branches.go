package app

import (
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

func (a *App) GitBranches(dir, repo string) (files.GitBranchList, error) {
	r := a.route()
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitBranchList{}, offline.err()
	}
	b, ok := r.backend.(engine.GitBranchesBackend)
	if !ok {
		return files.GitBranchList{}, engine.ErrUnsupportedAction
	}
	return b.GitBranches(a.appCtx(), dir, repo)
}

// GitSwitch uses one routing snapshot. The caller must collect live draftPaths
// before calling, then reload clean tabs/tree/status/diagnostics after success.
func (a *App) GitSwitch(dir, repo string, req files.GitSwitchRequest, host string) error {
	r := a.route()
	if r.host != host {
		return ErrSaveHostChanged
	}
	if offline, ok := r.backend.(offlineBackend); ok {
		return offline.err()
	}
	b, ok := r.backend.(engine.GitBranchesBackend)
	if !ok {
		return engine.ErrUnsupportedAction
	}
	return b.GitSwitch(a.appCtx(), dir, repo, req)
}

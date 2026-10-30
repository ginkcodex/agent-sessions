package app

import (
	"errors"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitHistoryRefAction runs on the captured host. CancelGitSync(actionID) cancels it.
func (a *App) GitHistoryRefAction(req files.GitHistoryRefActionRequest, actionID, host string) (files.GitHistoryRefActionResult, error) {
	r := a.route()
	if r.host != host {
		return files.GitHistoryRefActionResult{}, ErrSaveHostChanged
	}
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryRefActionResult{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryActionsBackend)
	if !ok {
		return files.GitHistoryRefActionResult{}, engine.ErrUnsupportedAction
	}
	ctx, done := a.gitSyncs.start(a.appCtx(), actionID)
	defer done()
	out, err := b.GitHistoryRefAction(ctx, req)
	var syncErr *files.GitSyncError
	if err != nil && ctx.Err() != nil && !errors.As(err, &syncErr) {
		err = files.NewGitSyncError(files.GitSyncCanceled, "Git history action was canceled.")
	}
	return out, err
}

func (a *App) GitHistoryCompare(req files.GitHistoryCompareRequest, host string) (files.GitHistoryComparison, error) {
	r := a.route()
	if r.host != host {
		return files.GitHistoryComparison{}, ErrSaveHostChanged
	}
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryComparison{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryCompareBackend)
	if !ok {
		return files.GitHistoryComparison{}, engine.ErrUnsupportedAction
	}
	return b.GitHistoryCompare(a.appCtx(), req)
}

func (a *App) ReadGitHistoryCompareDiff(req files.GitHistoryCompareDiffRequest, host string) (files.GitHistoryCompareDiff, error) {
	r := a.route()
	if r.host != host {
		return files.GitHistoryCompareDiff{}, ErrSaveHostChanged
	}
	if offline, ok := r.backend.(offlineBackend); ok {
		return files.GitHistoryCompareDiff{}, offline.err()
	}
	b, ok := r.backend.(engine.GitHistoryCompareBackend)
	if !ok {
		return files.GitHistoryCompareDiff{}, engine.ErrUnsupportedAction
	}
	return b.ReadGitHistoryCompareDiff(a.appCtx(), req)
}

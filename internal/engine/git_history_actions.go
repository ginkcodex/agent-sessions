package engine

import (
	"context"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/files"
)

// GitHistoryActionsBackend is optional and requires repository write permission.
type GitHistoryActionsBackend interface {
	GitHistoryRefAction(context.Context, files.GitHistoryRefActionRequest) (files.GitHistoryRefActionResult, error)
}

// GitHistoryCompareBackend is independent of repository write permission.
type GitHistoryCompareBackend interface {
	GitHistoryCompare(context.Context, files.GitHistoryCompareRequest) (files.GitHistoryComparison, error)
	ReadGitHistoryCompareDiff(context.Context, files.GitHistoryCompareDiffRequest) (files.GitHistoryCompareDiff, error)
}

func (e *Engine) GitHistoryRefAction(ctx context.Context, req files.GitHistoryRefActionRequest) (files.GitHistoryRefActionResult, error) {
	if !e.svc.knownDirectory(req.Dir) {
		return files.GitHistoryRefActionResult{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	e.svc.projects.invalidate("")
	defer e.svc.projects.invalidate("")
	return files.GitHistoryRefAction(ctx, e.editPolicy(), req)
}

func (e *Engine) GitHistoryCompare(ctx context.Context, req files.GitHistoryCompareRequest) (files.GitHistoryComparison, error) {
	if !e.svc.knownDirectory(req.Dir) {
		return files.GitHistoryComparison{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	return files.GitHistoryCompare(ctx, req)
}

func (e *Engine) ReadGitHistoryCompareDiff(ctx context.Context, req files.GitHistoryCompareDiffRequest) (files.GitHistoryCompareDiff, error) {
	if !e.svc.knownDirectory(req.Dir) {
		return files.GitHistoryCompareDiff{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, req.Dir)
	}
	return files.ReadGitHistoryCompareDiff(ctx, req)
}

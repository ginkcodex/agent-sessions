package engine

import (
	"context"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/manage"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type CodexRepairResult = manage.CodexRepairResult

// CodexRepairBackend is optional: older remote hosts never fall back locally.
type CodexRepairBackend interface {
	RepairCodexSession(context.Context, model.SessionRef) (CodexRepairResult, error)
}

func (e *Engine) RepairCodexSession(ctx context.Context, ref model.SessionRef) (CodexRepairResult, error) {
	if e.svc == nil || e.svc.Catalog() == nil {
		return CodexRepairResult{}, ErrUnknownSession
	}
	if err := e.svc.ValidateRefs([]model.SessionRef{ref}); err != nil {
		return CodexRepairResult{}, err
	}
	mgr, err := e.ensureManage()
	if err != nil {
		return CodexRepairResult{}, err
	}
	result, err := mgr.RepairCodexSession(ctx, ref, e.svc.Catalog().All())
	if err != nil {
		return result, err
	}
	if err := e.Scan(ctx); err != nil {
		return result, fmt.Errorf("Session repaired, but refresh failed. Refresh the session list: %w", err)
	}
	return result, nil
}

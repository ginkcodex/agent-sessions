package app

import (
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type CodexRepairResult = engine.CodexRepairResult

func (a *App) RepairCodexSession(ref model.SessionRef) (CodexRepairResult, error) {
	backend := a.activeBackend()
	repairer, ok := backend.(engine.CodexRepairBackend)
	if !ok {
		return CodexRepairResult{}, engine.ErrUnsupportedAction
	}
	return repairer.RepairCodexSession(a.appCtx(), ref)
}

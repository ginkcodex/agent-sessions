package rpc

import (
	"context"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type RepairCodexParams struct {
	Ref model.SessionRef `json:"ref"`
}

func (c *Client) RepairCodexSession(ctx context.Context, ref model.SessionRef) (engine.CodexRepairResult, error) {
	if !c.Capabilities().RepairCodexSession {
		return engine.CodexRepairResult{}, engine.ErrUnsupportedAction
	}
	var out engine.CodexRepairResult
	err := c.call(ctx, "repairCodexSession", RepairCodexParams{Ref: ref}, &out)
	return out, err
}

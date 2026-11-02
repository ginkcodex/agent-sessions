package rpc

import (
	"context"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

type RenameSessionParams struct {
	Ref   model.SessionRef `json:"ref"`
	Title string           `json:"title"`
}

func (c *Client) RenameSession(ctx context.Context, ref model.SessionRef, title string) error {
	if !c.Capabilities().RenameSession {
		return engine.ErrUnsupportedAction
	}
	return c.call(ctx, "renameSession", RenameSessionParams{Ref: ref, Title: title}, nil)
}

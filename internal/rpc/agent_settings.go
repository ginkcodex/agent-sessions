package rpc

import (
	"context"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

type SetAgentSettingsParams struct {
	Agents []string `json:"agents"`
}

func (c *Client) GetAgentSettings(ctx context.Context) (engine.AgentSettings, error) {
	if !c.Capabilities().AgentSettings {
		return engine.AgentSettings{}, engine.ErrUnsupportedAction
	}
	var out engine.AgentSettings
	err := c.call(ctx, "getAgentSettings", nil, &out)
	return out, err
}

func (c *Client) SetAgentSettings(ctx context.Context, agents []string) (engine.AgentSettings, error) {
	if !c.Capabilities().AgentSettings {
		return engine.AgentSettings{}, engine.ErrUnsupportedAction
	}
	var out engine.AgentSettings
	err := c.call(ctx, "setAgentSettings", SetAgentSettingsParams{Agents: agents}, &out)
	return out, err
}

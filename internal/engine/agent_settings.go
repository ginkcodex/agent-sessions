package engine

import (
	"context"
	"slices"

	"github.com/ginkcode/agent-sessions/internal/provider/all"
)

// AgentSettingsBackend is optional so older/unsupported hosts remain explicit.
type AgentSettingsBackend interface {
	GetAgentSettings(context.Context) (AgentSettings, error)
	SetAgentSettings(context.Context, []string) (AgentSettings, error)
}

type AgentSettings struct {
	Supported       []all.Agent `json:"supported"`
	Enabled         []string    `json:"enabled"`
	Active          []string    `json:"active"`
	RestartRequired bool        `json:"restartRequired"`
}

func (e *Engine) agentSettings(ids []string) AgentSettings {
	active := all.EffectiveIDs(e.svc.Providers())
	// Expose known providers in registry order, with injected providers afterwards.
	ordered := make([]string, 0, len(active))
	for _, id := range all.IDs() {
		if slices.Contains(active, id) {
			ordered = append(ordered, id)
		}
	}
	for _, id := range active {
		if !slices.Contains(ordered, id) {
			ordered = append(ordered, id)
		}
	}
	return AgentSettings{Supported: all.Supported(), Enabled: ids, Active: ordered, RestartRequired: !slices.Equal(ids, ordered)}
}

func (e *Engine) GetAgentSettings(ctx context.Context) (AgentSettings, error) {
	if err := ctx.Err(); err != nil {
		return AgentSettings{}, err
	}
	ids, err := all.LoadEnabled(e.roots.Config)
	if err != nil {
		return AgentSettings{}, err
	}
	return e.agentSettings(ids), nil
}

// SetAgentSettings persists only: scanners, watchers and launch providers stay
// fixed until the host engine is reconstructed (restart or reconnect).
func (e *Engine) SetAgentSettings(ctx context.Context, ids []string) (AgentSettings, error) {
	if err := ctx.Err(); err != nil {
		return AgentSettings{}, err
	}
	ids, err := all.SaveEnabled(e.roots.Config, ids)
	if err != nil {
		return AgentSettings{}, err
	}
	return e.agentSettings(ids), nil
}

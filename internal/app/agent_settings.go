package app

import "github.com/ginkcode/agent-sessions/internal/engine"

func (a *App) GetAgentSettings() (engine.AgentSettings, error) {
	b, ok := a.activeBackend().(engine.AgentSettingsBackend)
	if !ok {
		return engine.AgentSettings{}, engine.ErrUnsupportedAction
	}
	return b.GetAgentSettings(a.appCtx())
}

func (a *App) SetAgentSettings(agents []string) (engine.AgentSettings, error) {
	b, ok := a.activeBackend().(engine.AgentSettingsBackend)
	if !ok {
		return engine.AgentSettings{}, engine.ErrUnsupportedAction
	}
	return b.SetAgentSettings(a.appCtx(), agents)
}

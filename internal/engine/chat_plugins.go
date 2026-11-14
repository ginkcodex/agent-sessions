package engine

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ginkcode/agent-sessions/internal/acp"
	"github.com/ginkcode/agent-sessions/internal/codexbridge"
	"github.com/ginkcode/agent-sessions/internal/manage"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// ChatPlugins is a chat's answer to /plugin: the plugins the agent's CLI
// reports installed on disk. A running session may not have loaded all of
// them yet; its command catalog says what it did load.
type ChatPlugins struct {
	// Supported is false when the agent's CLI cannot list its plugins in a
	// form the app reads (OpenCode).
	Supported bool              `json:"supported"`
	Plugins   []provider.Plugin `json:"plugins"`
}

// runPluginCLI runs the listing; tests replace it.
var runPluginCLI = manage.CaptureCLI

// Plugins lists the plugins installed for the chat's agent, with the CLI
// the chat's adapter drives, in the chat's directory and environment. It
// stops when ctx ends or the chat closes.
func (m *ChatManager) Plugins(ctx context.Context, chatID string) (ChatPlugins, error) {
	ch, err := m.get(chatID)
	if err != nil {
		return ChatPlugins{}, err
	}
	if _, _, err := ch.live(); err != nil {
		return ChatPlugins{}, err
	}
	if m.opts.Plugins == nil {
		return ChatPlugins{Plugins: []provider.Plugin{}}, nil
	}
	ch.mu.Lock()
	launch := ch.launch
	ch.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ch.ctx, cancel)
	defer stop()
	out, err := m.opts.Plugins(ctx, ch.agent, launch)
	if err != nil && ch.ctx.Err() != nil {
		return ChatPlugins{}, ErrChatClosed
	}
	return out, err
}

// ChatPlugins lists agent's installed plugins. launch is the chat's adapter
// command: its directory and environment, and the CLI it was pointed at.
func (s *Service) ChatPlugins(ctx context.Context, agent model.AgentID, launch acp.Command) (ChatPlugins, error) {
	prov, ok := s.providers.Get(agent)
	if !ok {
		return ChatPlugins{}, fmt.Errorf("%w: %s", ErrUnknownProvider, agent)
	}
	lister, ok := prov.(provider.PluginLister)
	if !ok {
		return ChatPlugins{Plugins: []provider.Plugin{}}, nil
	}
	spec := lister.PluginList()
	cli := pluginCLI(prov, launch)
	if cli == "" {
		return ChatPlugins{}, fmt.Errorf("%s has no CLI to list its plugins", prov.DisplayName())
	}
	ctx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	stdout, err := runPluginCLI(ctx, append([]string{cli}, spec.Args...), launch.Dir, launch.Env)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ChatPlugins{}, fmt.Errorf("listing %s plugins took longer than %s", prov.DisplayName(), spec.Timeout)
	}
	if err != nil {
		return ChatPlugins{}, fmt.Errorf("could not list %s plugins: %w", prov.DisplayName(), err)
	}
	plugins, err := spec.Parse(stdout)
	if err != nil {
		// An older CLI without --json, or a format that changed: never an
		// empty list.
		return ChatPlugins{}, fmt.Errorf("this %s version does not list its plugins in a form the app reads; update it", prov.DisplayName())
	}
	return ChatPlugins{Supported: true, Plugins: plugins}, nil
}

// pluginCLI is the CLI the chat's adapter was pointed at, else the one new
// sessions start.
func pluginCLI(prov provider.Provider, launch acp.Command) string {
	l, ok := prov.(provider.ACPLauncher)
	if !ok {
		return ""
	}
	a := l.ACPAdapter()
	env := append(os.Environ(), launch.Env...)
	if a.DaemonBridge {
		// The adapter's CLI variable names this program; the bridge's names codex.
		if cli := envLookup(env, codexbridge.EnvCodex); cli != "" {
			return cli
		}
	}
	if a.CLIEnv != "" {
		if cli := envLookup(env, a.CLIEnv); cli != "" {
			return cli
		}
	}
	if argv := prov.NewSessionCommand(launch.Dir).Argv; len(argv) > 0 {
		return argv[0]
	}
	return ""
}

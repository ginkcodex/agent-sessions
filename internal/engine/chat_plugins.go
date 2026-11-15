package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

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

// ChatInventory is a chat's answer to an agent's MCP or skills listing
// (OpenCode's /mcps and /skills), read from the agent's CLI rather than
// asked of the model. Only the list of its Kind is set.
type ChatInventory struct {
	Kind string `json:"kind"`
	// Supported is false when the agent's CLI cannot list this kind.
	Supported  bool                 `json:"supported"`
	MCPServers []provider.MCPServer `json:"mcpServers,omitempty"`
	Skills     []provider.Skill     `json:"skills,omitempty"`
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

// Inventory lists the chat's agent's MCP servers or skills, as Plugins
// lists its plugins.
func (m *ChatManager) Inventory(ctx context.Context, chatID, kind string) (ChatInventory, error) {
	ch, err := m.get(chatID)
	if err != nil {
		return ChatInventory{}, err
	}
	if _, _, err := ch.live(); err != nil {
		return ChatInventory{}, err
	}
	if m.opts.Inventory == nil {
		return emptyInventory(kind, false), nil
	}
	ch.mu.Lock()
	launch := ch.launch
	ch.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ch.ctx, cancel)
	defer stop()
	out, err := m.opts.Inventory(ctx, ch.agent, launch, kind)
	if err != nil && ch.ctx.Err() != nil {
		return ChatInventory{}, ErrChatClosed
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
	spec := lister.PluginList(launch.Dir)
	stdout, err := runListing(ctx, prov, launch, "plugins", spec.Args, spec.Timeout)
	if err != nil {
		return ChatPlugins{}, err
	}
	plugins, err := spec.Parse(stdout)
	if err != nil {
		// An older CLI without --json, or a format that changed: never an
		// empty list.
		return ChatPlugins{}, fmt.Errorf("this %s version does not list its plugins in a form the app reads; update it", prov.DisplayName())
	}
	return ChatPlugins{Supported: true, Plugins: plugins}, nil
}

// ChatInventory lists agent's MCP servers or skills (kind is
// provider.InventoryMCP or provider.InventorySkills) the way ChatPlugins
// lists its plugins.
func (s *Service) ChatInventory(ctx context.Context, agent model.AgentID, launch acp.Command, kind string) (ChatInventory, error) {
	if kind != provider.InventoryMCP && kind != provider.InventorySkills {
		return ChatInventory{}, fmt.Errorf("unknown inventory %q", kind)
	}
	prov, ok := s.providers.Get(agent)
	if !ok {
		return ChatInventory{}, fmt.Errorf("%w: %s", ErrUnknownProvider, agent)
	}
	lister, ok := prov.(provider.InventoryLister)
	if !ok {
		return emptyInventory(kind, false), nil
	}
	spec, ok := lister.InventoryList(kind, launch.Dir)
	if !ok {
		return emptyInventory(kind, false), nil
	}
	what := inventoryNoun(kind)
	stdout, err := runListing(ctx, prov, launch, what, spec.Args, spec.Timeout)
	if err != nil {
		return ChatInventory{}, err
	}
	inv, err := spec.Parse(stdout)
	if err != nil {
		return ChatInventory{}, fmt.Errorf("this %s version does not list its %s in a form the app reads; update it", prov.DisplayName(), what)
	}
	out := emptyInventory(kind, true)
	if kind == provider.InventoryMCP && inv.MCPServers != nil {
		out.MCPServers = inv.MCPServers
	}
	if kind == provider.InventorySkills && inv.Skills != nil {
		out.Skills = inv.Skills
	}
	return out, nil
}

func inventoryNoun(kind string) string {
	if kind == provider.InventoryMCP {
		return "MCP servers"
	}
	return kind
}

// emptyInventory has the kind's list set, empty, so it reads as [] not null.
func emptyInventory(kind string, supported bool) ChatInventory {
	out := ChatInventory{Kind: kind, Supported: supported}
	if kind == provider.InventoryMCP {
		out.MCPServers = []provider.MCPServer{}
	} else {
		out.Skills = []provider.Skill{}
	}
	return out
}

// runListing runs the agent's CLI with args in the chat's directory and
// environment, within timeout. what names the listing in errors.
func runListing(ctx context.Context, prov provider.Provider, launch acp.Command, what string, args []string, timeout time.Duration) ([]byte, error) {
	cli := pluginCLI(prov, launch)
	if cli == "" {
		return nil, fmt.Errorf("%s has no CLI to list its %s", prov.DisplayName(), what)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout, err := runPluginCLI(ctx, append([]string{cli}, args...), launch.Dir, launch.Env)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("listing %s %s took longer than %s", prov.DisplayName(), what, timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("could not list %s %s: %w", prov.DisplayName(), what, err)
	}
	return stdout, nil
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

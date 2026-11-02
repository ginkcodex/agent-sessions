// Package all is the registry of supported coding agents.
package all

import (
	"fmt"
	"sort"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/claude"
	"github.com/ginkcode/agent-sessions/internal/provider/codex"
	"github.com/ginkcode/agent-sessions/internal/provider/opencode"
)

type Agent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type registration struct {
	Agent
	build func(paths.Roots, *pathutil.GitResolver) provider.Provider
}

var registry = []registration{
	{Agent{string(model.AgentClaude), "Claude Code"}, func(r paths.Roots, g *pathutil.GitResolver) provider.Provider { return claude.New(r.Claude, g) }},
	{Agent{"codex", "Codex"}, func(r paths.Roots, g *pathutil.GitResolver) provider.Provider { return codex.New(r.Codex, g) }},
	{Agent{"opencode", "OpenCode"}, func(r paths.Roots, g *pathutil.GitResolver) provider.Provider { return opencode.New(r.OpenCodeData, g) }},
}

// Supported is independent of installed or enabled providers.
func Supported() []Agent {
	out := make([]Agent, 0, len(registry))
	for _, r := range registry {
		out = append(out, r.Agent)
	}
	return out
}

func IDs() []string {
	out := make([]string, 0, len(registry))
	for _, r := range registry {
		out = append(out, r.ID)
	}
	return out
}

// Canonical validates a nonempty selection and deduplicates it in registry order.
func Canonical(ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		seen[id] = true
	}
	out := make([]string, 0, len(seen))
	for _, r := range registry {
		if seen[r.ID] {
			out = append(out, r.ID)
			delete(seen, r.ID)
		}
	}
	if len(seen) > 0 {
		return nil, fmt.Errorf("agents: unsupported agent selection")
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agents: at least one agent must be enabled")
	}
	return out, nil
}

// Providers constructs every supported provider, regardless of persisted settings.
func Providers(roots paths.Roots, git *pathutil.GitResolver) provider.Set {
	return selectedProviders(roots, git, IDs())
}

func selectedProviders(roots paths.Roots, git *pathutil.GitResolver, ids []string) provider.Set {
	if git == nil {
		git = pathutil.NewGitResolver()
	}
	out := make(provider.Set, 0, len(ids))
	for _, r := range registry {
		for _, id := range ids {
			if r.ID == id {
				out = append(out, r.build(roots, git))
				break
			}
		}
	}
	return out
}

// EnabledProviders snapshots the host's persisted selection at construction.
func EnabledProviders(roots paths.Roots, git *pathutil.GitResolver) (provider.Set, error) {
	ids, err := LoadEnabled(roots.Config)
	if err != nil {
		return nil, err
	}
	return selectedProviders(roots, git, ids), nil
}

// EffectiveIDs returns a stable set identity, including injected providers.
func EffectiveIDs(provs provider.Set) []string {
	seen := map[model.AgentID]bool{}
	ids := make([]string, 0, len(provs))
	for _, p := range provs {
		if !seen[p.ID()] {
			seen[p.ID()] = true
			ids = append(ids, string(p.ID()))
		}
	}
	sort.Strings(ids)
	return ids
}

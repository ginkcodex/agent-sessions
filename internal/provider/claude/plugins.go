package claude

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

var _ provider.PluginLister = (*Provider)(nil)

// PluginList runs `claude plugin list --json`, which reads the install
// state on disk and takes under a second.
func (p *Provider) PluginList() provider.PluginList {
	return provider.PluginList{Args: []string{"plugin", "list", "--json"}, Timeout: 30 * time.Second, Parse: parsePlugins}
}

// parsePlugins reads the JSON array `claude plugin list --json` prints.
// installPath, installedAt, lastUpdated and hasUserConfig are dropped.
func parsePlugins(stdout []byte) ([]provider.Plugin, error) {
	var raw []struct {
		ID             string `json:"id"`
		Version        string `json:"version"`
		Scope          string `json:"scope"`
		Enabled        bool   `json:"enabled"`
		ProjectEnabled *bool  `json:"projectEnabled"`
	}
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return nil, err
	}
	out := make([]provider.Plugin, 0, len(raw))
	for _, r := range raw {
		name, market, _ := strings.Cut(r.ID, "@")
		if name == "" {
			return nil, errors.New("a plugin has no id")
		}
		out = append(out, provider.Plugin{ID: r.ID, Name: name, Marketplace: market, Version: r.Version, Scope: r.Scope, Enabled: r.Enabled, ProjectEnabled: r.ProjectEnabled})
	}
	return out, nil
}

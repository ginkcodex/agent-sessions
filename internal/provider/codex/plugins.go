package codex

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

var _ provider.PluginLister = (*Provider)(nil)

// PluginList runs `codex plugin list --json`. It queries the remote
// marketplaces and took 68 s in testing, so it may take two minutes.
func (p *Provider) PluginList() provider.PluginList {
	return provider.PluginList{Args: []string{"plugin", "list", "--json"}, Timeout: 2 * time.Minute, Parse: parsePlugins}
}

// parsePlugins reads the installed list of `codex plugin list --json`.
// source (a connector ID), installPolicy and authPolicy are dropped.
func parsePlugins(stdout []byte) ([]provider.Plugin, error) {
	var raw struct {
		Installed *[]struct {
			PluginID        string `json:"pluginId"`
			Name            string `json:"name"`
			MarketplaceName string `json:"marketplaceName"`
			Version         string `json:"version"`
			Enabled         bool   `json:"enabled"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return nil, err
	}
	if raw.Installed == nil {
		return nil, errors.New("no installed list")
	}
	out := make([]provider.Plugin, 0, len(*raw.Installed))
	for _, r := range *raw.Installed {
		if r.Name == "" {
			return nil, errors.New("a plugin has no name")
		}
		out = append(out, provider.Plugin{ID: r.PluginID, Name: r.Name, Marketplace: r.MarketplaceName, Version: r.Version, Enabled: r.Enabled})
	}
	return out, nil
}

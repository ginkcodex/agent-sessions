package opencode

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

var (
	_ provider.PluginLister    = (*Provider)(nil)
	_ provider.InventoryLister = (*Provider)(nil)
)

// inventoryTimeout bounds one `opencode api` read. It answers in well under
// a second from the background service; starting that service takes longer.
const inventoryTimeout = 30 * time.Second

// apiArgs is `opencode api <operation>` for the chat's directory. The
// operations are read-only GETs of the server's OpenAPI surface (OpenCode
// 2.3): they list, and never connect, add or change anything.
func apiArgs(operation, dir string) []string {
	args := []string{"api", operation}
	if dir != "" {
		args = append(args, "--param", "location[directory]="+dir)
	}
	return args
}

// PluginList runs `opencode api plugin.list`, OpenCode's /plugins.
func (p *Provider) PluginList(dir string) provider.PluginList {
	return provider.PluginList{Args: apiArgs("plugin.list", dir), Timeout: inventoryTimeout, Parse: parsePlugins}
}

// InventoryList runs `opencode api mcp.list` (OpenCode's /mcps) or
// `opencode api skill.list` (its /skills).
func (p *Provider) InventoryList(kind, dir string) (provider.InventoryList, bool) {
	switch kind {
	case provider.InventoryMCP:
		return provider.InventoryList{Args: apiArgs("mcp.list", dir), Timeout: inventoryTimeout, Parse: parseMCPServers}, true
	case provider.InventorySkills:
		return provider.InventoryList{Args: apiArgs("skill.list", dir), Timeout: inventoryTimeout, Parse: parseSkills}, true
	}
	return provider.InventoryList{}, false
}

// apiData reads the {"location": …, "data": [...]} envelope every list
// operation answers with. A missing data array is an unknown format.
func apiData[T any](stdout []byte) ([]T, error) {
	var env struct {
		Data *[]T `json:"data"`
	}
	if err := json.Unmarshal(stdout, &env); err != nil {
		return nil, err
	}
	if env.Data == nil {
		return nil, errors.New("no data list")
	}
	return *env.Data, nil
}

// parsePlugins keeps the installed plugins: packages, local files and SDK
// ones. OpenCode's own builtins are part of OpenCode, not installed. A
// local plugin's path is reduced to its file name.
func parsePlugins(stdout []byte) ([]provider.Plugin, error) {
	raw, err := apiData[struct {
		ID     string `json:"id"`
		Source struct {
			Type    string `json:"type"`
			Target  string `json:"target"`
			Version string `json:"version"`
			Path    string `json:"path"`
		} `json:"source"`
		State struct {
			Status string `json:"status"`
		} `json:"state"`
	}](stdout)
	if err != nil {
		return nil, err
	}
	out := []provider.Plugin{}
	for i, r := range raw {
		pl := provider.Plugin{ID: r.ID, Enabled: r.State.Status == "active"}
		switch r.Source.Type {
		case "builtin":
			continue
		case "package":
			pl.Name, pl.Version, pl.Scope = r.Source.Target, r.Source.Version, "package"
		case "local":
			pl.Name, pl.Scope = filepath.Base(filepath.FromSlash(r.Source.Path)), "local"
		default:
			pl.Name, pl.Scope = r.ID, r.Source.Type
		}
		if pl.Name == "" || pl.Name == "." {
			pl.Name = r.ID
		}
		if pl.Name == "" {
			// The schema makes id optional: an SDK plugin may have none.
			pl.Name = "(unnamed)"
		}
		if pl.ID == "" {
			// The list keys rows by ID, so an unnamed one gets its position.
			pl.ID = "#" + strconv.Itoa(i)
		}
		out = append(out, pl)
	}
	return out, nil
}

// parseMCPServers keeps each server's name, title and status.
func parseMCPServers(stdout []byte) (provider.Inventory, error) {
	raw, err := apiData[struct {
		Name   string `json:"name"`
		Title  string `json:"title"`
		Status struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"status"`
	}](stdout)
	if err != nil {
		return provider.Inventory{}, err
	}
	out := make([]provider.MCPServer, 0, len(raw))
	for _, r := range raw {
		if r.Name == "" || r.Status.Status == "" {
			return provider.Inventory{}, errors.New("a server has no name or status")
		}
		out = append(out, provider.MCPServer{Name: r.Name, Title: r.Title, Status: r.Status.Status, Error: r.Status.Error})
	}
	return provider.Inventory{MCPServers: out}, nil
}

// parseSkills keeps each skill's ID, name and description; its path and
// content are dropped.
func parseSkills(stdout []byte) (provider.Inventory, error) {
	raw, err := apiData[struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}](stdout)
	if err != nil {
		return provider.Inventory{}, err
	}
	out := make([]provider.Skill, 0, len(raw))
	for _, r := range raw {
		if r.ID == "" {
			return provider.Inventory{}, errors.New("a skill has no id")
		}
		if r.Name == "" {
			r.Name = r.ID
		}
		out = append(out, provider.Skill{ID: r.ID, Name: r.Name, Description: r.Description})
	}
	return provider.Inventory{Skills: out}, nil
}

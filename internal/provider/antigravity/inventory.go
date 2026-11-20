package antigravity

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

var _ provider.InventoryLister = (*Provider)(nil)

// inventoryTimeout bounds one `agy mcp list` or `agy -p /skills`. Both only
// read agy's config: neither connects to a server or runs a model turn.
const inventoryTimeout = 30 * time.Second

// noMCPServers is what `agy mcp list` prints when none are configured.
const noMCPServers = "No MCP servers configured."

// InventoryList runs `agy mcp list`, the TUI's /mcp, or the TUI's /skills
// in print mode: agy has no skills subcommand, and answers /skills itself
// (docs/tasks/antigravity-cli/AGY-21.md). `agy mcp` also adds, removes,
// enables and disables servers; the app runs only list.
func (p *Provider) InventoryList(kind, _ string) (provider.InventoryList, bool) {
	switch kind {
	case provider.InventoryMCP:
		return provider.InventoryList{Args: []string{"mcp", "list"}, Timeout: inventoryTimeout, Parse: parseMCPList}, true
	case provider.InventorySkills:
		// Plan mode: an agy that sent /skills to the model could not edit.
		return provider.InventoryList{Args: []string{"-p", "/skills", "--output-format", "json", "--mode", "plan"}, Timeout: inventoryTimeout, Parse: parseSkills}, true
	}
	return provider.InventoryList{}, false
}

// parseSkills reads the command block of `agy -p /skills --output-format
// json` (agy 1.5.12), keeping each skill's name and description; the path is
// dropped. A reply with a conversation, or without the block, was not agy's
// own /skills: an error, never an empty list.
func parseSkills(stdout []byte) (provider.Inventory, error) {
	var reply struct {
		ConversationID string `json:"conversation_id"`
		Status         string `json:"status"`
		Command        *struct {
			Name string `json:"name"`
			Data struct {
				Skills []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"skills"`
			} `json:"data"`
		} `json:"command"`
	}
	if err := json.Unmarshal(stdout, &reply); err != nil {
		return provider.Inventory{}, err
	}
	if reply.ConversationID != "" || reply.Status != "SUCCESS" || reply.Command == nil || reply.Command.Name != "skills" {
		return provider.Inventory{}, errors.New("not agy's own /skills listing")
	}
	out := make([]provider.Skill, 0, len(reply.Command.Data.Skills))
	for _, s := range reply.Command.Data.Skills {
		if s.Name == "" {
			return provider.Inventory{}, errors.New("a skill has no name")
		}
		out = append(out, provider.Skill{ID: s.Name, Name: s.Name, Description: s.Description})
	}
	return provider.Inventory{Skills: out}, nil
}

// parseMCPList reads `agy mcp list`'s table (agy 1.5.12):
//
//	NAME       TYPE   STATUS    COMMAND/URL
//	my server  stdio  enabled   echo hi
//
// The columns are space-aligned and a name may hold spaces, so each row is
// cut at the header's column starts. Status is enabled or disabled: agy
// connects to no server to list them. The command or URL is dropped, as it
// may carry a token.
func parseMCPList(stdout []byte) (provider.Inventory, error) {
	text := strings.TrimSpace(strings.ReplaceAll(string(stdout), "\r\n", "\n"))
	if text == noMCPServers {
		return provider.Inventory{MCPServers: []provider.MCPServer{}}, nil
	}
	lines := strings.Split(text, "\n")
	header := lines[0]
	if strings.Join(strings.Fields(header), " ") != "NAME TYPE STATUS COMMAND/URL" {
		return provider.Inventory{}, errors.New("not agy's MCP server table")
	}
	typeAt, statusAt, commandAt := strings.Index(header, "TYPE"), strings.Index(header, "STATUS"), strings.Index(header, "COMMAND/URL")
	out := make([]provider.MCPServer, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if len(line) <= commandAt || line[typeAt-1] != ' ' || line[statusAt-1] != ' ' || line[commandAt-1] != ' ' {
			return provider.Inventory{}, errors.New("an MCP server row does not fit the table's columns")
		}
		name := strings.TrimSpace(line[:typeAt])
		status := strings.TrimSpace(line[statusAt:commandAt])
		if name == "" || (status != "enabled" && status != "disabled") {
			return provider.Inventory{}, errors.New("an MCP server row has no name or a status agy does not print")
		}
		out = append(out, provider.MCPServer{Name: name, Status: status})
	}
	return provider.Inventory{MCPServers: out}, nil
}

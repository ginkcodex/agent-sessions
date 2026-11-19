package antigravity

import (
	"errors"
	"strings"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

var _ provider.InventoryLister = (*Provider)(nil)

// inventoryTimeout bounds one `agy mcp list`. It only reads agy's config and
// connects to no server.
const inventoryTimeout = 30 * time.Second

// noMCPServers is what `agy mcp list` prints when none are configured.
const noMCPServers = "No MCP servers configured."

// InventoryList runs `agy mcp list`, the TUI's /mcp. agy lists no skills.
// `agy mcp` also adds, removes, enables and disables servers; the app runs
// only list.
func (p *Provider) InventoryList(kind, _ string) (provider.InventoryList, bool) {
	if kind != provider.InventoryMCP {
		return provider.InventoryList{}, false
	}
	return provider.InventoryList{Args: []string{"mcp", "list"}, Timeout: inventoryTimeout, Parse: parseMCPList}, true
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

package antigravity

import (
	"reflect"
	"slices"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestInventoryList(t *testing.T) {
	p := New(t.TempDir(), nil)
	mcp, ok := p.InventoryList(provider.InventoryMCP, "/work/a")
	if !ok || !slices.Equal(mcp.Args, []string{"mcp", "list"}) || mcp.Timeout <= 0 {
		t.Fatalf("mcp = %+v, %v", mcp, ok)
	}
	if _, ok := p.InventoryList(provider.InventorySkills, "/work/a"); ok {
		t.Fatal("skills are listed")
	}
}

func TestParseMCPList(t *testing.T) {
	// What `agy mcp list` printed on agy 1.5.12 (Linux).
	none, err := parseMCPList([]byte("No MCP servers configured.\n"))
	if err != nil || none.MCPServers == nil || len(none.MCPServers) != 0 {
		t.Fatalf("none = %+v, %v", none, err)
	}
	table := "NAME       TYPE   STATUS    COMMAND/URL\n" +
		"api        http   disabled  https://example.com/mcp?token=secret\n" +
		"fs         stdio  enabled   npx -y @modelcontextprotocol/server-filesystem /work\n" +
		"my server  stdio  enabled   echo hi\n"
	got, err := parseMCPList([]byte(table))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.MCPServer{
		{Name: "api", Status: "disabled"},
		{Name: "fs", Status: "enabled"},
		{Name: "my server", Status: "enabled"},
	}
	if !reflect.DeepEqual(got.MCPServers, want) {
		t.Fatalf("servers = %+v", got.MCPServers)
	}
	for _, bad := range []string{
		"",
		"Error: no config",
		"NAME TYPE STATUS\nfs stdio enabled",
		"NAME  TYPE   STATUS    COMMAND/URL\nfs    stdio  failing   npx\n",
		"NAME  TYPE   STATUS    COMMAND/URL\nfs stdio\n",
		"NAME  TYPE   STATUS    COMMAND/URL\n      stdio  enabled   npx\n",
	} {
		if inv, err := parseMCPList([]byte(bad)); err == nil {
			t.Errorf("parsed %q as %+v", bad, inv)
		}
	}
}

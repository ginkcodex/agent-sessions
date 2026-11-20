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
	skills, ok := p.InventoryList(provider.InventorySkills, "/work/a")
	if !ok || !slices.Equal(skills.Args, []string{"-p", "/skills", "--output-format", "json", "--mode", "plan"}) || skills.Timeout <= 0 {
		t.Fatalf("skills = %+v, %v", skills, ok)
	}
}

func TestParseSkills(t *testing.T) {
	// What `agy -p /skills --output-format json` printed on agy 1.5.12
	// (Linux), trimmed: agy answers it itself, with no turn.
	reply := `{"conversation_id":"","status":"SUCCESS","response":"agy-customizations\tGuide.\ndemo-skill\tLine one.\nLine two.",` +
		`"duration_seconds":0,"num_turns":0,"usage":{"input_tokens":0,"output_tokens":0},` +
		`"command":{"name":"skills","data":{"skills":[` +
		`{"name":"agy-customizations","description":"Guide.","path":"/home/u/.gemini/antigravity-cli/builtin/skills/agy-customizations/SKILL.md","builtin":true,"model_invocable":true},` +
		`{"name":"demo-skill","description":"Line one.\nLine two.","path":"/work/.agents/skills/demo-skill/SKILL.md","builtin":false,"model_invocable":true}]}}}` + "\n"
	got, err := parseSkills([]byte(reply))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Skill{
		{ID: "agy-customizations", Name: "agy-customizations", Description: "Guide."},
		{ID: "demo-skill", Name: "demo-skill", Description: "Line one.\nLine two."},
	}
	if !reflect.DeepEqual(got.Skills, want) {
		t.Fatalf("skills = %+v", got.Skills)
	}
	none, err := parseSkills([]byte(`{"conversation_id":"","status":"SUCCESS","command":{"name":"skills","data":{"skills":null}}}`))
	if err != nil || none.Skills == nil || len(none.Skills) != 0 {
		t.Fatalf("none = %+v, %v", none, err)
	}
	for _, bad := range []string{
		"",
		"agy-customizations\tGuide.",
		// An agy that sent /skills to the model.
		`{"conversation_id":"c2b87113","status":"SUCCESS","response":"Here are my skills"}`,
		`{"conversation_id":"c2b87113","status":"SUCCESS","command":{"name":"skills","data":{"skills":[]}}}`,
		`{"conversation_id":"","status":"ERROR","command":{"name":"skills","data":{"skills":[]}}}`,
		`{"conversation_id":"","status":"SUCCESS","command":{"name":"mcp","data":{"skills":[]}}}`,
		`{"conversation_id":"","status":"SUCCESS","command":{"name":"skills","data":{"skills":[{"description":"x"}]}}}`,
	} {
		if inv, err := parseSkills([]byte(bad)); err == nil {
			t.Errorf("parsed %q as %+v", bad, inv)
		}
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

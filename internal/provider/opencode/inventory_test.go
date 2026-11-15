package opencode

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestInventoryArgs(t *testing.T) {
	p := New(t.TempDir(), nil)
	if got := p.PluginList("/work/a").Args; !slices.Equal(got, []string{"api", "plugin.list", "--param", "location[directory]=/work/a"}) {
		t.Fatalf("plugin args = %q", got)
	}
	mcp, ok := p.InventoryList(provider.InventoryMCP, "/work/a")
	if !ok || !slices.Equal(mcp.Args, []string{"api", "mcp.list", "--param", "location[directory]=/work/a"}) || mcp.Timeout <= 0 {
		t.Fatalf("mcp = %+v, %v", mcp, ok)
	}
	skills, ok := p.InventoryList(provider.InventorySkills, "")
	if !ok || !slices.Equal(skills.Args, []string{"api", "skill.list"}) {
		t.Fatalf("skills = %+v, %v", skills, ok)
	}
	if _, ok := p.InventoryList("nope", "/w"); ok {
		t.Fatal("an unknown kind is listed")
	}
}

func TestParsePlugins(t *testing.T) {
	// The shape `opencode api plugin.list` printed on OpenCode 2.3.0.
	out := `{"location":{"directory":"/w"},"data":[
		{"id":"opencode.mcp","source":{"type":"builtin"},"features":{"server":true},"state":{"status":"active"}},
		{"id":"notifier","source":{"type":"package","target":"opencode-notifier","version":"1.2.0"},"features":{"server":true},"state":{"status":"active"}},
		{"id":"mine","source":{"type":"local","path":"/home/u/.config/opencode/plugins/mine.ts"},"features":{"tui":true},"state":{"status":"failed","error":"boom","ref":"x"}},
		{"source":{"type":"sdk"},"features":{},"state":{"status":"active"}}
	]}`
	got, err := parsePlugins([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Plugin{
		{ID: "notifier", Name: "opencode-notifier", Version: "1.2.0", Scope: "package", Enabled: true},
		{ID: "mine", Name: "mine.ts", Scope: "local", Enabled: false},
	}
	if len(got) != 3 || !reflect.DeepEqual(got[:2], want) || got[2] != (provider.Plugin{ID: "#3", Name: "(unnamed)", Scope: "sdk", Enabled: true}) {
		t.Fatalf("plugins = %+v", got)
	}
	if strings.Contains(got[1].Name, "/home") {
		t.Fatal("a local plugin's path is shown")
	}
	if empty, err := parsePlugins([]byte(`{"location":{},"data":[{"id":"b","source":{"type":"builtin"},"state":{"status":"active"}}]}`)); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("builtins only = %+v, %v", empty, err)
	}
	for _, bad := range []string{"", "No plugins found", `<!doctype html>`, `{"location":{}}`, `{"data":{}}`} {
		if _, err := parsePlugins([]byte(bad)); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestParseMCPServers(t *testing.T) {
	out := `{"location":{"directory":"/w"},"data":[
		{"name":"super-fetch","status":{"status":"connected"},"integrationID":"mcp_1"},
		{"name":"docs","title":"Docs","status":{"status":"failed","error":"connect ECONNREFUSED"}},
		{"name":"gh","status":{"status":"needs_auth","error":"sign in"}},
		{"name":"old","status":{"status":"disabled"}}
	]}`
	got, err := parseMCPServers([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.MCPServer{
		{Name: "super-fetch", Status: "connected"},
		{Name: "docs", Title: "Docs", Status: "failed", Error: "connect ECONNREFUSED"},
		{Name: "gh", Status: "needs_auth", Error: "sign in"},
		{Name: "old", Status: "disabled"},
	}
	if !reflect.DeepEqual(got.MCPServers, want) {
		t.Fatalf("servers = %+v", got.MCPServers)
	}
	if empty, err := parseMCPServers([]byte(`{"location":{},"data":[]}`)); err != nil || empty.MCPServers == nil || len(empty.MCPServers) != 0 {
		t.Fatalf("none = %+v, %v", empty, err)
	}
	for _, bad := range []string{"No MCP servers configured", `{"location":{}}`, `{"data":[{"name":"a"}]}`, `{"data":[{"status":{"status":"connected"}}]}`} {
		if _, err := parseMCPServers([]byte(bad)); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestParseSkills(t *testing.T) {
	out := `{"location":{"directory":"/w"},"data":[
		{"id":"report","name":"Report","description":"Report an issue","path":"skill://report/SKILL.md","content":"# secret body"},
		{"id":"local","name":"","path":"/home/u/p/.claude/skills/local/SKILL.md","content":"x"}
	]}`
	got, err := parseSkills([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Skill{{ID: "report", Name: "Report", Description: "Report an issue"}, {ID: "local", Name: "local"}}
	if !reflect.DeepEqual(got.Skills, want) {
		t.Fatalf("skills = %+v", got.Skills)
	}
	for _, bad := range []string{"", `{"location":{}}`, `{"data":[{"name":"x"}]}`} {
		if _, err := parseSkills([]byte(bad)); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
}

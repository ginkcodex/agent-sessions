package claude

import (
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestParsePlugins(t *testing.T) {
	t.Parallel()
	yes, no := true, false
	got, err := parsePlugins([]byte(`[
		{"id":"ponytail@ponytail","version":"5.3.3","scope":"user","enabled":true,"projectEnabled":false,"installPath":"/home/u/.claude/plugins/cache/ponytail","installedAt":"2026-01-01","hasUserConfig":true},
		{"id":"gopls-lsp@claude-plugins-official","version":"1.0.0","scope":"project","enabled":false},
		{"id":"local","enabled":true,"projectEnabled":true}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Plugin{
		{ID: "ponytail@ponytail", Name: "ponytail", Marketplace: "ponytail", Version: "5.3.3", Scope: "user", Enabled: true, ProjectEnabled: &no},
		{ID: "gopls-lsp@claude-plugins-official", Name: "gopls-lsp", Marketplace: "claude-plugins-official", Version: "1.0.0", Scope: "project"},
		{ID: "local", Name: "local", Enabled: true, ProjectEnabled: &yes},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, err := parsePlugins([]byte(`[]`)); err != nil || len(got) != 0 {
		t.Fatalf("empty list = %v, %v", got, err)
	}
	for _, bad := range []string{``, `Unknown option --json`, `{"installed":[]}`, `[{"version":"1"}]`, `[{"id":"@market"}]`} {
		if _, err := parsePlugins([]byte(bad)); err == nil {
			t.Errorf("parsePlugins(%q) succeeded", bad)
		}
	}
}

package codex

import (
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestParsePlugins(t *testing.T) {
	t.Parallel()
	got, err := parsePlugins([]byte(`{"installed":[
		{"pluginId":"sites@openai-curated-remote","name":"sites","marketplaceName":"openai-curated-remote","version":"1.1.6","installed":true,"enabled":true,"source":{"connectorId":"secret"},"installPolicy":"x","authPolicy":"y"},
		{"pluginId":"visualize@m","name":"visualize","marketplaceName":"m","version":"1.0.89","installed":true,"enabled":false}
	],"available":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Plugin{
		{ID: "sites@openai-curated-remote", Name: "sites", Marketplace: "openai-curated-remote", Version: "1.1.6", Enabled: true},
		{ID: "visualize@m", Name: "visualize", Marketplace: "m", Version: "1.0.89"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, err := parsePlugins([]byte(`{"installed":[]}`)); err != nil || len(got) != 0 {
		t.Fatalf("empty list = %v, %v", got, err)
	}
	for _, bad := range []string{``, `error: unexpected argument '--json'`, `[]`, `{}`, `{"installed":[{"pluginId":"x"}]}`} {
		if _, err := parsePlugins([]byte(bad)); err == nil {
			t.Errorf("parsePlugins(%q) succeeded", bad)
		}
	}
}

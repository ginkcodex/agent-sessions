package all

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/manage"
)

func TestAgentConfigValidationAndPreservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := "# header\n[manage]\nenabled = true\nfuture = 'keep'\n\n[agents] # selection\n# keep comment\nenabled = [\n 'codex',\n 'opencode',\n]\nfuture = 42\n\n[remote]\nhost = 'box'\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]string{nil, {}, {"unknown"}, {"codex", "unknown"}} {
		if _, err := SaveEnabled(dir, ids); err == nil {
			t.Fatalf("accepted %v", ids)
		}
		data, _ := os.ReadFile(path)
		if string(data) != original {
			t.Fatal("invalid selection changed file")
		}
	}
	want := []string{"claude-code", "codex"}
	got, err := SaveEnabled(dir, []string{"codex", "claude-code", "codex"})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("save=%v %v", got, err)
	}
	data, _ := os.ReadFile(path)
	for _, keep := range []string{"# header", "[agents] # selection", "# keep comment", "future = 'keep'", "future = 42", "[remote]\nhost = 'box'"} {
		if !strings.Contains(string(data), keep) {
			t.Fatalf("lost %q in %s", keep, data)
		}
	}
	// The preexisting manage writer must preserve the independent agents table.
	if err := manage.NewConfigStore(path).Save(manage.Config{AllowPermanentDelete: true}); err != nil {
		t.Fatal(err)
	}
	got, err = LoadEnabled(dir)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("load=%v %v", got, err)
	}
}

func TestAgentConfigDefaultsAndMalformed(t *testing.T) {
	dir := t.TempDir()
	got, err := LoadEnabled(dir)
	if err != nil || !reflect.DeepEqual(got, IDs()) {
		t.Fatalf("defaults=%v %v", got, err)
	}
	for _, text := range []string{"[agents]\nenabled = []\n", "[agents]\nenabled = ['future']\n", "[agents]\nenabled = true\n", "[agents]\nenabled = ["} {
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadEnabled(dir); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}

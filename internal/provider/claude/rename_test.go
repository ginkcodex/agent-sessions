package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestCustomTitleWinsAndSurvivesCheckpoint(t *testing.T) {
	var cp checkpoint
	var d provider.Diagnostics
	for _, line := range []string{`{"type":"summary","summary":"summary"}`, `{"type":"custom-title","customTitle":"first"}`, `{"type":"ai-title","aiTitle":"generated"}`, `{"type":"custom-title","customTitle":"native"}`, `{"type":"custom-title","customTitle":""}`} {
		cp.consume([]byte(line), &d)
	}
	if got := cp.finalize(time.Time{}).Title; got != "native" {
		t.Fatal(got)
	}
	encoded, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	var resumed checkpoint
	if err := json.Unmarshal(encoded, &resumed); err != nil {
		t.Fatal(err)
	}
	resumed.consume([]byte(`{"type":"ai-title","aiTitle":"later generated"}`), &d)
	if got := resumed.finalize(time.Time{}).Title; got != "native" {
		t.Fatal(got)
	}
}

func TestCustomTitleIncrementalAndOldCheckpoint(t *testing.T) {
	root, path := fixtureRoot(t, "basic")
	p := New(root, nil)
	first, err := p.Scan(context.Background(), provider.ScanState{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("{\"type\":\"custom-title\",\"customTitle\":\"native renamed\"}\n{\"type\":\"ai-title\",\"aiTitle\":\"later AI\"}\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	second, err := p.Scan(context.Background(), first.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Changed) != 1 || second.Changed[0].Title != "native renamed" {
		t.Fatalf("changed=%+v", second.Changed)
	}
	// Simulate a pre-custom-title checkpoint that already consumed the record.
	state := second.State
	source := state.Sources[path]
	var raw map[string]any
	if err := json.Unmarshal(source.Checkpoint, &raw); err != nil {
		t.Fatal(err)
	}
	raw["version"] = float64(5)
	delete(raw, "customTitle")
	source.Checkpoint, _ = json.Marshal(raw)
	state.Sources[path] = source
	third, err := New(root, nil).Scan(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Changed) != 1 || third.Changed[0].Title != "native renamed" {
		t.Fatalf("old checkpoint replay failed: %+v (%s)", third.Changed, filepath.Base(path))
	}
}

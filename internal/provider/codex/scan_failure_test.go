package codex

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestScanUnreadableDayRetainsState(t *testing.T) {
	root := t.TempDir()
	path := writeBasic(t, root)
	p := New(root, nil)
	first, err := p.Scan(t.Context(), provider.ScanState{})
	if err != nil {
		t.Fatal(err)
	}
	day := filepath.Dir(path)
	if err := os.Chmod(day, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(day, 0o700) })
	if _, err := os.ReadDir(day); err == nil {
		t.Skip("permissions not enforced")
	}
	writeRollout(t, root, "healthy", line("2026-09-28T12:00:00Z", "session_meta", `{"id":"healthy","cwd":"/tmp"}`), true)
	partial, err := p.Scan(t.Context(), first.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Changed) != 1 || partial.Changed[0].Ref.ID != "healthy" || len(partial.Removed) != 0 {
		t.Fatalf("partial scan = %+v", partial)
	}
	if len(partial.Diag.Warnings) == 0 || !reflect.DeepEqual(partial.State.Sources[path], first.State.Sources[path]) {
		t.Fatal("missing warning/retained state")
	}
	if err := os.Chmod(day, 0o700); err != nil {
		t.Fatal(err)
	}
	recovered, err := p.Scan(t.Context(), partial.State)
	if err != nil || len(recovered.Changed) != 0 || len(recovered.Removed) != 0 {
		t.Fatalf("recovery = %+v, %v", recovered, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	removed, err := p.Scan(t.Context(), recovered.State)
	if err != nil || len(removed.Removed) != 1 {
		t.Fatalf("deletion = %+v, %v", removed, err)
	}
}

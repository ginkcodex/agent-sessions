package claude

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestScanUnreadableSourceRetainsState(t *testing.T) {
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "project"
		}
		t.Run(name, func(t *testing.T) {
			root, path := fixtureRoot(t, "basic")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			p := New(root, nil)
			first, err := p.Scan(t.Context(), provider.ScanState{})
			if err != nil {
				t.Fatal(err)
			}
			// Force an incremental read rather than the unchanged-source fast path.
			if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			blocked := path
			if directory {
				blocked = filepath.Dir(path)
			}
			if err := os.Chmod(blocked, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(blocked, 0700) })
			if directory {
				if _, err := os.ReadDir(blocked); err == nil {
					t.Skip("permissions not enforced")
				}
			} else if f, err := os.Open(blocked); err == nil {
				_ = f.Close()
				t.Skip("permissions not enforced")
			}
			healthy := filepath.Join(root, "projects", "healthy", "healthy.jsonl")
			if err := os.MkdirAll(filepath.Dir(healthy), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(healthy, data, 0600); err != nil {
				t.Fatal(err)
			}
			partial, err := p.Scan(t.Context(), first.State)
			if err != nil {
				t.Fatal(err)
			}
			if len(partial.Removed) != 0 || len(partial.Changed) != 1 || partial.Changed[0].Ref.ID != "healthy" {
				t.Fatalf("partial scan = %+v", partial)
			}
			if !reflect.DeepEqual(partial.State.Sources[path], first.State.Sources[path]) || len(partial.Diag.Warnings) == 0 {
				t.Fatal("missing retained state/warning")
			}
			if err := os.Chmod(blocked, 0700); err != nil {
				t.Fatal(err)
			}
			recovered, err := p.Scan(t.Context(), partial.State)
			if err != nil || len(recovered.Removed) != 0 || len(recovered.Diag.Warnings) != 0 {
				t.Fatalf("recovery = %+v, %v", recovered, err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			removed, err := p.Scan(t.Context(), recovered.State)
			if err != nil || len(removed.Removed) != 1 {
				t.Fatalf("real deletion = %+v, %v", removed, err)
			}
		})
	}
}

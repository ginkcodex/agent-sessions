package codex

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/provider"
)

// TestScanCheckpointReuseMatchesFullRecode runs the mutation matrix through a
// provider that reuses unchanged checkpoints and one that decodes and
// re-encodes all of them, as before PF-10A. Results, diagnostics and state
// bytes must be identical, and the reusing scan decodes each previous source
// once and encodes only the checkpoints whose bytes change.
func TestScanCheckpointReuseMatchesFullRecode(t *testing.T) {
	root, steps := mutationMatrix(t)
	// A rollout with no ID in its records or name is scanned but never canonical.
	idless := filepath.Join(root, "sessions", "2026", "09", "28", "rollout-.jsonl")
	if err := os.WriteFile(idless, []byte(line("2026-09-28T12:00:00Z", "response_item", `{"type":"message","role":"user","content":"no id"}`)), 0o600); err != nil {
		t.Fatal(err)
	}
	reuse, full := New(root, nil), New(root, nil)
	reuse.noCheckpointCache = true // PF-10A's per-scan work contract
	full.noCheckpointReuse = true
	ctx := context.Background()
	var reusePrev, fullPrev provider.ScanState
	scan := func(name string, reads bool) {
		t.Helper()
		var rw, fw provider.ScanWork
		got, err := reuse.Scan(provider.WithScanWork(ctx, &rw), reusePrev)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, err := full.Scan(provider.WithScanWork(ctx, &fw), fullPrev)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !reflect.DeepEqual(got, want) {
			for path, src := range want.State.Sources {
				if !bytes.Equal(got.State.Sources[path].Checkpoint, src.Checkpoint) {
					t.Errorf("%s: %s checkpoint\nreuse: %s\nfull:  %s", name, path, got.State.Sources[path].Checkpoint, src.Checkpoint)
				}
			}
			t.Fatalf("%s: scan results differ\nreuse: %+v %+v\nfull:  %+v %+v", name, got.Changed, got.Removed, want.Changed, want.Removed)
		}
		if rw.CheckpointDecodes.Load() > fw.CheckpointDecodes.Load() || rw.CheckpointEncodes.Load() > fw.CheckpointEncodes.Load() {
			t.Fatalf("%s: reuse did more work: decodes %d/%d encodes %d/%d", name,
				rw.CheckpointDecodes.Load(), fw.CheckpointDecodes.Load(), rw.CheckpointEncodes.Load(), fw.CheckpointEncodes.Load())
		}
		if !reads && reusePrev.Sources != nil {
			var rewritten int64
			for path, src := range got.State.Sources {
				if !bytes.Equal(src.Checkpoint, reusePrev.Sources[path].Checkpoint) {
					rewritten++
				}
			}
			if rw.CheckpointDecodes.Load() != int64(len(reusePrev.Sources)) || rw.CheckpointEncodes.Load() != rewritten {
				t.Fatalf("%s: decodes=%d for %d previous sources, encodes=%d for %d rewritten checkpoints", name,
					rw.CheckpointDecodes.Load(), len(reusePrev.Sources), rw.CheckpointEncodes.Load(), rewritten)
			}
		}
		reusePrev, fullPrev = got.State, want.State
	}
	for _, step := range steps {
		step.mutate()
		scan(step.name, step.reads)
	}
	scan("unchanged after matrix", false)
}

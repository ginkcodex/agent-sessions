package codex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// coldCanonical is the metadata a fresh provider would resolve.
func coldCanonical(t *testing.T, root string) map[model.SessionRef]model.SessionMeta {
	t.Helper()
	res, err := New(root, nil).Scan(context.Background(), provider.ScanState{})
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[model.SessionRef]model.SessionMeta, len(res.Changed))
	for _, meta := range res.Changed {
		out[meta.Ref] = meta
	}
	return out
}

// touch gives a rewritten file a distinct mtime even on coarse filesystems.
func touch(t *testing.T, path string, at time.Time) {
	t.Helper()
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

type mutationStep struct {
	name   string
	mutate func()
	reads  bool // whether an incremental scan may read rollout records
}

// mutationMatrix builds a root with duplicates, an index and edges, and the
// sequence of rollout and index changes that incremental scans must follow.
func mutationMatrix(t *testing.T) (string, []mutationStep) {
	t.Helper()
	// Like the catalog, the scan checkpoint keeps the cwd normalized at parse
	// time. A resolved root keeps that symlink resolution out of this matrix.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "workdir")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	active := indexRollout(t, root, indexID, false)
	archived := indexRollout(t, root, indexID, true)
	child := indexRollout(t, root, childID, false)
	orphan := writeRollout(t, root, orphanID, line("2026-09-28T12:00:00Z", "session_meta", `{"id":"`+orphanID+`","cwd":"`+cwd+`"}`)+
		line("2026-09-28T12:00:01Z", "response_item", `{"type":"message","role":"user","content":"orphan"}`), false)
	db := stateDB(t, root, "state_5.sqlite", stateSQL)
	insertThread(t, db, indexID, archived, "Archived index", 1)
	insertThread(t, db, childID, child, "Child", 0)
	if _, err := db.Exec(`INSERT INTO thread_spawn_edges VALUES (?,?)`, indexID, childID); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-time.Hour)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	return root, []mutationStep{
		{"initial", func() {}, true},
		{"unchanged", func() {}, false},
		{"index overlay", func() { exec(`UPDATE threads SET title='Renamed' WHERE id=?`, childID) }, false},
		{"invalid index row", func() { exec(`UPDATE threads SET rollout_path='/outside/rollout.jsonl' WHERE id=?`, childID) }, false},
		{"edge change", func() {
			exec(`DELETE FROM thread_spawn_edges`)
			exec(`INSERT INTO thread_spawn_edges VALUES (?,?)`, childID, orphanID)
		}, false},
		{"wal update", func() {
			exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0`)
			exec(`UPDATE threads SET title='WAL title' WHERE id=?`, indexID)
			if info, err := os.Stat(filepath.Join(root, "state_5.sqlite-wal")); err != nil || info.Size() == 0 {
				t.Fatalf("no WAL: %v", err)
			}
		}, false},
		{"index archived flag", func() { exec(`UPDATE threads SET archived=0 WHERE id=?`, indexID) }, false},
		{"index row deleted", func() { exec(`DELETE FROM threads WHERE id=?`, childID) }, false},
		{"higher state db", func() {
			next := stateDB(t, root, "state_7.sqlite", stateSQL)
			insertThread(t, next, indexID, active, "Numeric seven", 0)
		}, false},
		{"cwd removed", func() {
			if err := os.Remove(cwd); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"append", func() {
			f, err := os.OpenFile(orphan, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.WriteString(line("2026-09-28T12:00:02Z", "response_item", `{"type":"message","role":"user","content":"appended"}`))
			if closeErr := f.Close(); err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}, true},
		{"added newer duplicate", func() {
			raw := string(mustRead(t, child)) + line("2099-01-01T00:00:00Z", "event_msg", `{"type":"task_complete"}`)
			writeRollout(t, root, childID, raw, true)
		}, true},
		{"replaced", func() {
			raw := line("2026-09-28T12:00:00Z", "session_meta", `{"id":"`+childID+`","cwd":"/tmp/replaced"}`) +
				line("2026-09-28T13:00:00Z", "response_item", `{"type":"message","role":"user","content":"replaced prompt"}`)
			if err := os.WriteFile(child, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			touch(t, child, stamp)
		}, true},
		{"truncated", func() {
			data := mustRead(t, active)
			cut := strings.Index(string(data), "\n") + 1
			if err := os.WriteFile(active, data[:cut], 0o600); err != nil {
				t.Fatal(err)
			}
			touch(t, active, stamp)
		}, true},
		{"indexed source removed", func() {
			if err := os.Remove(archived); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"removed", func() {
			if err := os.Remove(orphan); err != nil {
				t.Fatal(err)
			}
		}, false},
	}
}

func TestResolverMatchesColdScan(t *testing.T) {
	root, steps := mutationMatrix(t)
	p := New(root, nil)
	ctx := context.Background()
	for _, step := range steps {
		step.mutate()
		var work provider.ScanWork
		if _, err := p.rolloutFor(provider.WithScanWork(ctx, &work), model.SessionRef{Agent: model.AgentCodex, ID: indexID}); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		want := coldCanonical(t, root)
		if !reflect.DeepEqual(p.resolver.canonical, want) {
			t.Fatalf("%s: warm resolution differs from cold scan\nwarm: %+v\ncold: %+v", step.name, p.resolver.canonical, want)
		}
		if !step.reads && (work.SourceOpens.Load() != 0 || work.RecordBytes.Load() != 0) {
			t.Fatalf("%s: unchanged rollouts reread: opens=%d bytes=%d", step.name, work.SourceOpens.Load(), work.RecordBytes.Load())
		}
	}
	if meta := p.resolver.canonical[model.SessionRef{Agent: model.AgentCodex, ID: indexID}]; meta.Archived || meta.Title != "Numeric seven" {
		t.Fatalf("matrix did not reach its final selection: %+v", meta)
	}
}

func mustReadFile(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestResolverWarmWork(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{indexID, childID, orphanID, missingID, outsideID} {
		indexRollout(t, root, id, false)
	}
	target := indexRollout(t, root, "66666666-6666-6666-6666-666666666666", false)
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "66666666-6666-6666-6666-666666666666"}
	p := New(root, nil)
	ctx := context.Background()
	var cold provider.ScanWork
	if _, err := p.Load(provider.WithScanWork(ctx, &cold), ref); err != nil {
		t.Fatal(err)
	}
	if cold.SourceOpens.Load() != 6 {
		t.Fatalf("cold resolution opened %d rollouts", cold.SourceOpens.Load())
	}
	var unchanged provider.ScanWork
	if _, err := p.Load(provider.WithScanWork(ctx, &unchanged), ref); err != nil {
		t.Fatal(err)
	}
	if unchanged.SourceOpens.Load() != 0 || unchanged.RecordBytes.Load() != 0 {
		t.Fatalf("unchanged resolution read opens=%d bytes=%d", unchanged.SourceOpens.Load(), unchanged.RecordBytes.Load())
	}
	before := mustReadFile(t, target)
	appended := line("2026-09-28T12:00:05Z", "response_item", `{"type":"message","role":"user","content":"more"}`)
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(appended)
	if closeErr := f.Close(); err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	var warm provider.ScanWork
	tr, err := p.Load(provider.WithScanWork(ctx, &warm), ref)
	if err != nil {
		t.Fatal(err)
	}
	// Only the appended record, without its newline, is read during resolution.
	if warm.SourceOpens.Load() != 1 || warm.Records.Load() != 1 || warm.RecordBytes.Load() != mustReadFile(t, target)-before-1 {
		t.Fatalf("append resolution opens=%d records=%d bytes=%d", warm.SourceOpens.Load(), warm.Records.Load(), warm.RecordBytes.Load())
	}
	want, err := New(root, nil).Load(ctx, ref)
	if err != nil || !reflect.DeepEqual(tr, want) {
		t.Fatalf("warm load differs from cold: %v", err)
	}
}

func TestResolverCapAndFallback(t *testing.T) {
	root := t.TempDir()
	path := indexRollout(t, root, indexID, false)
	indexRollout(t, root, childID, false)
	ref := model.SessionRef{Agent: model.AgentCodex, ID: indexID}
	ctx := context.Background()
	p := New(root, nil)
	old := resolverCap
	resolverCap = 0
	t.Cleanup(func() { resolverCap = old })
	for range 2 {
		var work provider.ScanWork
		if _, err := p.Load(provider.WithScanWork(ctx, &work), ref); err != nil {
			t.Fatal(err)
		}
		if p.resolver.canonical != nil || p.resolver.state.Sources != nil || work.SourceOpens.Load() != 2 {
			t.Fatalf("over cap: retained=%v opens=%d", p.resolver.canonical != nil, work.SourceOpens.Load())
		}
	}
	resolverCap = old
	if _, err := p.Load(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if p.resolver.retained <= 0 {
		t.Fatal("retained resolution not accounted")
	}

	// A previously scanned rollout replaced by a directory fails the warm scan,
	// which cannot prove removal. Resolution then matches a cold scan.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, nil).Scan(ctx, p.resolver.state); err == nil {
		t.Fatal("fixture did not fail a warm scan")
	}
	if _, err := p.Load(ctx, ref); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing session after fallback: %v", err)
	}
	if !reflect.DeepEqual(p.resolver.canonical, coldCanonical(t, root)) {
		t.Fatal("fallback differs from cold scan")
	}
	if _, err := p.Load(ctx, model.SessionRef{Agent: model.AgentCodex, ID: childID}); err != nil {
		t.Fatal(err)
	}
}

func TestResolverCancellation(t *testing.T) {
	root := t.TempDir()
	indexRollout(t, root, indexID, false)
	ref := model.SessionRef{Agent: model.AgentCodex, ID: indexID}
	p := New(root, nil)
	ctx := context.Background()
	if _, err := p.Load(ctx, ref); err != nil {
		t.Fatal(err)
	}
	state, canonical := p.resolver.state, p.resolver.canonical

	// A caller waiting behind another resolution can give up.
	p.resolver.lock <- struct{}{}
	waiting, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	_, _, err := p.resolver.resolve(waiting, p, ref)
	cancel()
	<-p.resolver.lock
	if err != context.DeadlineExceeded {
		t.Fatalf("waiting caller: %v", err)
	}

	// A cancelled scan keeps the previous state.
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.resolver.refresh(canceled, p); err != context.Canceled {
		t.Fatalf("cancelled scan: %v", err)
	}
	if !reflect.DeepEqual(p.resolver.state, state) || !reflect.DeepEqual(p.resolver.canonical, canonical) {
		t.Fatal("cancelled scan changed retained resolution")
	}
	if _, err := p.Load(canceled, ref); err != context.Canceled {
		t.Fatalf("cancelled load: %v", err)
	}
	var work provider.ScanWork
	if _, err := p.Load(provider.WithScanWork(ctx, &work), ref); err != nil || work.SourceOpens.Load() != 0 {
		t.Fatalf("after cancellation: opens=%d err=%v", work.SourceOpens.Load(), err)
	}
}

func TestResolverCoalesces(t *testing.T) {
	root := t.TempDir()
	path := indexRollout(t, root, indexID, false)
	ref := model.SessionRef{Agent: model.AgentCodex, ID: indexID}
	p := New(root, nil)
	ctx := context.Background()
	r := p.resolver
	// A caller arriving before scan 1 began reuses it; one arriving after
	// scans again, and so sees a write made before it arrived.
	if _, _, err := r.resolveAfter(ctx, p, ref, 0); err != nil {
		t.Fatal(err)
	}
	var reused provider.ScanWork
	if _, ok, err := r.resolveAfter(provider.WithScanWork(ctx, &reused), p, ref, 0); err != nil || !ok || reused.CheckpointDecodes.Load() != 0 {
		t.Fatalf("earlier arrival rescanned: ok=%v decodes=%d err=%v", ok, reused.CheckpointDecodes.Load(), err)
	}
	if err := os.WriteFile(path, []byte(line("2026-09-28T12:00:00Z", "session_meta", `{"id":"`+indexID+`"}`)), 0o600); err != nil {
		t.Fatal(err)
	}
	touch(t, path, time.Now().Add(-time.Hour))
	meta, _, err := r.resolveAfter(ctx, p, ref, r.started.Load())
	if err != nil {
		t.Fatal(err)
	}
	if meta != coldCanonical(t, root)[ref] {
		t.Fatal("later arrival reused an older scan")
	}

	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := p.rolloutFor(ctx, ref)
			if err != nil || got != meta {
				t.Errorf("concurrent resolution: %v", err)
			}
		}()
	}
	wg.Wait()
}

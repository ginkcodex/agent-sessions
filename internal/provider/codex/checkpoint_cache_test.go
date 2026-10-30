package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestCheckpointCacheMutationMatrix(t *testing.T) {
	root, steps := mutationMatrix(t)
	cached, baseline := New(root, nil), New(root, nil)
	baseline.noCheckpointCache = true
	var prev, old provider.ScanState
	for _, step := range steps {
		step.mutate()
		var work provider.ScanWork
		got, err := cached.Scan(provider.WithScanWork(context.Background(), &work), prev)
		if err != nil {
			t.Fatal(step.name, err)
		}
		want, err := baseline.Scan(context.Background(), old)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: different results: %v", step.name, err)
		}
		if !step.reads && work.CheckpointDecodes.Load() != 0 {
			t.Fatalf("%s: %d decodes", step.name, work.CheckpointDecodes.Load())
		}
		prev, old = got.State, want.State
	}
}

func TestCheckpointCacheIdentityAndLifecycle(t *testing.T) {
	p := New(t.TempDir(), nil)
	root := p.root
	path := indexRollout(t, root, indexID, false)
	ctx := context.Background()
	first, err := p.Scan(ctx, provider.ScanState{})
	if err != nil {
		t.Fatal(err)
	}
	if p.checkpoints.retained <= 0 {
		t.Fatal("no retained accounting")
	}
	t.Logf("retained estimate per source: %d bytes", p.checkpoints.retained)
	raw := first.State.Sources[path].Checkpoint
	if p.checkpoints.lookup(path, raw) == nil {
		t.Fatal("missing entry")
	}
	copyRaw := append([]byte(nil), raw...)
	copyRaw[0] = ' '
	if p.checkpoints.lookup(path, copyRaw) != nil {
		t.Fatal("different bytes hit")
	}
	var fresh provider.ScanWork
	if _, err := New(root, nil).Scan(provider.WithScanWork(ctx, &fresh), first.State); err != nil || fresh.CheckpointDecodes.Load() == 0 {
		t.Fatalf("new provider: %v", err)
	}
	var reset provider.ScanWork
	if _, err := p.Scan(provider.WithScanWork(ctx, &reset), provider.ScanState{}); err != nil || reset.SourceOpens.Load() != 1 {
		t.Fatalf("reset: %v", err)
	}
	var cp checkpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatal(err)
	}
	cp.Version++
	bad, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	state := first.State.Sources[path]
	state.Checkpoint = bad
	var version provider.ScanWork
	if _, err := p.Scan(provider.WithScanWork(ctx, &version), provider.ScanState{Sources: map[string]provider.SourceState{path: state}}); err != nil || version.SourceOpens.Load() != 1 {
		t.Fatalf("version: %v", err)
	}
	before := p.checkpoints.entries
	beforeCP := before[path].cp
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := p.Scan(canceled, first.State); err != context.Canceled {
		t.Fatal(err)
	}
	if fmt.Sprintf("%p", before) != fmt.Sprintf("%p", p.checkpoints.entries) || p.checkpoints.entries[path].cp != beforeCP {
		t.Fatal("canceled scan published")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Scan(ctx, first.State); err == nil {
		t.Fatal("expected failed scan")
	}
	if fmt.Sprintf("%p", before) != fmt.Sprintf("%p", p.checkpoints.entries) || p.checkpoints.entries[path].cp != beforeCP {
		t.Fatal("failed scan published")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	indexRollout(t, root, indexID, false)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.Scan(ctx, first.State); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(raw, first.State.Sources[path].Checkpoint) {
		t.Fatal("previous bytes mutated")
	}
}

func TestCheckpointCacheOwnsCanonical(t *testing.T) {
	cp := &checkpoint{Version: checkpointVersion, Canonical: &model.SessionMeta{Title: "invalid\xff title"}}
	raw, err := json.Marshal(cp)
	if err != nil {
		t.Fatal(err)
	}
	var want checkpoint
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	c := new(checkpointCache)
	state := provider.ScanState{Sources: map[string]provider.SourceState{"path": {Checkpoint: raw}}}
	c.publish([]string{"path"}, state, map[string]*checkpoint{"path": cp})
	got := c.lookup("path", raw)
	if !reflect.DeepEqual(got, &want) {
		t.Fatal("cached value differs from persisted JSON")
	}
	cp.Canonical.Title = "mutated caller"
	if !reflect.DeepEqual(got, &want) {
		t.Fatal("canonical metadata not owned")
	}
	c.publish([]string{"path"}, state, map[string]*checkpoint{"path": &want})
	if c.lookup("path", raw) != got {
		t.Fatal("unchanged entry recopied")
	}
}

func TestCheckpointCacheAdmission(t *testing.T) {
	cp := &checkpoint{Version: checkpointVersion}
	c := new(checkpointCache)
	raw := make([]byte, checkpointCacheBytes)
	state := provider.ScanState{Sources: map[string]provider.SourceState{"large": {Checkpoint: raw}}}
	c.publish([]string{"large"}, state, map[string]*checkpoint{"large": cp})
	if c.retained != 0 || len(c.entries) != 0 {
		t.Fatal("oversized checkpoint retained")
	}
	state.Sources["small"] = provider.SourceState{Checkpoint: []byte("small")}
	c.publish([]string{"small"}, state, map[string]*checkpoint{"small": cp})
	if c.lookup("small", []byte("small")) == nil {
		t.Fatal("small checkpoint rejected")
	}
	var paths []string
	decoded := make(map[string]*checkpoint)
	for i := range checkpointCacheEntries + 1 {
		path := fmt.Sprint(i)
		paths = append(paths, path)
		state.Sources[path] = provider.SourceState{Checkpoint: []byte("small")}
		decoded[path] = cp
	}
	c.publish(paths, state, decoded)
	if len(c.entries) != checkpointCacheEntries || c.retained > checkpointCacheBytes {
		t.Fatal("admission cap exceeded")
	}
	if c.lookup(paths[len(paths)-1], []byte("small")) != nil || c.lookup("small", []byte("small")) != nil {
		t.Fatal("eviction failed")
	}
	c.publish(nil, provider.ScanState{}, nil)
	if c.retained != 0 || c.lookup("small", []byte("small")) != nil {
		t.Fatal("removed entry retained")
	}
}

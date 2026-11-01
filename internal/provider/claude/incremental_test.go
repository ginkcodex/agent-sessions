package claude_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/claude"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalFixtureBoundaries(t *testing.T) {
	for _, name := range []string{"basic", "split_assistant", "compaction", "meta", "partial_last_line", "unknown_types", "task_notification"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", name+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			path := filepath.Join(root, "projects", "p", name+".jsonl")
			p := claude.New(root, nil)
			cold := claude.New(root, nil)
			ref := model.SessionRef{Agent: model.AgentClaude, ID: name}
			var h providertest.IncrementalHarness
			// Visit the valid unterminated record and its completed newline separately.
			for i := range data {
				if data[i] != '\n' {
					continue
				}
				for _, end := range []int{i, i + 1} {
					write(t, path, data[:end])
					if err := h.Check(t.Context(), p, cold, ref); err != nil {
						t.Fatalf("byte %d: %v", end, err)
					}
				}
			}
			write(t, path, data)
			if err := h.Check(t.Context(), p, cold, ref); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestIncrementalGeneratedAppendWork(t *testing.T) {
	ref := providertest.TranscriptRef("claude", providertest.TranscriptFixtureID)
	// Keep the fixture root explicit for append operations.
	root := t.TempDir()
	path := providertest.WriteTranscript(t, "claude", root, ref.ID, 4<<20)
	p := claude.New(root, nil)
	var h providertest.IncrementalHarness
	cold := claude.New(root, nil)
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 10, 100} {
		providertest.AppendTranscriptRecords(t, "claude", path, 1000+n, n)
		if err := h.Check(t.Context(), p, cold, ref); err != nil {
			t.Fatal(err)
		}
	}
	r, err := provider.LoadIncremental(t.Context(), p, ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := providertest.AppendTranscriptRecords(t, "claude", path, 2000, 1)
	var work provider.LoadWork
	next, err := provider.LoadIncremental(provider.WithLoadWork(t.Context(), &work), p, ref, r.State)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Resumed || work.Calls.Load() != 1 || work.SourceOpens.Load() != 1 || work.SourceBytes.Load() != n || work.ContinuityBytes.Load() > 16384 {
		t.Fatalf("resumed=%v calls=%d opens=%d bytes=%d want=%d continuity=%d", next.Resumed, work.Calls.Load(), work.SourceOpens.Load(), work.SourceBytes.Load(), n, work.ContinuityBytes.Load())
	}
}

func TestIncrementalTailReplacementAndSidecars(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", "parent.jsonl")
	prefix := []byte("{\"type\":\"assistant\",\"message\":{\"id\":\"a\",\"content\":[{\"type\":\"tool_use\",\"id\":\"tool\",\"name\":\"Task\",\"input\":{}}]}}\n")
	write(t, path, prefix)
	p := claude.New(root, nil)
	cold := claude.New(root, nil)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "parent"}
	var h providertest.IncrementalHarness
	check := func() {
		t.Helper()
		if err := h.Check(t.Context(), p, cold, ref); err != nil {
			t.Fatal(err)
		}
	}
	check()
	child := filepath.Join(filepath.Dir(path), "parent", "subagents", "agent-x.jsonl")
	sidecar := filepath.Join(filepath.Dir(child), "agent-x.meta.json")
	write(t, child, []byte("{\"type\":\"user\",\"message\":{\"content\":\"task\"}}\n"))
	write(t, sidecar, []byte(`{"agentType":"Explore","description":"child","toolUseId":"tool"}`))
	check()
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	check()
	tails := [][]byte{
		[]byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tool","content":"error","is_error":true}]}}`),
		[]byte(`{"type":"user","message":{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AA=="}}]},"isSidechain":true}`),
		[]byte(`{"type":"user","message":`),
		nil,
	}
	for _, tail := range tails {
		write(t, path, append(bytes.Clone(prefix), tail...))
		check()
	}
}

func TestIncrementalLongInvalidAndCancelled(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "projects", "p", "long.jsonl")
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "long"}
	p := claude.New(root, nil)
	cold := claude.New(root, nil)
	// Cross the reader's 1 MiB buffer without approaching its 256 MiB limit.
	data := []byte(fmt.Sprintf("{\"type\":\"user\",\"message\":{\"content\":%q}}\n", strings.Repeat("x", 2<<20)))
	write(t, path, data)
	var h providertest.IncrementalHarness
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("not json\n\n{\"type\":\"user\",\"message\":{\"content\":\"next\"}}\n")...)
	write(t, path, data)
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := h.Check(ctx, p, cold, ref); err != context.Canceled {
		t.Fatal(err)
	}
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalChildMetadata(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "p", "parent", "subagents")
	path := filepath.Join(dir, "agent-x.jsonl")
	sidecar := filepath.Join(dir, "agent-x.meta.json")
	write(t, path, []byte("{\"type\":\"user\",\"message\":{\"content\":\"task\"}}\n"))
	p := claude.New(root, nil)
	cold := claude.New(root, nil)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "parent/agent-x"}
	var h providertest.IncrementalHarness
	check := func() {
		t.Helper()
		if err := h.Check(t.Context(), p, cold, ref); err != nil {
			t.Fatal(err)
		}
	}
	check()
	write(t, sidecar, []byte(`{"agentType":"Explore","description":"named child","toolUseId":"tool"}`))
	check()
	write(t, sidecar, []byte(`{"agentType":"Plan","description":"renamed child","toolUseId":"other"}`))
	check()
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestIncrementalFrozenReaders(t *testing.T) {
	root := t.TempDir()
	ref := providertest.TranscriptRef("claude", providertest.TranscriptFixtureID)
	path := providertest.WriteTranscript(t, "claude", root, ref.ID, 0)
	p := claude.New(root, nil)
	first, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := providertest.CloneTranscript(first.Transcript)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			if !reflect.DeepEqual(first.Transcript, want) {
				t.Error("frozen snapshot changed")
				return
			}
		}
	}()
	state := first.State
	for i := range 10 {
		providertest.AppendTranscriptRecords(t, "claude", path, 1000+i*4, 4)
		next, err := p.LoadIncremental(t.Context(), ref, state)
		if err != nil {
			t.Fatal(err)
		}
		state = next.State
	}
	<-done
}

func BenchmarkIncrementalAppend(b *testing.B) {
	for _, mb := range []int{38, 100} {
		for _, records := range []int{1, 10, 100} {
			b.Run(fmt.Sprintf("%dMB/append-%d", mb, records), func(b *testing.B) {
				root := b.TempDir()
				ref := providertest.TranscriptRef("claude", providertest.TranscriptFixtureID)
				path := providertest.WriteTranscript(b, "claude", root, ref.ID, mb*1000000)
				p := claude.New(root, nil)
				var source, continuity, retained int64
				b.ReportAllocs()
				b.ResetTimer()
				b.StopTimer()
				for range b.N {
					initial, err := p.LoadIncremental(b.Context(), ref, nil)
					if err != nil {
						b.Fatal(err)
					}
					providertest.AppendTranscriptRecords(b, "claude", path, 2000, records)
					var work provider.LoadWork
					b.StartTimer()
					next, err := p.LoadIncremental(provider.WithLoadWork(b.Context(), &work), ref, initial.State)
					b.StopTimer()
					if err != nil {
						b.Fatal(err)
					}
					if !next.Resumed {
						b.Fatal("did not resume")
					}
					source += work.SourceBytes.Load()
					continuity += work.ContinuityBytes.Load()
					if next.State != nil {
						retained += next.State.RetainedBytes()
					}
					// Reset generated content outside timing.
					providertest.WriteTranscript(b, "claude", root, ref.ID, mb*1000000)
				}
				b.ReportMetric(float64(source)/float64(b.N), "source-B/op")
				b.ReportMetric(float64(continuity)/float64(b.N), "continuity-B/op")
				b.ReportMetric(float64(retained)/float64(b.N), "state-B/op")
			})
		}
	}
}

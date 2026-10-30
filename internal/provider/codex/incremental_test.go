package codex_test

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
	"github.com/ginkcode/agent-sessions/internal/provider/codex"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func rec(kind, payload string) string {
	return fmt.Sprintf(`{"timestamp":"2026-09-28T12:00:00Z","type":%q,"payload":%s}`+"\n", kind, payload)
}

func rolloutPath(root, day, id string) string {
	return filepath.Join(root, "sessions", "2026", "09", day, "rollout-2026-09-28T12-00-00-"+id+".jsonl")
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// conversation covers every loader path that can reach an earlier message.
func conversation(id string) []byte {
	return []byte(strings.Join([]string{
		rec("session_meta", fmt.Sprintf(`{"id":%q,"cwd":"/nonexistent/codex"}`, id)),
		rec("turn_context", `{"model":"gpt-fixture"}`),
		rec("response_item", `{"type":"message","role":"developer","content":[{"type":"input_text","text":"instructions"}]}`),
		rec("response_item", `{"type":"message","role":"user","content":[{"type":"input_text","text":"question"}]}`),
		rec("response_item", `{"type":"reasoning","summary":[{"text":"summary"}],"content":[{"type":"reasoning_text","text":"raw"}],"encrypted_content":"SECRET"}`),
		rec("response_item", `{"type":"function_call","call_id":"early","name":"shell","arguments":{"big":9007199254740993}}`),
		rec("response_item", `{"type":"function_call","call_id":"late","name":"shell","arguments":"{}"}`),
		rec("response_item", `{"type":"function_call_output","call_id":"early","output":"ok"}`),
		rec("response_item", `{"type":"message","id":"a1","role":"assistant","content":[{"type":"output_text","text":"answer"}]}`),
		rec("response_item", `{"type":"message","id":"a1","role":"assistant","content":[{"type":"output_text","text":"split"}]}`),
		rec("response_item", `{"type":"message","id":"a1","role":"assistant","content":[{"type":"output_text","text":"split"}]}`),
		rec("response_item", `{"type":"custom_tool_call","call_id":"patch","name":"apply_patch","input":{"n":7}}`),
		rec("turn_context", `{"model":"gpt-next"}`),
		rec("response_item", `{"type":"message","role":"user","content":"next"}`),
		rec("response_item", `{"type":"custom_tool_call_output","call_id":"patch","output":"patched","status":"failed"}`),
		rec("response_item", `{"type":"message","id":"a1","role":"assistant","content":[{"type":"output_text","text":"reached again"}]}`),
		rec("response_item", `{"type":"function_call_output","call_id":"missing","output":"x"}`),
		rec("compacted", `{"message":"summary of earlier turns"}`),
		rec("event_msg", `{"type":"turn_aborted","message":"stopped"}`),
		`{"role":"user","content":[{"type":"input_text","text":"historic"}]}` + "\n",
		fmt.Sprintf(`{"id":%q,"cwd":"/nonexistent/codex"}`, id) + "\n",
		rec("response_item", `{"type":"future_type"}`),
		"not json\n",
		rec("response_item", `{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,UE5H"}]}`),
		rec("response_item", fmt.Sprintf(`{"type":"function_call_output","call_id":"late","output":%q}`, strings.Repeat("多", 30000))),
		rec("response_item", `{"type":"reasoning","summary":[{"text":"continues"}]}`),
		rec("response_item", `{"type":"function_call","call_id":"open","name":"shell","arguments":"{}"}`),
	}, ""))
}

func TestIncrementalFixtureBoundaries(t *testing.T) {
	root := t.TempDir()
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "conv"}
	path := rolloutPath(root, "28", ref.ID)
	data := conversation(ref.ID)
	p, cold := codex.New(root, nil), codex.New(root, nil)
	var h providertest.IncrementalHarness
	// Visit each valid unterminated record and its completed newline separately.
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
	// An invalid tail is dropped; completing it later commits the record.
	tails := [][]byte{
		[]byte(`{"timestamp":"2026-09-28T12:00:01Z","type":"response_item","payload":{"type":"function_call_output","call_id":"open","output":"done"}}`),
		[]byte(`{"timestamp":"2026-09-28T12:00:01Z","type":"response_item","payload":{"type":"message","id":"a1","role":"assistant","content":[{"type":"output_text","text":"tail"}]}}`),
		[]byte(`{"type":"response_item","payload":{"type":"message"`),
		[]byte(`{"type":"response_item","payload":{"type":"message","id":"a1","role":"assistant","content":"done"}}` + "\n"),
	}
	for _, tail := range tails {
		write(t, path, append(bytes.Clone(data), tail...))
		if err := h.Check(t.Context(), p, cold, ref); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIncrementalGeneratedAppendWork(t *testing.T) {
	root := t.TempDir()
	ref := providertest.TranscriptRef("codex", providertest.TranscriptFixtureID)
	path := providertest.WriteTranscript(t, "codex", root, ref.ID, 4<<20)
	p, cold := codex.New(root, nil), codex.New(root, nil)
	var h providertest.IncrementalHarness
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, 10, 100} {
		providertest.AppendTranscriptRecords(t, "codex", path, 1000+n, n)
		if err := h.Check(t.Context(), p, cold, ref); err != nil {
			t.Fatal(err)
		}
	}
	r, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	n := providertest.AppendTranscriptRecords(t, "codex", path, 2000, 1)
	var work provider.LoadWork
	next, err := p.LoadIncremental(provider.WithLoadWork(t.Context(), &work), ref, r.State)
	if err != nil {
		t.Fatal(err)
	}
	// Resolution continues its scan; the transcript read is only the append.
	if !next.Resumed || work.Calls.Load() != 1 || work.SourceOpens.Load() != 1 || work.SourceBytes.Load() != n || work.ContinuityBytes.Load() > 16384 {
		t.Fatalf("resumed=%v calls=%d opens=%d bytes=%d want=%d continuity=%d", next.Resumed, work.Calls.Load(), work.SourceOpens.Load(), work.SourceBytes.Load(), n, work.ContinuityBytes.Load())
	}
	if next.FirstChanged < len(r.Transcript.Messages)-1 {
		t.Fatalf("append FirstChanged=%d of %d", next.FirstChanged, len(r.Transcript.Messages))
	}
}

func TestIncrementalCanonicalPathChangeRebuilds(t *testing.T) {
	root := t.TempDir()
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "moved"}
	path := rolloutPath(root, "28", ref.ID)
	write(t, path, conversation(ref.ID))
	p, cold := codex.New(root, nil), codex.New(root, nil)
	var h providertest.IncrementalHarness
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	r, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil || r.State == nil {
		t.Fatal("missing state", err)
	}
	moved := rolloutPath(root, "29", ref.ID)
	if err := os.MkdirAll(filepath.Dir(moved), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	next, err := p.LoadIncremental(t.Context(), ref, r.State)
	if err != nil || next.Resumed || next.FirstChanged != 0 || !strings.HasSuffix(next.Transcript.Meta.SourcePath, filepath.Join("09", "29", filepath.Base(moved))) {
		t.Fatalf("resumed=%v first=%d source=%q err=%v", next.Resumed, next.FirstChanged, next.Transcript.Meta.SourcePath, err)
	}
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalLongInvalidAndCancelled(t *testing.T) {
	root := t.TempDir()
	ref := model.SessionRef{Agent: model.AgentCodex, ID: "long"}
	path := rolloutPath(root, "28", ref.ID)
	// Cross the reader's 1 MiB buffer without approaching its 256 MiB limit.
	data := []byte(rec("session_meta", `{"id":"long"}`) +
		rec("response_item", fmt.Sprintf(`{"type":"function_call","call_id":"big","name":"run","arguments":%q}`, strings.Repeat("x", 2<<20))))
	write(t, path, data)
	p, cold := codex.New(root, nil), codex.New(root, nil)
	var h providertest.IncrementalHarness
	if err := h.Check(t.Context(), p, cold, ref); err != nil {
		t.Fatal(err)
	}
	data = append(data, "not json\n\n"+rec("response_item", `{"type":"function_call_output","call_id":"big","output":"ran"}`)...)
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

func TestIncrementalFrozenReaders(t *testing.T) {
	root := t.TempDir()
	ref := providertest.TranscriptRef("codex", providertest.TranscriptFixtureID)
	path := providertest.WriteTranscript(t, "codex", root, ref.ID, 0)
	p := codex.New(root, nil)
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
		providertest.AppendTranscriptRecords(t, "codex", path, 1000+i*4, 4)
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
				ref := providertest.TranscriptRef("codex", providertest.TranscriptFixtureID)
				path := providertest.WriteTranscript(b, "codex", root, ref.ID, mb*1000000)
				p := codex.New(root, nil)
				var source, continuity, retained, resolution int64
				b.ReportAllocs()
				b.ResetTimer()
				b.StopTimer()
				for range b.N {
					initial, err := p.LoadIncremental(b.Context(), ref, nil)
					if err != nil {
						b.Fatal(err)
					}
					providertest.AppendTranscriptRecords(b, "codex", path, 2000, records)
					var work provider.LoadWork
					var scan provider.ScanWork
					ctx := provider.WithScanWork(provider.WithLoadWork(b.Context(), &work), &scan)
					b.StartTimer()
					next, err := p.LoadIncremental(ctx, ref, initial.State)
					b.StopTimer()
					if err != nil {
						b.Fatal(err)
					}
					if !next.Resumed {
						b.Fatal("did not resume")
					}
					source += work.SourceBytes.Load()
					continuity += work.ContinuityBytes.Load()
					resolution += scan.RecordBytes.Load()
					if next.State != nil {
						retained += next.State.RetainedBytes()
					}
					// Reset generated content outside timing.
					providertest.WriteTranscript(b, "codex", root, ref.ID, mb*1000000)
				}
				b.ReportMetric(float64(source)/float64(b.N), "source-B/op")
				b.ReportMetric(float64(continuity)/float64(b.N), "continuity-B/op")
				b.ReportMetric(float64(resolution)/float64(b.N), "resolve-B/op")
				b.ReportMetric(float64(retained)/float64(b.N), "state-B/op")
			})
		}
	}
}

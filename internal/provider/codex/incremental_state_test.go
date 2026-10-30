package codex

import (
	"context"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestIncrementalLiveToEnded(t *testing.T) {
	p, ref, _ := syntheticRollout(t,
		record("session_meta", `{"id":"fixture"}`),
		record("response_item", `{"type":"message","role":"user","content":"run"}`),
		record("response_item", `{"type":"function_call","call_id":"call","name":"shell","arguments":"{}"}`),
		record("response_item", `{"type":"message","role":"user","content":"later"}`),
	)
	meta, err := p.rolloutFor(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	load := func(live bool, prev provider.TranscriptState) provider.IncrementalResult {
		t.Helper()
		m := meta
		m.Live = live
		r, _, err := p.loadRolloutIncremental(context.Background(), m, prev)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	status := func(r provider.IncrementalResult) model.ToolStatus {
		return r.Transcript.Messages[1].Parts[0].Tool.Status
	}
	live := load(true, nil)
	if status(live) != model.ToolPending {
		t.Fatalf("live status %q", status(live))
	}
	ended := load(false, live.State)
	if !ended.Resumed || ended.FirstChanged > 1 || status(ended) != model.ToolUnknown || status(live) != model.ToolPending {
		t.Fatalf("ended: resumed=%v first=%d status=%q", ended.Resumed, ended.FirstChanged, status(ended))
	}
	again := load(true, ended.State)
	if !again.Resumed || again.FirstChanged > 1 || status(again) != model.ToolPending || status(ended) != model.ToolUnknown {
		t.Fatalf("live again: resumed=%v first=%d status=%q", again.Resumed, again.FirstChanged, status(again))
	}
}

func TestIncrementalStateCap(t *testing.T) {
	p, ref, _ := syntheticRollout(t,
		record("session_meta", `{"id":"fixture"}`),
		record("response_item", `{"type":"message","id":"a","role":"assistant","content":"hello"}`),
	)
	r, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.State == nil || r.State.RetainedBytes() <= 0 {
		t.Fatal("missing accounted state")
	}
	old := stateCap
	stateCap = 0
	t.Cleanup(func() { stateCap = old })
	r, err = p.LoadIncremental(t.Context(), ref, r.State)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Resumed || r.State != nil || len(r.Transcript.Messages) != 1 {
		t.Fatal("cap should drop state, not transcript")
	}
}

func TestIncrementalStateExcludesDiagnostics(t *testing.T) {
	p, ref, _ := syntheticRollout(t,
		record("session_meta", `{"id":"fixture"}`),
		"not json\n",
	)
	r, err := p.LoadIncremental(t.Context(), ref, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st := r.State.(*loadState); st.l.diag != nil {
		t.Fatal("pass diagnostics retained in state")
	}
}

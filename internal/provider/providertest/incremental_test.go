package providertest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

type harnessState struct{ snapshot *model.Transcript }

func (*harnessState) RetainedBytes() int64 { return 0 }

type harnessProvider struct {
	*Fake
	mode    string
	fail    error
	hadPrev bool
}

func (p *harnessProvider) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	p.hadPrev = prev != nil
	if p.fail != nil {
		return provider.IncrementalResult{}, p.fail
	}
	tr, err := p.Load(ctx, ref)
	if err != nil {
		return provider.IncrementalResult{}, err
	}
	tr, _ = CloneTranscript(tr)
	result := provider.IncrementalResult{Transcript: tr, State: &harnessState{tr}}
	if prev != nil {
		old := prev.(*harnessState).snapshot
		result.Resumed = true
		result.FirstChanged = FirstMessageDifference(old.Messages, tr.Messages)
		switch p.mode {
		case "high":
			result.FirstChanged = len(tr.Messages)
		case "mutate":
			old.Messages[0].Parts[0].Text = "corruption"
		case "truncate":
			if len(tr.Messages) < len(old.Messages) {
				result.Transcript = old
			}
		}
	}
	return result, nil
}

func TestIncrementalHarness(t *testing.T) {
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "fixture"}
	for _, mode := range []string{"correct", "high", "mutate", "truncate"} {
		t.Run(mode, func(t *testing.T) {
			f := NewFake(ref.Agent, "generated")
			p := &harnessProvider{Fake: f, mode: mode}
			h := new(IncrementalHarness)
			set := func(texts ...string) {
				tr := &model.Transcript{}
				for _, text := range texts {
					tr.Messages = append(tr.Messages, model.Message{Parts: []model.Part{{Text: text}}})
				}
				f.Transcripts[ref.ID] = tr
			}
			set("a", "b")
			if err := h.Check(t.Context(), p, f, ref); err != nil {
				t.Fatal(err)
			}
			if mode == "truncate" {
				set("a")
			} else {
				set("changed", "b", "c")
			}
			err := h.Check(t.Context(), p, f, ref)
			if mode == "correct" && err != nil {
				t.Fatal(err)
			}
			if mode != "correct" && err == nil {
				t.Fatal("broken provider accepted")
			}
		})
	}
}

func TestIncrementalHarnessEveryBoundaryAndFailure(t *testing.T) {
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "fixture"}
	f := NewFake(ref.Agent, "generated")
	p := &harnessProvider{Fake: f}
	h := new(IncrementalHarness)
	// One message per generated line, visiting every committed boundary.
	lines := strings.Split("first\nsecond\nthird\nfourth\n", "\n")
	tr := &model.Transcript{Messages: []model.Message{}}
	f.Transcripts[ref.ID] = tr
	for _, line := range lines[:len(lines)-1] {
		tr.Messages = append(tr.Messages, model.Message{Parts: []model.Part{{Text: line}}})
		if err := h.Check(t.Context(), p, f, ref); err != nil {
			t.Fatal(err)
		}
	}
	for _, failure := range []error{errors.New("read failure"), context.Canceled} {
		p.fail = failure
		if err := h.Check(t.Context(), p, f, ref); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if err := h.CheckImmutable(); err != nil {
			t.Fatal(err)
		}
		p.fail = nil
		if err := h.Check(t.Context(), p, f, ref); err != nil {
			t.Fatal(err)
		}
		if p.hadPrev {
			t.Fatal("reused consumed state after error")
		}
	}
}

func TestIncrementalFallbackAndCancellation(t *testing.T) {
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "fixture"}
	f := NewFake(ref.Agent, "generated")
	f.Transcripts[ref.ID] = &model.Transcript{}
	r, err := provider.LoadIncremental(t.Context(), f, ref, &harnessState{})
	if err != nil || r.Transcript == nil || r.Resumed || r.FirstChanged != 0 || r.State != nil {
		t.Fatalf("fallback: %+v %v", r, err)
	}
	p := &harnessProvider{Fake: f}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := provider.LoadIncremental(ctx, p, ref, &harnessState{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if p.hadPrev {
		t.Fatal("called provider with cancelled context")
	}
}

func TestCloneTranscriptNested(t *testing.T) {
	tr := &model.Transcript{Messages: []model.Message{{Parts: []model.Part{{Files: []string{}, File: &model.FileRef{Ref: "file"}, Tool: &model.ToolCall{Input: []byte("{}"), Child: &model.SessionRef{ID: "child"}}}}}}}
	copy, err := CloneTranscript(tr)
	if err != nil {
		t.Fatal(err)
	}
	part := &copy.Messages[0].Parts[0]
	part.File.Ref = "other"
	part.Tool.Input[0] = '['
	part.Tool.Child.ID = "other"
	if tr.Messages[0].Parts[0].File.Ref != "file" || string(tr.Messages[0].Parts[0].Tool.Input) != "{}" || tr.Messages[0].Parts[0].Tool.Child.ID != "child" {
		t.Fatal("clone shared nested storage")
	}
	if part.Files == nil {
		t.Fatal("empty slice lost")
	}
}

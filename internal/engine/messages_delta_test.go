package engine

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// deltaSource is an in-memory incremental provider over a mutable transcript.
// A resume reports the first real difference from its state's snapshot, or
// sometimes an earlier index, as a provider may.
type deltaSource struct {
	*providertest.Fake
	mu      sync.Mutex
	rng     *rand.Rand
	tr      *model.Transcript
	rebuild bool // the next load does not resume
}

type deltaState struct{ tr *model.Transcript }

func (*deltaState) RetainedBytes() int64 { return 0 }

func (s *deltaSource) LoadIncremental(_ context.Context, _ model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := provider.IncrementalResult{Transcript: s.tr, State: &deltaState{s.tr}}
	if p, ok := prev.(*deltaState); ok && !s.rebuild {
		result.Resumed = true
		result.FirstChanged = firstDiff(p.tr, s.tr)
		if s.rng.IntN(3) == 0 {
			result.FirstChanged = s.rng.IntN(result.FirstChanged + 1)
		}
	}
	s.rebuild = false
	return result, nil
}

// changeEarlier replaces one message of tr, as a late tool result does.
func changeEarlier(rng *rand.Rand, tr *model.Transcript, id *int) *model.Transcript {
	if len(tr.Messages) == 0 {
		return tr
	}
	next := &model.Transcript{Meta: tr.Meta, Messages: append([]model.Message(nil), tr.Messages...)}
	*id++
	i := rng.IntN(len(next.Messages))
	msg := next.Messages[i]
	msg.Parts = append(append([]model.Part(nil), msg.Parts...), model.Part{Tool: &model.ToolCall{Output: fmt.Sprint("result ", *id)}})
	next.Messages[i] = msg
	return next
}

func sameMessages(a, b []model.Message) bool {
	return len(a) == 0 && len(b) == 0 || reflect.DeepEqual(a, b)
}

// A client window updated only by deltas always equals a fresh full page, over
// appends, truncations, earlier changes, rebuilds, unreported changes,
// evictions and bad base revisions; and a delta never exceeds its limit.
func TestMessagesDeltaMatchesFullPages(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	src := &deltaSource{Fake: providertest.NewFake(model.AgentClaude, "synthetic"), rng: rand.New(rand.NewPCG(1, 2))}
	var id int
	src.tr = mutateTranscript(rng, nil, &id)
	for len(src.tr.Messages) < 30 {
		src.tr = mutateTranscript(rng, src.tr, &id)
	}
	svc := NewService(nil, provider.Set{src})
	t.Cleanup(svc.transcripts.stop)
	ctx := t.Context()
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "delta"}

	page, err := svc.GetMessages(ctx, ref, -1, 20)
	if err != nil || page.Revision == "" {
		t.Fatalf("first page: %q, %v", page.Revision, err)
	}
	win, winOff, rev := page.Messages, page.Offset, page.Revision
	var reusedRounds, advanced, resets int
	for round := range 600 {
		src.mu.Lock()
		switch r := rng.IntN(10); {
		case r < 5:
			src.tr = mutateTranscript(rng, src.tr, &id)
		case r < 7:
			src.tr = changeEarlier(rng, src.tr, &id)
		}
		src.rebuild = rng.IntN(15) == 0
		src.mu.Unlock()
		switch r := rng.IntN(20); {
		case r == 0:
			svc.transcripts.evict(ref.Key())
		case r < 18:
			svc.EvictTranscripts([]model.SessionRef{ref}, nil) // as onChanged does
		}

		req := MessagesSinceRequest{Ref: ref, BaseRevision: rev, HaveOffset: winOff, HaveCount: len(win)}
		switch rng.IntN(3) {
		case 0: // following the tail
			req.Offset, req.Limit = -1, max(len(win), 20)
		case 1: // refreshing the window in place
			req.Offset, req.Limit = winOff, max(len(win), 20)
		default:
			req.Offset, req.Limit = rng.IntN(len(src.tr.Messages)+3), 1+rng.IntN(40)
		}
		switch rng.IntN(25) {
		case 0:
			req.BaseRevision = ""
		case 1:
			req.BaseRevision = "unknown.1.1"
		}
		delta, err := svc.GetMessagesSince(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if delta.Reused+len(delta.Messages) > req.Limit {
			t.Fatalf("round %d: %d+%d messages for limit %d", round, delta.Reused, len(delta.Messages), req.Limit)
		}
		var next []model.Message
		if delta.Reused > 0 {
			if req.BaseRevision != rev || delta.Offset < winOff || delta.Offset-winOff+delta.Reused > len(win) {
				t.Fatalf("round %d: reused %d at %d from window [%d, +%d)", round, delta.Reused, delta.Offset, winOff, len(win))
			}
			reusedRounds++
			if delta.Revision != rev {
				advanced++
			}
			next = append(next, win[delta.Offset-winOff:delta.Offset-winOff+delta.Reused]...)
		} else {
			resets++
		}
		next = append(next, delta.Messages...)

		full, err := svc.GetMessages(ctx, ref, req.Offset, req.Limit)
		if err != nil {
			t.Fatal(err)
		}
		if full.Revision != delta.Revision || full.Offset != delta.Offset || full.TotalCount != delta.TotalCount || full.HasMore != delta.HasMore || full.Limit != delta.Limit {
			t.Fatalf("round %d: page %+v, delta %+v", round, full, delta.MessagesPage)
		}
		if !sameMessages(next, full.Messages) {
			t.Fatalf("round %d: window from delta (reused %d) differs from the full page", round, delta.Reused)
		}
		win, winOff, rev = next, delta.Offset, delta.Revision
	}
	// Reuse across revisions, not only of an unchanged session.
	if reusedRounds < 200 || advanced < 100 || resets < 20 {
		t.Fatalf("reused in %d rounds, %d of them advanced; reset in %d", reusedRounds, advanced, resets)
	}
}

// An unchanged session reuses the whole overlap; a window that starts after
// the page, or a base the cache never issued, reuses nothing.
func TestMessagesDeltaReuse(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	src := &deltaSource{Fake: providertest.NewFake(model.AgentClaude, "synthetic"), rng: rand.New(rand.NewPCG(3, 4))}
	var id int
	src.tr = mutateTranscript(rng, nil, &id)
	for len(src.tr.Messages) < 100 {
		src.tr = mutateTranscript(rng, src.tr, &id)
	}
	svc := NewService(nil, provider.Set{src})
	t.Cleanup(svc.transcripts.stop)
	ctx := t.Context()
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "reuse"}
	page, err := svc.GetMessages(ctx, ref, 40, 30)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name               string
		base               string
		haveOff, haveCount int
		offset, limit      int
		reused             int
	}{
		{"same window", page.Revision, 40, 30, 40, 30, 30},
		{"window shifted later", page.Revision, 40, 30, 50, 30, 20},
		{"window starts before the client's", page.Revision, 40, 30, 30, 30, 0},
		{"client window past the end", page.Revision, 40, 300, 40, 30, 30},
		{"empty client window", page.Revision, 40, 0, 40, 30, 0},
		{"no base", "", 40, 30, 40, 30, 0},
		{"page past the end", page.Revision, 40, 30, 500, 30, 0},
	}
	for _, tc := range cases {
		delta, err := svc.GetMessagesSince(ctx, MessagesSinceRequest{Ref: ref, BaseRevision: tc.base, HaveOffset: tc.haveOff, HaveCount: tc.haveCount, Offset: tc.offset, Limit: tc.limit})
		if err != nil || delta.Reused != tc.reused {
			t.Errorf("%s: reused %d, want %d (%v)", tc.name, delta.Reused, tc.reused, err)
		}
	}

	for _, req := range []MessagesSinceRequest{
		{Ref: ref, Offset: -2, Limit: 1},
		{Ref: ref, Offset: 0, Limit: 0},
		{Ref: ref, Offset: 0, Limit: 1, HaveOffset: -1},
		{Ref: ref, Offset: 0, Limit: 1, HaveCount: -1},
	} {
		if _, err := svc.GetMessagesSince(ctx, req); err == nil {
			t.Errorf("%+v accepted", req)
		}
	}
	if _, err := svc.GetMessagesSince(ctx, MessagesSinceRequest{Ref: model.SessionRef{Agent: "missing", ID: "x"}, Limit: 1}); err == nil {
		t.Error("unknown provider accepted")
	}
}

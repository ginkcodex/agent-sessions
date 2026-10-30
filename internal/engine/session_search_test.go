package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func sessionSearchService(t *testing.T, tr *model.Transcript) (*Service, model.SessionRef) {
	t.Helper()
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "find"}
	fake := providertest.NewFake(model.AgentClaude, "synthetic")
	fake.Transcripts = map[string]*model.Transcript{ref.ID: tr}
	svc := NewService(nil, provider.Set{fake})
	t.Cleanup(svc.transcripts.stop)
	return svc, ref
}

func TestSessionSearchMatches(t *testing.T) {
	tr := &model.Transcript{Messages: []model.Message{
		{Role: model.RoleUser, Parts: []model.Part{{Kind: model.PartText, Text: "Please fix the Retry\n  Policy"}}},
		{Role: model.RoleAssistant, Parts: []model.Part{
			{Kind: model.PartReasoning, Text: "the retry loop"},
			{Kind: model.PartText, Text: "Done."},
			{Kind: model.PartTool, Tool: &model.ToolCall{Name: "Bash", Input: json.RawMessage(`{"command":"go test ./policy"}`), Output: "ok  policy 0.2s"}},
		}},
		{Role: model.RoleUser, IsMeta: true, Parts: []model.Part{{Kind: model.PartText, Text: "retry policy reminder"}}},
		{Role: model.RoleSystem, Parts: []model.Part{{Kind: model.PartNotice, Text: "retry policy notice"}}},
		{Role: model.RoleUser, Parts: []model.Part{{Kind: model.PartFile, File: &model.FileRef{Name: "Policy.png"}}, {Kind: model.PartText, Text: "unrelated"}}},
	}}
	svc, ref := sessionSearchService(t, tr)
	ctx := t.Context()

	cases := []struct {
		query  string
		hidden bool
		want   []SessionSearchHit
	}{
		// Substrings, any case, every term anywhere in the message.
		{query: "RETRY polic", want: []SessionSearchHit{{0, []int{0}}, {1, []int{0, 2}}}},
		{query: "etr", want: []SessionSearchHit{{0, []int{0}}, {1, []int{0}}}},
		// A phrase matches across a line break and spaces.
		{query: `"retry policy"`, want: []SessionSearchHit{{0, []int{0}}}},
		// Tool name, input values and output are searched; JSON keys are not.
		{query: "bash ./policy 0.2s", want: []SessionSearchHit{{1, []int{2}}}},
		{query: "command", want: []SessionSearchHit{}},
		// Attachment names.
		{query: "policy.png", want: []SessionSearchHit{{4, []int{0}}}},
		// Meta and system messages only when asked for.
		{query: `"retry policy"`, hidden: true, want: []SessionSearchHit{{0, []int{0}}, {2, []int{0}}, {3, []int{0}}}},
		// Filter prefixes are not terms.
		{query: "agent:codex done", want: []SessionSearchHit{{1, []int{1}}}},
	}
	for _, tc := range cases {
		got, err := svc.SearchSession(ctx, SessionSearchRequest{Ref: ref, Query: tc.query, IncludeHidden: tc.hidden})
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		if !reflect.DeepEqual(got.Hits, tc.want) || got.Truncated {
			t.Errorf("%q hidden=%v: got %+v truncated=%v, want %+v", tc.query, tc.hidden, got.Hits, got.Truncated, tc.want)
		}
	}

	for _, query := range []string{"", "   ", `"open`, "agent:codex"} {
		if _, err := svc.SearchSession(ctx, SessionSearchRequest{Ref: ref, Query: query}); err == nil {
			t.Errorf("%q: want an error", query)
		}
	}
	if _, err := svc.SearchSession(ctx, SessionSearchRequest{Ref: model.SessionRef{Agent: "nope", ID: "x"}, Query: "x"}); !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("unknown provider: %v", err)
	}
}

func TestSessionSearchLimitAndCancel(t *testing.T) {
	tr := &model.Transcript{}
	for i := range 2*sessionSearchCheckEvery + 3 {
		tr.Messages = append(tr.Messages, model.Message{Role: model.RoleUser, Parts: []model.Part{{Kind: model.PartText, Text: fmt.Sprint("item ", i)}}})
	}
	svc, ref := sessionSearchService(t, tr)

	got, err := svc.SearchSession(t.Context(), SessionSearchRequest{Ref: ref, Query: "item", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hits) != 2 || got.Hits[1].MessageIndex != 1 || !got.Truncated {
		t.Fatalf("limit 2: %+v truncated=%v", got.Hits, got.Truncated)
	}
	got, err = svc.SearchSession(t.Context(), SessionSearchRequest{Ref: ref, Query: "item 1"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Hits); n == 0 || got.Truncated || got.Revision == "" {
		t.Fatalf("default limit: %d hits truncated=%v revision=%q", n, got.Truncated, got.Revision)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := svc.SearchSession(ctx, SessionSearchRequest{Ref: ref, Query: "item"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// Folded text reused across revisions of a changing session gives the same
// hits as searching each snapshot cold, over appends, truncations, earlier
// changes, rebuilds and evictions.
func TestSessionSearchReuseMatchesColdSearch(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	src := &deltaSource{Fake: providertest.NewFake(model.AgentClaude, "synthetic"), rng: rand.New(rand.NewPCG(3, 4))}
	for src.tr == nil || len(src.tr.Messages) < 30 {
		src.tr = mutateSearchTranscript(rng, src.tr, false)
	}
	svc := NewService(nil, provider.Set{src})
	t.Cleanup(svc.transcripts.stop)
	ctx := t.Context()
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "live"}

	var reused, matched int
	var prevRev string
	for round := range 400 {
		src.mu.Lock()
		switch r := rng.IntN(10); {
		case r < 5:
			src.tr = mutateSearchTranscript(rng, src.tr, false)
		case r < 7:
			src.tr = mutateSearchTranscript(rng, src.tr, true)
		}
		src.rebuild = rng.IntN(15) == 0
		snapshot := src.tr
		src.mu.Unlock()
		// Every round sees the source's current snapshot: a dropped entry
		// parses cold, a stale one resumes.
		if rng.IntN(20) == 0 {
			svc.transcripts.evict(ref.Key())
		} else {
			svc.EvictTranscripts([]model.SessionRef{ref}, nil)
		}

		query := searchWords[rng.IntN(len(searchWords))]
		if rng.IntN(3) == 0 {
			query += " " + searchWords[rng.IntN(len(searchWords))]
		}
		before := firstFoldedPart(svc, ref)
		got, err := svc.SearchSession(ctx, SessionSearchRequest{Ref: ref, Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if got.Revision != prevRev && before != nil && before == firstFoldedPart(svc, ref) {
			reused++
		}
		prevRev = got.Revision
		cold, coldRef := sessionSearchService(t, snapshot)
		want, err := cold.SearchSession(ctx, SessionSearchRequest{Ref: coldRef, Query: query})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.Hits, want.Hits) {
			t.Fatalf("round %d, %s: got %+v, want %+v", round, query, got.Hits, want.Hits)
		}
		matched += len(got.Hits)
	}
	if reused == 0 || matched == 0 {
		t.Fatalf("%d searches reused folded text, %d hits", reused, matched)
	}
}

var searchWords = []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}

// mutateSearchTranscript returns a copy of prev that drops up to three
// trailing messages and appends up to three, or, with earlier, rewrites one
// message's text, as a late tool result does.
func mutateSearchTranscript(rng *rand.Rand, prev *model.Transcript, earlier bool) *model.Transcript {
	words := func() string {
		out := make([]string, 1+rng.IntN(4))
		for i := range out {
			out[i] = searchWords[rng.IntN(len(searchWords))]
		}
		return strings.Join(out, " ")
	}
	message := func() model.Message {
		msg := model.Message{Role: model.RoleAssistant, Parts: []model.Part{{Kind: model.PartText, Text: words()}}}
		if rng.IntN(2) == 0 {
			msg.Parts = append(msg.Parts, model.Part{Kind: model.PartTool, Tool: &model.ToolCall{Name: "Bash", Output: words()}})
		}
		return msg
	}
	next := &model.Transcript{}
	if prev != nil {
		next.Messages = append([]model.Message(nil), prev.Messages...)
	}
	if earlier && len(next.Messages) > 0 {
		next.Messages[rng.IntN(len(next.Messages))] = message()
		return next
	}
	next.Messages = next.Messages[:len(next.Messages)-rng.IntN(min(len(next.Messages), 3)+1)]
	for range rng.IntN(4) {
		next.Messages = append(next.Messages, message())
	}
	return next
}

// firstFoldedPart identifies the cached folded text of the first message: the
// same pointer after a search means its text was reused, not folded again.
func firstFoldedPart(svc *Service, ref model.SessionRef) *string {
	cached, ok := svc.searchTexts.Peek(ref.Key())
	if !ok || len(cached.messages) == 0 || len(cached.messages[0].parts) == 0 {
		return nil
	}
	return &cached.messages[0].parts[0]
}

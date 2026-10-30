package engine

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// sizedState is synthetic provider state retaining a chosen number of bytes.
type sizedState int64

func (s sizedState) RetainedBytes() int64 { return int64(s) }

// loadResult loads key from c, replying with result.
func loadResult(t *testing.T, c *transcriptCache, key string, result provider.IncrementalResult) string {
	t.Helper()
	tr, rev, err := c.load(t.Context(), key, true, func(context.Context, provider.TranscriptState) (provider.IncrementalResult, error) {
		return result, nil
	})
	if err != nil || tr != result.Transcript {
		t.Fatalf("load %s: %p, %v", key, tr, err)
	}
	return rev
}

func residentKeys(c *transcriptCache) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var keys []string
	c.lru.Range(func(key string, _ *transcriptEntry) bool {
		keys = append(keys, key)
		return true
	})
	return keys
}

// Mixed sizes evict least recently used entries until the rest fit; the most
// recently used entry stays even when it alone exceeds the budget.
func TestTranscriptByteBudget(t *testing.T) {
	const unit = 10 << 10
	// Each entry also holds an empty snapshot, far smaller than a unit.
	empty := model.TranscriptBytes(&model.Transcript{})
	c := newTranscriptCache(transcriptLRUCapacity, 100*unit+transcriptLRUCapacity*empty, transcriptBackgroundBudget)
	t.Cleanup(c.stop)
	calls := 0
	load := func(key string, units int64) {
		t.Helper()
		_, _, err := c.load(t.Context(), key, true, func(context.Context, provider.TranscriptState) (provider.IncrementalResult, error) {
			calls++
			return provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(units * unit)}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	want := func(keys ...string) {
		t.Helper()
		if got := residentKeys(c); !slices.Equal(got, keys) {
			t.Fatalf("resident %v, want %v", got, keys)
		}
	}
	load("a", 30)
	load("b", 50)
	load("c", 20)
	want("c", "b", "a")
	// 140 units: evicting a alone leaves 110, so b goes too.
	load("d", 40)
	want("d", "c")
	load("c", 0) // a hit makes d the oldest
	if calls != 4 {
		t.Fatalf("calls=%d", calls)
	}
	load("e", 90)
	want("e")
	// The oversized MRU entry is kept and served without reloading.
	load("f", 500)
	want("f")
	load("f", 0)
	if calls != 6 {
		t.Fatalf("oversized MRU reloaded: calls=%d", calls)
	}
	load("g", 10)
	want("g")
	// An exact fit keeps both.
	load("h", 90)
	want("h", "g")

	// Stale entries count towards the budget too.
	c.invalidate("h", "g")
	load("i", 95)
	want("i")
	// So does a detached result kept as stale.
	_, _, err := c.load(t.Context(), "j", true, func(context.Context, provider.TranscriptState) (provider.IncrementalResult, error) {
		c.invalidate("j")
		return provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(20 * unit)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want("j")
}

// mutateTranscript keeps a random prefix of prev's messages and appends new
// ones, sometimes with spare capacity or new metadata.
func mutateTranscript(rng *rand.Rand, prev *model.Transcript, id *int) *model.Transcript {
	text := func() string { return strings.Repeat("x", rng.IntN(64)) }
	next := &model.Transcript{}
	if prev != nil {
		keep := len(prev.Messages) - rng.IntN(min(len(prev.Messages), 3)+1)
		next.Meta = prev.Meta
		next.Messages = make([]model.Message, keep, keep+rng.IntN(4))
		copy(next.Messages, prev.Messages)
	}
	for range rng.IntN(4) {
		*id++
		msg := model.Message{ID: fmt.Sprint(*id), Parts: []model.Part{{Text: text()}}}
		if rng.IntN(2) == 0 {
			msg.Parts = append(msg.Parts, model.Part{Tool: &model.ToolCall{Output: text()}})
		}
		next.Messages = append(next.Messages, msg)
	}
	if rng.IntN(4) == 0 {
		next.Meta.Title = text()
	}
	return next
}

// firstDiff is the earliest index at which b differs from a.
func firstDiff(a, b *model.Transcript) int {
	if a == nil {
		return 0
	}
	i := 0
	for i < len(a.Messages) && i < len(b.Messages) && reflect.DeepEqual(a.Messages[i], b.Messages[i]) {
		i++
	}
	return i
}

func entryBytes(c *transcriptCache, key string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.lru.Peek(key)
	if !ok {
		return -1
	}
	return e.bytes()
}

// Random resumes, rebuilds, detached results and failures: the incremental
// estimate always equals a full recompute, and the change reported since the
// last revision is the minimum FirstChanged since it, never above the first
// real difference.
func TestTranscriptEstimateAndChangesAfterRandomResumes(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	c := newTranscriptCache(transcriptLRUCapacity, transcriptByteBudget, transcriptBackgroundBudget)
	t.Cleanup(c.stop)
	const key = "random"
	var id int
	// published is the last served revision; pending is the minimum
	// FirstChanged since it, -1 when its generation was lost.
	var published, last *model.Transcript
	var rev string
	pending := math.MaxInt
	for round := range 400 {
		next := mutateTranscript(rng, last, &id)
		state := sizedState(rng.Int64N(1 << 10))
		var fc int
		detach, fail := rng.IntN(5) == 0, rng.IntN(20) == 0
		tr, got, err := c.load(t.Context(), key, true, func(_ context.Context, prev provider.TranscriptState) (provider.IncrementalResult, error) {
			if fail {
				return provider.IncrementalResult{}, errors.New("load failed")
			}
			result := provider.IncrementalResult{Transcript: next, State: state}
			if prev != nil && rng.IntN(8) != 0 {
				result.Resumed = true
				result.FirstChanged = rng.IntN(firstDiff(last, next) + 1)
			}
			fc = result.FirstChanged
			if detach {
				c.invalidate(key)
			}
			return result, nil
		})
		switch {
		case fail:
			if err == nil {
				t.Fatal("failure lost")
			}
			// The consumed state cannot be resumed, so the entry is dropped.
			if entryBytes(c, key) != -1 {
				t.Fatalf("round %d: failed entry kept", round)
			}
			last, pending = nil, -1
			continue
		case err != nil || tr != next:
			t.Fatalf("round %d: %p, %v", round, tr, err)
		}
		if want := model.TranscriptBytes(next) + int64(state); entryBytes(c, key) != want {
			t.Fatalf("round %d: estimate %d, recompute %d", round, entryBytes(c, key), want)
		}
		last = next
		if pending >= 0 {
			pending = min(pending, fc)
		}
		if detach {
			// A detached reader gets the snapshot, but no revision.
			if got != "" {
				t.Fatalf("round %d: detached revision %q", round, got)
			}
			continue
		}
		from, ok := c.changedSince(key, rev, got)
		switch {
		case published == nil || pending < 0:
			if ok {
				t.Fatalf("round %d: lost generation reported %d", round, from)
			}
		case !ok:
			t.Fatalf("round %d: revision %q not retained", round, rev)
		case from != pending || from > firstDiff(published, next):
			t.Fatalf("round %d: changed %d, want %d (first difference %d)", round, from, pending, firstDiff(published, next))
		}
		published, rev, pending = next, got, math.MaxInt
		c.invalidate(key)
	}
}

// Every pair of retained revisions reports the minimum FirstChanged between
// them; a base older than the ring resets, and a rebuild reports 0.
func TestTranscriptRevisionHistory(t *testing.T) {
	c := newTranscriptCache(transcriptLRUCapacity, transcriptByteBudget, transcriptBackgroundBudget)
	t.Cleanup(c.stop)
	const key = "history"
	publish := func(fc int, resumed bool) string {
		t.Helper()
		c.invalidate(key)
		return loadResult(t, c, key, provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(0), FirstChanged: fc, Resumed: resumed})
	}
	revs := []string{"", publish(0, false)} // revs[seq]
	changed := []int{0, 0}
	for seq := 2; seq <= 21; seq++ {
		fc := (seq * 7) % 13
		changed = append(changed, fc)
		revs = append(revs, publish(fc, true))
	}
	check := func(last int) {
		t.Helper()
		for b := 1; b <= last; b++ {
			for tg := 1; tg <= last; tg++ {
				from, ok := c.changedSince(key, revs[b], revs[tg])
				want, wantOK := math.MaxInt, b <= tg && b >= last-transcriptHistory
				for s := b + 1; s <= tg; s++ {
					want = min(want, changed[s])
				}
				if ok != wantOK || ok && from != want {
					t.Fatalf("changedSince(%d, %d) = %d, %v; want %d, %v", b, tg, from, ok, want, wantOK)
				}
			}
		}
	}
	check(21)
	// After a reset the client's base is the latest revision again.
	if from, ok := c.changedSince(key, revs[21], revs[21]); !ok || from != math.MaxInt {
		t.Fatal("latest revision")
	}
	// A rebuild changes everything from 0, keeping the generation.
	changed = append(changed, 0)
	revs = append(revs, publish(5, false))
	check(22)
	// A resumed load from the entry's own state still carries a FirstChanged.
	changed = append(changed, 3)
	revs = append(revs, publish(3, true))
	check(23)

	for _, base := range []string{"", "x", revs[23] + "0", "a.b.c", strings.Replace(revs[23], ".23", ".0", 1), strings.Replace(revs[23], ".23", ".24", 1)} {
		if _, ok := c.changedSince(key, base, revs[23]); ok {
			t.Fatalf("base %q accepted", base)
		}
		if _, ok := c.changedSince(key, revs[22], base); ok {
			t.Fatalf("target %q accepted", base)
		}
	}
	if _, ok := c.changedSince("other", revs[22], revs[23]); ok {
		t.Fatal("other key accepted the revision")
	}
	// Eviction loses the history: the next publish starts a generation.
	c.evict(key)
	if _, ok := c.changedSince(key, revs[22], revs[23]); ok {
		t.Fatal("evicted key")
	}
	fresh := loadResult(t, c, key, provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(0)})
	if _, ok := c.changedSince(key, revs[23], fresh); ok {
		t.Fatalf("older generation accepted: %q", fresh)
	}
	other := loadResult(t, c, "other", provider.IncrementalResult{Transcript: &model.Transcript{}})
	if _, ok := c.changedSince(key, other, fresh); ok {
		t.Fatal("another key's generation accepted")
	}
}

func TestTranscriptEpochChanges(t *testing.T) {
	resets := map[string]func(*transcriptCache){
		"clear": (*transcriptCache).clear,
		"restart": func(c *transcriptCache) {
			c.stop()
			c.start(context.Background())
		},
	}
	for name, reset := range resets {
		t.Run(name, func(t *testing.T) {
			c := newTranscriptCache(transcriptLRUCapacity, transcriptByteBudget, transcriptBackgroundBudget)
			t.Cleanup(c.stop)
			result := provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(0)}
			before := loadResult(t, c, "k", result)
			epoch, _, _ := strings.Cut(before, ".")
			reset(c)
			after := loadResult(t, c, "k", result)
			if strings.HasPrefix(after, epoch+".") {
				t.Fatalf("epoch kept: %q then %q", before, after)
			}
			if _, ok := c.changedSince("k", before, after); ok {
				t.Fatal("revision from before the reset accepted")
			}
			if from, ok := c.changedSince("k", after, after); !ok || from != math.MaxInt {
				t.Fatal("new revision")
			}
		})
	}
}

type revResult struct {
	rev string
	err error
}

// startRevision loads ref in the background, returning the provider attempt
// and the result's revision.
func startRevision(t *testing.T, svc *Service, gate *providertest.Barrier, ref model.SessionRef) (*providertest.LoadAttempt, <-chan revResult) {
	t.Helper()
	result := make(chan revResult, 1)
	go func() {
		_, rev, err := svc.loadTranscriptRevision(t.Context(), ref)
		result <- revResult{rev, err}
	}()
	return <-gate.Started, result
}

func mustRev(t *testing.T, result <-chan revResult) string {
	t.Helper()
	r := <-result
	if r.err != nil {
		t.Fatal(r.err)
	}
	return r.rev
}

// replyResumed replies with new state, resuming when the attempt has state.
func replyResumed(a *providertest.LoadAttempt, fc int) provider.TranscriptState {
	state := &resumeState{}
	a.ReplyIncremental(provider.IncrementalResult{Transcript: &model.Transcript{}, State: state, FirstChanged: fc, Resumed: a.Prev != nil}, nil)
	return state
}

// Results that were never published, from detached flights and predecessors,
// carry their FirstChanged into the next revision of the same generation.
func TestTranscriptRevisionsAcrossDetachedResumes(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	key := ref.Key()
	c := svc.transcripts
	a, result := startRevision(t, svc, gate.Barrier, ref)
	replyResumed(a, 0)
	rev1 := mustRev(t, result)

	// Invalidation during a resume keeps the entry for the owner's result.
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	b, result := startRevision(t, svc, gate.Barrier, ref)
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	if c.lru.Len() != 1 {
		t.Fatal("entry dropped while its owner runs")
	}
	state := replyResumed(b, 5)
	if rev := mustRev(t, result); rev != "" {
		t.Fatalf("detached revision %q", rev)
	}
	d, result := startRevision(t, svc, gate.Barrier, ref)
	wantPrev(t, d, state)
	replyResumed(d, 7)
	rev2 := mustRev(t, result)
	if from, ok := c.changedSince(key, rev1, rev2); !ok || from != 5 {
		t.Fatalf("detached change: %d, %v", from, ok)
	}

	for _, outcome := range []string{"success", "failure"} {
		// The successor carries its predecessor's change.
		svc.EvictTranscripts([]model.SessionRef{ref}, nil)
		pred, predRev := startRevision(t, svc, gate.Barrier, ref)
		svc.EvictTranscripts([]model.SessionRef{ref}, nil)
		succRev := make(chan revResult, 1)
		go func() {
			_, rev, err := svc.loadTranscriptRevision(t.Context(), ref)
			succRev <- revResult{rev, err}
		}()
		waitTranscriptState(t, c, func() bool {
			f := c.active[key]
			return f != nil && f.pred != nil
		})
		want := 0 // a failed predecessor leaves a cold rebuild
		if outcome == "success" {
			replyResumed(pred, 9)
			want = 9
			if rev := mustRev(t, predRev); rev != "" {
				t.Fatalf("detached predecessor revision %q", rev)
			}
		} else {
			pred.ReplyIncremental(provider.IncrementalResult{}, errors.New("pred failed"))
			if r := <-predRev; r.err == nil {
				t.Fatal("predecessor failure lost")
			}
		}
		succ := <-gate.Started
		replyResumed(succ, 12)
		rev3 := mustRev(t, succRev)
		if from, ok := c.changedSince(key, rev2, rev3); !ok || from != want {
			t.Fatalf("%s: predecessor change %d, %v; want %d", outcome, from, ok, want)
		}
		rev2 = rev3
	}

	// A failed resume with no successor drops the entry and its history.
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	f, failed := startRevision(t, svc, gate.Barrier, ref)
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	f.ReplyIncremental(provider.IncrementalResult{}, errors.New("resume failed"))
	<-failed
	waitTranscriptState(t, c, func() bool { return len(c.owned) == 0 })
	if c.lru.Len() != 0 {
		t.Fatal("stateless entry kept")
	}
	g, result := startRevision(t, svc, gate.Barrier, ref)
	wantPrev(t, g, nil)
	replyResumed(g, 0)
	if _, ok := c.changedSince(key, rev2, mustRev(t, result)); ok {
		t.Fatal("lost history accepted")
	}
}

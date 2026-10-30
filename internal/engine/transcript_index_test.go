package engine

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// indexLoad runs an index load of key replying with state, and returns the
// state it resumed.
func indexLoad(t *testing.T, c *transcriptCache, key string, state provider.TranscriptState) provider.TranscriptState {
	t.Helper()
	var prev provider.TranscriptState
	want := &model.Transcript{}
	tr, err := c.loadForIndex(t.Context(), key, func(_ context.Context, p provider.TranscriptState) (provider.IncrementalResult, error) {
		prev = p
		return provider.IncrementalResult{Transcript: want, State: state, Resumed: p != nil}, nil
	})
	if err != nil || tr != want {
		t.Fatalf("index load %s: %p, %v", key, tr, err)
	}
	return prev
}

func backgroundKey(c *transcriptCache) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bgKey
}

// An index load sees appends no invalidation reported, by resuming even a
// fresh foreground entry; repeated jobs for a session that is not open resume
// from the background slot, which the first foreground load promotes.
func TestLoadForIndexResumesToCurrentSource(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			path := providertest.WriteTranscript(t, agent, root, providertest.TranscriptFixtureID, 2_000_000)
			ref := providertest.TranscriptRef(agent, providertest.TranscriptFixtureID)
			key := ref.Key()
			work := new(provider.LoadWork)
			svc := NewService(nil, provider.Set{providertest.TranscriptProvider(agent, root)})
			svc.transcripts.start(provider.WithLoadWork(context.Background(), work))
			t.Cleanup(svc.transcripts.stop)
			c := svc.transcripts
			turn := providertest.TranscriptTurns
			appendAndIndex := func() *model.Transcript {
				t.Helper()
				before := work.SourceBytes.Load()
				appended := providertest.AppendTranscriptRecords(t, agent, path, turn, providertest.TranscriptTurnRecords(agent))
				turn++
				got, err := svc.LoadForIndex(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				if read := work.SourceBytes.Load() - before; read > appended+4096 {
					t.Fatalf("index load read %d bytes for %d appended", read, appended)
				}
				want, err := providertest.TranscriptProvider(agent, root).Load(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatal("index snapshot differs from a cold load")
				}
				return got
			}

			// Not open: one cold parse, then resumes from the background slot.
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.LoadForIndex(t.Context(), ref); err != nil {
				t.Fatal(err)
			}
			if work.Calls.Load() != 1 || work.SourceBytes.Load() < info.Size() {
				t.Fatalf("cold parse: calls=%d bytes=%d", work.Calls.Load(), work.SourceBytes.Load())
			}
			for range 3 {
				appendAndIndex()
				if c.lru.Len() != 0 || backgroundKey(c) != key {
					t.Fatalf("background job: resident %v, background %q", residentKeys(c), backgroundKey(c))
				}
			}

			// Opening it promotes the background state: the load resumes.
			before := work.SourceBytes.Load()
			appended := providertest.AppendTranscriptRecords(t, agent, path, turn, providertest.TranscriptTurnRecords(agent))
			turn++
			last, rev, err := svc.loadTranscriptRevision(t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			if read := work.SourceBytes.Load() - before; read > appended+4096 {
				t.Fatalf("promotion read %d bytes for %d appended", read, appended)
			}
			if c.lru.Len() != 1 || backgroundKey(c) != "" {
				t.Fatalf("promotion: resident %v, background %q", residentKeys(c), backgroundKey(c))
			}

			// Open: an append the scanner has not reported yet is still
			// indexed, and advances the foreground entry.
			for range 3 {
				got := appendAndIndex()
				hit, next, err := svc.loadTranscriptRevision(t.Context(), ref)
				if err != nil || hit != got {
					t.Fatalf("foreground entry not advanced: %v", err)
				}
				if from, ok := c.changedSince(key, rev, next); !ok || from > firstDiff(last, got) || from < len(last.Messages)-2 {
					t.Fatalf("changed %d, %v; first difference %d of %d", from, ok, firstDiff(last, got), len(last.Messages))
				}
				last, rev = got, next
			}
			if backgroundKey(c) != "" {
				t.Fatal("open session kept in the background slot")
			}
			if want := int64(8); work.Calls.Load() != want {
				t.Fatalf("calls=%d, want %d", work.Calls.Load(), want)
			}
		})
	}
}

// Index jobs never reorder or evict foreground entries; advancing one keeps
// its recency but publishes a revision.
func TestLoadForIndexKeepsForegroundOrder(t *testing.T) {
	const unit = 1 << 20
	c := newTranscriptCache(transcriptLRUCapacity, 100*unit, 90*unit)
	t.Cleanup(c.stop)
	result := func(units int64) provider.IncrementalResult {
		return provider.IncrementalResult{Transcript: &model.Transcript{}, State: sizedState(units * unit)}
	}
	for _, key := range []string{"a", "b", "c", "d"} {
		loadResult(t, c, key, result(20))
	}
	want := []string{"d", "c", "b", "a"}
	check := func() {
		t.Helper()
		if got := residentKeys(c); !slices.Equal(got, want) {
			t.Fatalf("resident %v, want %v", got, want)
		}
	}
	// Background jobs larger than the foreground budget's headroom.
	for _, key := range []string{"x", "y", "x", "z"} {
		indexLoad(t, c, key, sizedState(80*unit))
		check()
	}
	if backgroundKey(c) != "z" {
		t.Fatalf("background %q", backgroundKey(c))
	}

	// Advancing the least recently used entry keeps it least recently used.
	c.mu.Lock()
	e, _ := c.lru.Peek("a")
	rev1 := c.revisionLocked(e, e.seq)
	c.mu.Unlock()
	if prev := indexLoad(t, c, "a", sizedState(20*unit)); prev != sizedState(20*unit) {
		t.Fatalf("foreground entry not resumed: %v", prev)
	}
	check()
	if backgroundKey(c) != "z" {
		t.Fatal("foreground job replaced the background slot")
	}
	// The next foreground load hits the advanced entry.
	_, rev2, err := c.load(t.Context(), "a", true, func(context.Context, provider.TranscriptState) (provider.IncrementalResult, error) {
		return provider.IncrementalResult{}, errors.New("advanced entry reloaded")
	})
	if err != nil {
		t.Fatal(err)
	}
	if from, ok := c.changedSince("a", rev1, rev2); !ok || from != 0 {
		t.Fatalf("changed %d, %v", from, ok)
	}
	want = []string{"a", "d", "c", "b"}
	check()
}

// The background slot holds one session within its budget, and goes with an
// eviction or a clear.
func TestLoadForIndexBackgroundSlot(t *testing.T) {
	const budget = 1 << 20
	c := newTranscriptCache(transcriptLRUCapacity, transcriptByteBudget, budget)
	t.Cleanup(c.stop)
	empty := model.TranscriptBytes(&model.Transcript{})
	fits := sizedState(budget - empty)

	indexLoad(t, c, "k", fits)
	if prev := indexLoad(t, c, "k", fits+1); prev != fits {
		t.Fatalf("background state not resumed: %v", prev)
	}
	if prev := indexLoad(t, c, "k", fits); prev != nil {
		t.Fatalf("over-budget state kept: %v", prev)
	}
	// Another session replaces it.
	indexLoad(t, c, "other", fits)
	if prev := indexLoad(t, c, "k", fits); prev != nil {
		t.Fatalf("replaced state kept: %v", prev)
	}
	c.evict("k")
	if prev := indexLoad(t, c, "k", fits); prev != nil {
		t.Fatalf("evicted state kept: %v", prev)
	}
	c.clear()
	if prev := indexLoad(t, c, "k", fits); prev != nil {
		t.Fatalf("cleared state kept: %v", prev)
	}
	// Invalidation does not drop it: an index load always resumes to the end.
	c.invalidate("k")
	if prev := indexLoad(t, c, "k", fits); prev != fits {
		t.Fatalf("invalidated state dropped: %v", prev)
	}
	// A failed load leaves nothing to resume.
	if _, err := c.loadForIndex(t.Context(), "k", func(context.Context, provider.TranscriptState) (provider.IncrementalResult, error) {
		return provider.IncrementalResult{}, errors.New("failed")
	}); err == nil {
		t.Fatal("failure lost")
	}
	if prev := indexLoad(t, c, "k", fits); prev != nil {
		t.Fatalf("state kept after failure: %v", prev)
	}
	if c.lru.Len() != 0 {
		t.Fatalf("index loads made entries: %v", residentKeys(c))
	}
}

func startIndex(t *testing.T, svc *Service, gate *providertest.Barrier, ref model.SessionRef) (*providertest.LoadAttempt, <-chan *model.Transcript) {
	t.Helper()
	result := make(chan *model.Transcript, 1)
	go func() {
		tr, err := svc.LoadForIndex(t.Context(), ref)
		if err != nil {
			t.Error(err)
		}
		result <- tr
	}()
	return <-gate.Started, result
}

// An index load never joins a foreground flight that may have read the source
// before it: it continues that flight's result, and publishes in its place.
func TestLoadForIndexDetachesForegroundFlight(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	key := ref.Key()
	c := svc.transcripts
	a, fg := startRevision(t, svc, gate.Barrier, ref)
	indexed := make(chan *model.Transcript, 1)
	go func() {
		tr, err := svc.LoadForIndex(t.Context(), ref)
		if err != nil {
			t.Error(err)
		}
		indexed <- tr
	}()
	waitTranscriptState(t, c, func() bool {
		f := c.active[key]
		return f != nil && f.pred != nil
	})
	_, s1 := reply(a, 1)
	if rev := mustRev(t, fg); rev != "" {
		t.Fatalf("detached foreground revision %q", rev)
	}
	b := <-gate.Started
	wantPrev(t, b, s1)
	tr2, _ := reply(b, 2)
	if got := <-indexed; got != tr2 {
		t.Fatal("index load returned another snapshot")
	}
	// Published for the foreground: the next load hits it.
	if got, err := svc.loadTranscript(t.Context(), ref); err != nil || got != tr2 {
		t.Fatalf("foreground load: %v", err)
	}
	if c.lru.Len() != 1 || backgroundKey(c) != "" {
		t.Fatalf("resident %v, background %q", residentKeys(c), backgroundKey(c))
	}

	// A foreground reader joining a background index flight makes it
	// foreground.
	other := model.SessionRef{Agent: ref.Agent, ID: "other"}
	d, bg := startIndex(t, svc, gate.Barrier, other)
	fgOther := requestTranscript(t.Context(), svc, other)
	waitTranscriptState(t, c, func() bool { return c.active[other.Key()].waiters == 2 })
	tr3, _ := reply(d, 3)
	if <-bg != tr3 {
		t.Fatal("index snapshot")
	}
	if err := <-fgOther; err != nil {
		t.Fatal(err)
	}
	if got := residentKeys(c); !slices.Equal(got, []string{other.Key(), key}) || backgroundKey(c) != "" {
		t.Fatalf("resident %v, background %q", got, backgroundKey(c))
	}
}

// Without incremental loading, index loads parse in full and leave the cache
// alone.
func TestLoadForIndexNonResumable(t *testing.T) {
	gate, svc := newTranscriptGate(t, false, 4)
	t.Cleanup(svc.transcripts.stop)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "plain"}
	open, opened := startTranscript(t, svc, gate, ref)
	fg := &model.Transcript{}
	open.Reply(fg, nil)
	<-opened
	for range 2 {
		a, result := startIndex(t, svc, gate, ref)
		want := &model.Transcript{}
		a.Reply(want, nil)
		if <-result != want {
			t.Fatal("snapshot")
		}
	}
	c := svc.transcripts
	if got, err := svc.loadTranscript(t.Context(), ref); err != nil || got != fg {
		t.Fatalf("foreground entry replaced: %v", err)
	}
	if c.lru.Len() != 1 || len(c.active) != 0 || backgroundKey(c) != "" {
		t.Fatal("index load used the cache")
	}
	if _, err := svc.LoadForIndex(t.Context(), model.SessionRef{Agent: "missing", ID: "x"}); !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("unknown provider: %v", err)
	}
}

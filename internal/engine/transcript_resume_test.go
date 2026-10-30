package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

// resumeState is synthetic provider state; gen identifies the load that made it.
type resumeState struct{ gen int }

func (*resumeState) RetainedBytes() int64 { return 1 }

// statefulGate publishes fresh state with every successful stateless reply, so
// stale entries and predecessor flights take part in the PF-04/PF-05 tests.
type statefulGate struct {
	providertest.IncrementalBarrier
}

func (g statefulGate) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	result, err := g.IncrementalBarrier.LoadIncremental(ctx, ref, prev)
	if err == nil && result.State == nil {
		result.State = &resumeState{}
	}
	return result, err
}

// newTranscriptGate returns a barrier and a service loading through it. A
// resumable gate implements provider.IncrementalLoader.
func newTranscriptGate(t *testing.T, resumable bool, capacity int) (*providertest.Barrier, *Service) {
	t.Helper()
	var p provider.Provider
	gate := providertest.NewBarrier(providertest.NewFake(model.AgentClaude, "synthetic"), capacity)
	p = gate
	if resumable {
		p = statefulGate{providertest.IncrementalBarrier{Barrier: gate}}
	}
	t.Cleanup(gate.Close)
	return gate, NewService(nil, provider.Set{p})
}

func forTranscriptGates(t *testing.T, test func(t *testing.T, resumable bool)) {
	for _, resumable := range []bool{false, true} {
		t.Run(fmt.Sprintf("resumable=%v", resumable), func(t *testing.T) { test(t, resumable) })
	}
}

// newResumeGate is an incremental barrier whose replies the test supplies.
func newResumeGate(t *testing.T) (providertest.IncrementalBarrier, *Service, model.SessionRef) {
	t.Helper()
	gate := providertest.NewIncrementalBarrier(providertest.NewFake(model.AgentClaude, "synthetic"), 4)
	t.Cleanup(gate.Close)
	svc := NewService(nil, provider.Set{gate})
	t.Cleanup(svc.transcripts.stop)
	return gate, svc, model.SessionRef{Agent: model.AgentClaude, ID: "resume"}
}

// reply completes a with a new snapshot and state numbered gen.
func reply(a *providertest.LoadAttempt, gen int) (*model.Transcript, *resumeState) {
	tr, state := &model.Transcript{Meta: model.SessionMeta{Title: fmt.Sprint(gen)}}, &resumeState{gen: gen}
	a.ReplyIncremental(provider.IncrementalResult{Transcript: tr, State: state, Resumed: a.Prev != nil}, nil)
	return tr, state
}

func wantPrev(t *testing.T, a *providertest.LoadAttempt, want provider.TranscriptState) {
	t.Helper()
	if a.Prev != want {
		t.Fatalf("prev = %v, want %v", a.Prev, want)
	}
}

func TestTranscriptStaleResume(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	a, result := startTranscript(t, svc, gate.Barrier, ref)
	wantPrev(t, a, nil)
	first, state := reply(a, 1)
	if <-result != first {
		t.Fatal("first load")
	}
	if got, err := svc.loadTranscript(t.Context(), ref); got != first || err != nil {
		t.Fatal("fresh entry not served")
	}
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	// The stale snapshot is never served; the reload continues its state.
	b, result := startTranscript(t, svc, gate.Barrier, ref)
	wantPrev(t, b, state)
	second, state := reply(b, 2)
	if <-result != second {
		t.Fatal("resumed load")
	}
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	c, result := startTranscript(t, svc, gate.Barrier, ref)
	wantPrev(t, c, state)
	third, _ := reply(c, 3)
	if <-result != third || gate.Calls.Load() != 3 {
		t.Fatal("second resume")
	}
}

func TestTranscriptStaleResumeFailureKeepsNothing(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	a, result := startTranscript(t, svc, gate.Barrier, ref)
	_, state := reply(a, 1)
	<-result
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	failure := errors.New("resume failed")
	errs := requestTranscript(t.Context(), svc, ref)
	b := <-gate.Started
	wantPrev(t, b, state)
	b.ReplyIncremental(provider.IncrementalResult{}, failure)
	if err := <-errs; !errors.Is(err, failure) {
		t.Fatal(err)
	}
	// The provider consumed the state, so the retry is cold.
	c, result := startTranscript(t, svc, gate.Barrier, ref)
	wantPrev(t, c, nil)
	reply(c, 2)
	<-result
}

// A cancelled flight that never reached the provider returns its state.
func TestTranscriptQueuedCancellationKeepsState(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	a, result := startTranscript(t, svc, gate.Barrier, ref)
	_, state := reply(a, 1)
	<-result
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	var busy []*providertest.LoadAttempt
	var busyResults []<-chan error
	for i := range transcriptLoadLimit {
		busyResults = append(busyResults, requestTranscript(t.Context(), svc, model.SessionRef{Agent: model.AgentClaude, ID: fmt.Sprint(i)}))
		busy = append(busy, <-gate.Started)
	}
	ctx, cancel := context.WithCancel(t.Context())
	queued := requestTranscript(ctx, svc, ref)
	waitTranscriptState(t, svc.transcripts, func() bool { return svc.transcripts.active[ref.Key()] != nil })
	cancel()
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitTranscriptState(t, svc.transcripts, func() bool { return len(svc.transcripts.owned) == transcriptLoadLimit })
	// Queue the retry before the busy loads publish and fill the LRU.
	retry := requestTranscript(t.Context(), svc, ref)
	waitTranscriptState(t, svc.transcripts, func() bool { return svc.transcripts.active[ref.Key()] != nil })
	for i, b := range busy {
		reply(b, 10+i)
		<-busyResults[i]
	}
	b := <-gate.Started
	wantPrev(t, b, state)
	reply(b, 2)
	if err := <-retry; err != nil {
		t.Fatal(err)
	}
}

func TestTranscriptPredecessorHandoff(t *testing.T) {
	for _, outcome := range []string{"success", "failure", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			gate, svc, ref := newResumeGate(t)
			a, result := startTranscript(t, svc, gate.Barrier, ref)
			_, state := reply(a, 1)
			<-result
			svc.EvictTranscripts([]model.SessionRef{ref}, nil)

			// Invalidation during a resume detaches it, but it keeps the state.
			predCtx, cancelPred := context.WithCancel(t.Context())
			defer cancelPred()
			predErr := requestTranscript(predCtx, svc, ref)
			pred := <-gate.Started
			wantPrev(t, pred, state)
			svc.EvictTranscripts([]model.SessionRef{ref}, nil)
			succResult := make(chan *model.Transcript, 1)
			go func() {
				tr, err := svc.loadTranscript(t.Context(), ref)
				if err != nil {
					t.Errorf("successor: %v", err)
				}
				succResult <- tr
			}()
			// The successor waits on the predecessor without a provider call or slot.
			waitTranscriptState(t, svc.transcripts, func() bool {
				f := svc.transcripts.active[ref.Key()]
				return f != nil && f.pred != nil && f.pred.waiters == 2
			})
			if gate.Calls.Load() != 2 || len(svc.transcripts.slots) != 1 {
				t.Fatalf("successor started early: calls=%d slots=%d", gate.Calls.Load(), len(svc.transcripts.slots))
			}
			var want provider.TranscriptState
			switch outcome {
			case "success":
				_, want = reply(pred, 2)
				if err := <-predErr; err != nil {
					t.Fatal(err)
				}
			case "failure":
				pred.ReplyIncremental(provider.IncrementalResult{}, errors.New("pred failed"))
				if err := <-predErr; err == nil {
					t.Fatal("predecessor failure lost")
				}
			case "cancellation":
				// Its caller leaves, but the waiting successor keeps it alive
				// until the stopped provider returns.
				cancelPred()
				if err := <-predErr; !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				if pred.Context.Err() != nil {
					t.Fatal("successor's wait did not keep the predecessor alive")
				}
				pred.ReplyIncremental(provider.IncrementalResult{}, context.Canceled)
			}
			succ := <-gate.Started
			wantPrev(t, succ, want)
			tr, _ := reply(succ, 3)
			if <-succResult != tr {
				t.Fatal("successor result")
			}
			if got, err := svc.loadTranscript(t.Context(), ref); got != tr || err != nil {
				t.Fatal("successor not published")
			}
			waitTranscriptState(t, svc.transcripts, func() bool { return len(svc.transcripts.owned) == 0 && len(svc.transcripts.owner) == 0 })
		})
	}
}

// A successor that gives up while its predecessor runs hands ownership back.
func TestTranscriptSuccessorCancellationRestoresOwner(t *testing.T) {
	gate, svc, ref := newResumeGate(t)
	a, result := startTranscript(t, svc, gate.Barrier, ref)
	_, state := reply(a, 1)
	<-result
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	predErr := requestTranscript(t.Context(), svc, ref)
	pred := <-gate.Started
	wantPrev(t, pred, state)
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	ctx, cancel := context.WithCancel(t.Context())
	succ := requestTranscript(ctx, svc, ref)
	waitTranscriptState(t, svc.transcripts, func() bool {
		f := svc.transcripts.active[ref.Key()]
		return f != nil && f.pred != nil
	})
	cancel()
	if err := <-succ; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	waitTranscriptState(t, svc.transcripts, func() bool { return len(svc.transcripts.owned) == 1 })
	if svc.transcripts.owner[ref.Key()] == nil || pred.Context.Err() != nil {
		t.Fatal("predecessor lost ownership or was cancelled")
	}
	_, next := reply(pred, 2)
	if err := <-predErr; err != nil {
		t.Fatal(err)
	}
	// The detached result is kept as stale state, never served.
	c, result := startTranscript(t, svc, gate.Barrier, ref)
	wantPrev(t, c, next)
	reply(c, 3)
	<-result
}

// Removal, Forget, clear and shutdown drop state, resident or in flight.
func TestTranscriptDropsState(t *testing.T) {
	drops := map[string]func(*Service, model.SessionRef){
		"removed": func(svc *Service, ref model.SessionRef) { svc.EvictTranscripts(nil, []model.SessionRef{ref}) },
		"forget":  func(svc *Service, ref model.SessionRef) { svc.EvictTranscript(ref.Key()) },
		"clear":   func(svc *Service, _ model.SessionRef) { svc.ClearTranscripts() },
		"restart": func(svc *Service, _ model.SessionRef) {
			svc.transcripts.stop()
			svc.transcripts.start(context.Background())
		},
	}
	for name, drop := range drops {
		for _, inflight := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/inflight=%v", name, inflight), func(t *testing.T) {
				gate, svc, ref := newResumeGate(t)
				a, result := startTranscript(t, svc, gate.Barrier, ref)
				reply(a, 1)
				<-result
				svc.EvictTranscripts([]model.SessionRef{ref}, nil)
				var pred *providertest.LoadAttempt
				var predErr <-chan error
				if inflight {
					predErr = requestTranscript(t.Context(), svc, ref)
					pred = <-gate.Started
				}
				drop(svc, ref)
				// The next load neither waits for nor continues the dropped state.
				b, result := startTranscript(t, svc, gate.Barrier, ref)
				wantPrev(t, b, nil)
				tr, _ := reply(b, 2)
				if <-result != tr {
					t.Fatal("cold reload")
				}
				if inflight {
					reply(pred, 3)
					<-predErr
					waitTranscriptState(t, svc.transcripts, func() bool { return len(svc.transcripts.owned) == 0 })
				}
				if got, err := svc.loadTranscript(t.Context(), ref); got != tr || err != nil {
					t.Fatal("dropped flight replaced the reload")
				}
			})
		}
	}
}

// A provider without state never waits for a detached predecessor.
func TestTranscriptNonResumableDoesNotWait(t *testing.T) {
	gate, svc := newTranscriptGate(t, false, 4)
	t.Cleanup(svc.transcripts.stop)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "plain"}
	a, old := startTranscript(t, svc, gate, ref)
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	b, fresh := startTranscript(t, svc, gate, ref)
	tr := &model.Transcript{}
	b.Reply(tr, nil)
	if <-fresh != tr {
		t.Fatal("fresh load")
	}
	a.Reply(&model.Transcript{}, nil)
	<-old
	svc.EvictTranscripts([]model.SessionRef{ref}, nil)
	// A stateless entry cannot resume, so invalidation frees its LRU slot.
	if svc.transcripts.lru.Len() != 0 {
		t.Fatal("stateless entry retained as stale")
	}
}

// One cold parse, then every reload after an append burst resumes, and each
// served snapshot equals a cold load of the same file.
func TestTranscriptAppendBurstResumes(t *testing.T) {
	for _, agent := range []string{"claude", "codex"} {
		t.Run(agent, func(t *testing.T) {
			root := t.TempDir()
			path := providertest.WriteTranscript(t, agent, root, providertest.TranscriptFixtureID, 2_000_000)
			ref := providertest.TranscriptRef(agent, providertest.TranscriptFixtureID)
			work := new(provider.LoadWork)
			svc := NewService(nil, provider.Set{providertest.TranscriptProvider(agent, root)})
			svc.transcripts.start(provider.WithLoadWork(context.Background(), work))
			t.Cleanup(svc.transcripts.stop)
			last, rev, err := svc.loadTranscriptRevision(t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if work.Calls.Load() != 1 || work.SourceBytes.Load() < info.Size() {
				t.Fatalf("cold parse: calls=%d bytes=%d size=%d", work.Calls.Load(), work.SourceBytes.Load(), info.Size())
			}
			for i := range 8 {
				before := work.SourceBytes.Load()
				appended := providertest.AppendTranscriptRecords(t, agent, path, providertest.TranscriptTurns+i, providertest.TranscriptTurnRecords(agent))
				svc.EvictTranscripts([]model.SessionRef{ref}, nil) // as onChanged does
				got, next, err := svc.loadTranscriptRevision(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				// A resume reads the appended records plus at most the
				// previously unterminated tail, never the whole file.
				if read := work.SourceBytes.Load() - before; read > appended+4096 {
					t.Fatalf("append %d: read %d bytes for %d appended", i, read, appended)
				}
				want, err := providertest.TranscriptProvider(agent, root).Load(t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("append %d: resumed snapshot differs from a cold load", i)
				}
				// The incremental estimate equals a full recompute, and the
				// revision reports no change before the first real difference.
				svc.transcripts.mu.Lock()
				e, _ := svc.transcripts.lru.Peek(ref.Key())
				estimate, recompute := e.bytes(), model.TranscriptBytes(got)+e.rp.state.RetainedBytes()
				svc.transcripts.mu.Unlock()
				if estimate != recompute {
					t.Fatalf("append %d: estimate %d, recompute %d", i, estimate, recompute)
				}
				if from, ok := svc.transcripts.changedSince(ref.Key(), rev, next); !ok || from > firstDiff(last, got) || from < len(last.Messages)-2 {
					t.Fatalf("append %d: changed %d, %v; first difference %d of %d", i, from, ok, firstDiff(last, got), len(last.Messages))
				}
				last, rev = got, next
			}
			if work.Calls.Load() != 9 {
				t.Fatalf("calls=%d", work.Calls.Load())
			}
		})
	}
}

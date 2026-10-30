package providertest

import (
	"context"
	"errors"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

func TestBarrierOrderingAndCancellation(t *testing.T) {
	b := NewBarrier(NewFake(model.AgentClaude, "synthetic"), 2)
	t.Cleanup(b.Close)
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "synthetic"}
	first := make(chan *model.Transcript, 1)
	go func() { tr, _ := b.Load(t.Context(), ref); first <- tr }()
	a := <-b.Started
	second := make(chan *model.Transcript, 1)
	go func() { tr, _ := b.Load(t.Context(), ref); second <- tr }()
	c := <-b.Started
	one, two := &model.Transcript{}, &model.Transcript{}
	c.Reply(two, nil)
	if <-second != two {
		t.Fatal("second reply")
	}
	a.Reply(one, nil)
	if <-first != one || b.Calls.Load() != 2 {
		t.Fatal("first reply/count")
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := b.Load(ctx, ref); done <- err }()
	attempt := <-b.Started
	cancel()
	if !errors.Is(<-done, context.Canceled) || attempt.Context.Err() != context.Canceled {
		t.Fatal("cancellation not observed")
	}
}

func TestBarrierIgnoredCancellationAndClose(t *testing.T) {
	b := NewBarrier(NewFake(model.AgentClaude, "synthetic"), 1)
	t.Cleanup(b.Close)
	b.IgnoreCancellation = true
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan error, 1)
	go func() { _, err := b.Load(ctx, model.SessionRef{}); done <- err }()
	attempt := <-b.Started
	sentinel := errors.New("reply")
	attempt.Reply(nil, sentinel)
	if !errors.Is(<-done, sentinel) {
		t.Fatal("ignored cancellation should permit reply")
	}
	go func() { _, err := b.Load(ctx, model.SessionRef{}); done <- err }()
	<-b.Started
	b.Close()
	b.Close()
	if !errors.Is(<-done, context.Canceled) {
		t.Fatal("close did not unblock load")
	}
}

type barrierState struct{}

func (barrierState) RetainedBytes() int64 { return 0 }

func TestIncrementalBarrier(t *testing.T) {
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "synthetic"}
	fake := NewFake(model.AgentClaude, "synthetic")
	want := &model.Transcript{}
	fake.Transcripts[ref.ID] = want
	b := NewIncrementalBarrier(fake, 1)
	t.Cleanup(b.Close)
	var _ provider.IncrementalLoader = b
	done := make(chan provider.IncrementalResult, 1)
	prev := barrierState{}
	go func() { r, _ := b.LoadIncremental(t.Context(), ref, prev); done <- r }()
	a := <-b.Started
	if a.Prev != prev {
		t.Fatal("prev not announced")
	}
	result := provider.IncrementalResult{Transcript: &model.Transcript{}, State: barrierState{}, Resumed: true}
	a.ReplyIncremental(result, nil)
	if got := <-done; got != result {
		t.Fatal("incremental reply")
	}
	// Forward falls back to the wrapped provider's full rebuild.
	go func() { r, _ := b.LoadIncremental(t.Context(), ref, prev); done <- r }()
	(<-b.Started).Forward()
	if got := <-done; got.Transcript != want || got.State != nil || got.Resumed {
		t.Fatal("forward")
	}
	if b.Calls.Load() != 2 {
		t.Fatal("calls")
	}
}

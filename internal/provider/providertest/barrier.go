package providertest

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// LoadAttempt is one blocked load. Tests receive Started, then release individual
// attempts in any order with Reply or Forward, at most once per attempt. The
// reply is buffered, so releasing an already-cancelled attempt never blocks.
// Context is the actual provider context, allowing cancellation ownership checks.
// Prev is the state an IncrementalBarrier attempt continues; nil for Load.
type LoadAttempt struct {
	Ref     model.SessionRef
	Context context.Context
	Prev    provider.TranscriptState
	reply   chan LoadReply
}

// LoadReply is the result a test supplies for one LoadAttempt.
type LoadReply struct {
	Result  provider.IncrementalResult
	Err     error
	forward bool
}

// Reply completes the attempt with tr and err without calling the wrapped
// provider. An incremental attempt publishes tr as a stateless full rebuild.
func (a *LoadAttempt) Reply(tr *model.Transcript, err error) {
	a.reply <- LoadReply{Result: provider.IncrementalResult{Transcript: tr}, Err: err}
}

// ReplyIncremental completes an incremental attempt with result and err.
func (a *LoadAttempt) ReplyIncremental(result provider.IncrementalResult, err error) {
	a.reply <- LoadReply{Result: result, Err: err}
}

// Forward releases the attempt to the embedded provider using the attempt context.
func (a *LoadAttempt) Forward() { a.reply <- LoadReply{forward: true} }

// Barrier embeds a provider for non-load methods. Each Load announces itself and
// waits for an explicit reply or cancellation; there are no sleep-based gates.
// Do not change IgnoreCancellation while loads run. Closing Stop releases all
// blocked loads, including attempts waiting to announce themselves.
type Barrier struct {
	provider.Provider
	Started            chan *LoadAttempt
	Stop               chan struct{}
	IgnoreCancellation bool
	Calls              atomic.Int64
	once               sync.Once
}

// NewBarrier wraps p. capacity bounds how many announced attempts may be queued
// before a Load waits for the test to receive from Started.
func NewBarrier(p provider.Provider, capacity int) *Barrier {
	return &Barrier{Provider: p, Started: make(chan *LoadAttempt, capacity), Stop: make(chan struct{})}
}

// Close releases every blocked load with context.Canceled. It is idempotent.
func (b *Barrier) Close() { b.once.Do(func() { close(b.Stop) }) }

// Load counts the call, announces it on Started and waits for its release.
func (b *Barrier) Load(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	result, err := b.wait(ctx, ref, nil, false)
	return result.Transcript, err
}

func (b *Barrier) wait(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState, incremental bool) (provider.IncrementalResult, error) {
	b.Calls.Add(1)
	a := &LoadAttempt{Ref: ref, Context: ctx, Prev: prev, reply: make(chan LoadReply, 1)}
	cancel := ctx.Done()
	if b.IgnoreCancellation {
		cancel = nil
	}
	select {
	case b.Started <- a:
	case <-cancel:
		return provider.IncrementalResult{}, ctx.Err()
	case <-b.Stop:
		return provider.IncrementalResult{}, context.Canceled
	}
	select {
	case r := <-a.reply:
		if !r.forward {
			return r.Result, r.Err
		}
		if incremental {
			return provider.LoadIncremental(ctx, b.Provider, ref, prev)
		}
		tr, err := b.Provider.Load(ctx, ref)
		return provider.IncrementalResult{Transcript: tr}, err
	case <-cancel:
		return provider.IncrementalResult{}, ctx.Err()
	case <-b.Stop:
		return provider.IncrementalResult{}, context.Canceled
	}
}

// IncrementalBarrier is a Barrier that also implements provider.IncrementalLoader.
// Each LoadIncremental announces its Prev; Forward continues it with the wrapped
// provider, falling back to a full rebuild as provider.LoadIncremental does.
type IncrementalBarrier struct{ *Barrier }

// NewIncrementalBarrier wraps p like NewBarrier.
func NewIncrementalBarrier(p provider.Provider, capacity int) IncrementalBarrier {
	return IncrementalBarrier{NewBarrier(p, capacity)}
}

// LoadIncremental counts the call, announces it with prev and waits for its release.
func (b IncrementalBarrier) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	return b.wait(ctx, ref, prev, true)
}

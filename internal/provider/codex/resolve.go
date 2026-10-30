package codex

import (
	"context"
	"sync/atomic"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// resolverCap bounds retained scan checkpoints and canonical metadata. Larger
// catalogs are resolved by a cold scan on every load, as before IT-06.
var resolverCap int64 = 32 << 20

// resolver keeps the last private Scan state so load resolution only reads
// rollouts that changed since the previous resolution. Each scan reports the
// full canonical set, so a resolution equals a cold Scan's Changed metadata
// rather than a fold of deltas.
type resolver struct {
	lock    chan struct{} // a mutex that waiting callers can abandon
	started atomic.Uint64 // scans begun
	// Guarded by lock. fresh is the sequence of the scan that produced canonical.
	fresh     uint64
	state     provider.ScanState
	canonical map[model.SessionRef]model.SessionMeta
	retained  int64
}

func newResolver() *resolver {
	return &resolver{lock: make(chan struct{}, 1)}
}

// resolve returns the canonical metadata for ref from a scan that began after
// the call: concurrent callers coalesce on the next scan rather than each
// scanning. ok is false when the session does not exist.
func (r *resolver) resolve(ctx context.Context, p *Provider, ref model.SessionRef) (model.SessionMeta, bool, error) {
	return r.resolveAfter(ctx, p, ref, r.started.Load())
}

// resolveAfter reuses canonical metadata only from a scan numbered after
// arrived, which therefore observed every write that preceded the call.
func (r *resolver) resolveAfter(ctx context.Context, p *Provider, ref model.SessionRef, arrived uint64) (model.SessionMeta, bool, error) {
	select {
	case r.lock <- struct{}{}:
	case <-ctx.Done():
		return model.SessionMeta{}, false, ctx.Err()
	}
	defer func() { <-r.lock }()
	if r.canonical != nil && r.fresh > arrived {
		meta, ok := r.canonical[ref]
		return meta, ok, nil
	}
	seq := r.started.Add(1)
	canonical, err := r.refresh(ctx, p)
	if err != nil {
		return model.SessionMeta{}, false, err
	}
	if r.canonical != nil {
		r.fresh = seq
	}
	meta, ok := canonical[ref]
	return meta, ok, nil
}

// refresh replaces the retained state only after a successful scan. A failed
// warm scan, e.g. a previously scanned rollout that became unreadable, falls
// back to a cold scan; cancellation leaves the previous state intact.
func (r *resolver) refresh(ctx context.Context, p *Provider) (map[model.SessionRef]model.SessionMeta, error) {
	warm := r.state.Sources != nil
	result, byID, err := p.scan(ctx, r.state)
	if err != nil && warm {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		r.drop()
		result, byID, err = p.scan(ctx, provider.ScanState{})
	}
	if err != nil {
		return nil, err
	}
	canonical := make(map[model.SessionRef]model.SessionMeta, len(byID))
	for _, src := range byID {
		canonical[src.meta.Ref] = src.meta
	}
	retained := retainedResolution(result.State, canonical)
	if retained > resolverCap {
		r.drop()
		return canonical, nil
	}
	r.state, r.canonical, r.retained = result.State, canonical, retained
	return canonical, nil
}

func (r *resolver) drop() {
	r.state, r.canonical, r.retained, r.fresh = provider.ScanState{}, nil, 0, 0
}

// retainedResolution estimates checkpoint and metadata storage, including
// approximate map entry overhead.
func retainedResolution(state provider.ScanState, canonical map[model.SessionRef]model.SessionMeta) int64 {
	var n int64
	for path, source := range state.Sources {
		n += int64(len(path)) + int64(cap(source.Checkpoint)) + int64(unsafe.Sizeof(source)) + 48
	}
	for _, meta := range canonical {
		n += model.TranscriptBytes(&model.Transcript{Meta: meta}) + 48
	}
	return n
}

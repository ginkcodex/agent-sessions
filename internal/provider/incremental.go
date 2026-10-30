package provider

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/model"
)

// TranscriptState is opaque provider-owned parser state. Callers hand it back at
// most once, including on failure or cancellation. RetainedBytes estimates state
// storage in addition to the published transcript, without counting it twice.
// Providers return nil state when their retained-state cap would be exceeded.
type TranscriptState interface {
	RetainedBytes() int64
}

// IncrementalResult publishes an immutable transcript and optionally its resume
// state. All nested slices, pointers and raw input bytes in Transcript must stay
// unchanged through future loads. FirstChanged is a conservative earliest message
// difference from prev's snapshot; append-only may use the old message count.
// Rebuilds use zero. Metadata must match a cold Load too, even with no message
// changes. Resumed reports continuation rather than a rebuild.
type IncrementalResult struct {
	Transcript   *model.Transcript
	State        TranscriptState
	FirstChanged int
	Resumed      bool
}

// IncrementalLoader optionally continues a transcript parser. prev is consumed
// even on error and must never be reused. Canonical source resolution and current
// publish-time metadata must be validated on every call; state authorizes no path.
// A resumed result must equal a cold Load of the same source snapshot. Only complete
// lines may be committed: an unterminated tail is applied to a disposable fork.
type IncrementalLoader interface {
	LoadIncremental(context.Context, model.SessionRef, TranscriptState) (IncrementalResult, error)
}

// LoadIncremental uses the optional loader, or publishes Load as a full rebuild.
func LoadIncremental(ctx context.Context, p Provider, ref model.SessionRef, prev TranscriptState) (IncrementalResult, error) {
	if err := ctx.Err(); err != nil {
		return IncrementalResult{}, err
	}
	if loader, ok := p.(IncrementalLoader); ok {
		return loader.LoadIncremental(ctx, ref, prev)
	}
	tr, err := p.Load(ctx, ref)
	if err != nil {
		return IncrementalResult{}, err
	}
	return IncrementalResult{Transcript: tr}, nil
}

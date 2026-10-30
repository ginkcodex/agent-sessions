package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/jsonl"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

// stateCap bounds parser state retained in addition to the published
// transcript. Larger states are dropped and the next load rebuilds.
var stateCap int64 = 64 << 20

// loadState is the committed parse of every complete line through offset.
type loadState struct {
	path   string
	mark   provider.ContinuityMark
	offset int64
	lines  int // physical lines before offset, the diagnostics line base
	l      *rolloutLoader
	// The last publication: the earliest message only its discarded tail fork
	// changed, and the pending calls it finalized as unknown.
	tailFirst int
	unknown   map[transcriptbuild.ToolLoc]bool
	retained  int64
}

func (s *loadState) RetainedBytes() int64 { return s.retained }

// fork copies parser state for a disposable best-effort tail.
func (l *rolloutLoader) fork() *rolloutLoader {
	f := *l
	f.store = *l.store.Fork()
	f.tools = maps.Clone(l.tools)
	f.assistant = maps.Clone(l.assistant)
	return &f
}

// LoadIncremental continues the previous parse when the canonical rollout is the
// same append-only file, reading only bytes after its last complete line.
func (p *Provider) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	if work := provider.LoadWorkFromContext(ctx); work != nil {
		work.Calls.Add(1)
	}
	// Resolution is repeated on every call; state never authorizes a path.
	meta, err := p.rolloutFor(ctx, ref)
	if err != nil {
		return provider.IncrementalResult{}, err
	}
	result, _, err := p.loadRolloutIncremental(ctx, meta, prev)
	return result, err
}

// loadRolloutIncremental also returns the pass's diagnostics, which only tests
// use; scanning owns the reported diagnostics.
func (p *Provider) loadRolloutIncremental(ctx context.Context, meta model.SessionMeta, prev provider.TranscriptState) (provider.IncrementalResult, *provider.Diagnostics, error) {
	path := meta.SourcePath
	f, err := provider.OpenLoadFile(ctx, path)
	if err != nil {
		return provider.IncrementalResult{}, nil, fmt.Errorf("codex: load rollout %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	// prev is consumed here: committed state is advanced in place, and the
	// builder copies anything an earlier snapshot can still see. A different
	// canonical path, e.g. a newly selected duplicate, always rebuilds.
	st, _ := prev.(*loadState)
	resumed := false
	if st != nil && st.path == path {
		if resumed, err = st.mark.ValidateFile(ctx, f); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return provider.IncrementalResult{}, nil, ctxErr
			}
			return provider.IncrementalResult{}, nil, fmt.Errorf("codex: load rollout %q: %w", path, err)
		}
	}
	if !resumed {
		st = &loadState{path: path, l: &rolloutLoader{path: path, current: -1}}
	}
	info, err := f.Stat()
	if err != nil {
		return provider.IncrementalResult{}, nil, fmt.Errorf("codex: load rollout %q: %w", path, err)
	}
	r, err := provider.LoadReader(ctx, f, st.offset)
	if err != nil {
		return provider.IncrementalResult{}, nil, fmt.Errorf("codex: load rollout %q: %w", path, err)
	}

	l := st.l
	l.diag = &provider.Diagnostics{}
	var tail []byte
	for n := 0; ; n++ {
		if n%128 == 0 {
			if err := ctx.Err(); err != nil {
				return provider.IncrementalResult{}, nil, err
			}
		}
		line, start, err := r.Next()
		if errors.Is(err, io.EOF) {
			// A valid but incomplete final line can be shown best-effort from a
			// fork. Its content must never update the scan metadata or state.
			if trailing := r.Trailing(); json.Valid(trailing) {
				tail = trailing
			}
			break
		}
		if errors.Is(err, jsonl.ErrLineTooLong) {
			l.diag.ParseErrors++
			l.diag.Warn(path, st.lines+r.LineNo(), "JSONL record exceeds maximum line length")
			continue
		}
		if err != nil {
			return provider.IncrementalResult{}, nil, fmt.Errorf("codex: read rollout %q: %w", path, err)
		}
		l.record(line, start, st.lines+r.LineNo())
	}
	if err := ctx.Err(); err != nil {
		return provider.IncrementalResult{}, nil, err
	}
	st.lines += r.LineNo()
	st.offset = r.Offset()
	keep := true
	if st.mark, err = provider.MarkContinuity(ctx, f, st.offset); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return provider.IncrementalResult{}, nil, ctxErr
		}
		keep = false // e.g. truncated while reading: publish, but rebuild next time
	}
	if after, err := f.Stat(); err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		keep = false // do not certify a parse spanning a concurrent write
	}
	committed, parserFirst := l.store.Freeze()

	pub := &l.store
	tailL := l
	tailFirst := len(committed)
	if tail != nil {
		tailL = l.fork()
		pub = &tailL.store
		tailL.record(tail, st.offset, st.lines+1)
		tailFirst = pub.Changed()
	}

	// There is no Codex live detector yet. Unpaired calls have unknown outcome;
	// pending is reserved for a positively identified live session.
	unknown := pendingUnknown(tailL, meta.Live)
	if len(unknown) > 0 && pub == &l.store {
		pub = l.store.Fork()
	}
	for loc := range unknown {
		pub.Tool(loc).Status = model.ToolUnknown
	}
	messages, _ := pub.Freeze()
	if err := ctx.Err(); err != nil {
		return provider.IncrementalResult{}, nil, err
	}

	result := provider.IncrementalResult{
		Transcript: &model.Transcript{Meta: meta, Messages: messages},
		Resumed:    resumed,
	}
	if resumed {
		first := min(parserFirst, st.tailFirst, tailFirst, unknownDiff(st.unknown, unknown))
		result.FirstChanged = min(first, len(messages))
	}
	st.tailFirst, st.unknown = tailFirst, unknown
	diag := l.diag
	l.diag = nil // diagnostics belong to this pass, not resumable parser state
	st.retained = st.retainedBytes(committed, messages)
	if keep && st.retained <= stateCap {
		result.State = st
	}
	return result, diag, nil
}

// pendingUnknown is the publish-time finalization of unanswered calls.
func pendingUnknown(l *rolloutLoader, live bool) map[transcriptbuild.ToolLoc]bool {
	if live {
		return nil
	}
	var unknown map[transcriptbuild.ToolLoc]bool
	for _, loc := range l.tools {
		if l.store.At(loc.Message).Parts[loc.Part].Tool.Status != model.ToolPending {
			continue
		}
		if unknown == nil {
			unknown = make(map[transcriptbuild.ToolLoc]bool)
		}
		unknown[loc] = true
	}
	return unknown
}

// unknownDiff is the earliest message whose finalization changed.
func unknownDiff(old, cur map[transcriptbuild.ToolLoc]bool) int {
	first := math.MaxInt
	for loc := range old {
		if !cur[loc] {
			first = min(first, loc.Message)
		}
	}
	for loc := range cur {
		if !old[loc] {
			first = min(first, loc.Message)
		}
	}
	return first
}

// retainedBytes estimates storage the state holds beyond the published
// snapshot: parser bookkeeping, plus the committed array and messages when a
// tail or finalization fork stopped sharing them.
func (s *loadState) retainedBytes(committed, published []model.Message) int64 {
	n := s.l.store.Overhead() + int64(unsafe.Sizeof(*s)) + int64(unsafe.Sizeof(*s.l))
	// The path and model strings are shared with published metadata and messages.
	for id := range s.l.tools {
		n += int64(len(id)) + 48
	}
	for id := range s.l.assistant {
		n += int64(len(id)) + 32
	}
	n += int64(len(s.unknown)) * 32
	return n + transcriptbuild.Unshared(committed, published)
}

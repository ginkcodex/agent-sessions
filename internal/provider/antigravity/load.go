package antigravity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/jsonl"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

// stateCap bounds parser state retained in addition to the published
// transcript. Larger states are dropped and the next load rebuilds.
var stateCap int64 = 64 << 20

// loadState is the committed parse of every complete line through offset:
// the messages, and the scan's checkpoint so the meta equals the scan's.
type loadState struct {
	path   string
	mark   provider.ContinuityMark
	offset int64
	cp     checkpoint
	l      *loader
	// tailFirst is the earliest message only the last discarded tail fork
	// changed.
	tailFirst int
	retained  int64
}

func (s *loadState) RetainedBytes() int64 { return s.retained }

// resolve finds ref's transcript the way discover does. The ID must name one
// brain/ entry; neither a hint nor load state ever authorizes a path.
func (p *Provider) resolve(ref model.SessionRef) (source, error) {
	id := ref.ID
	if ref.Agent != model.AgentAntigravity || id == "" || strings.HasPrefix(id, ".") || strings.ContainsAny(id, "/\\:\x00") {
		return source{}, fmt.Errorf("antigravity: invalid session reference (%q, %q)", ref.Agent, id)
	}
	src, ok := p.transcript(id, &provider.Diagnostics{})
	if !ok {
		return source{}, fmt.Errorf("antigravity: transcript for session %q not found", id)
	}
	return src, nil
}

// exists reports whether session id has a readable transcript.
func (p *Provider) exists(id string) bool {
	_, err := p.resolve(model.SessionRef{Agent: model.AgentAntigravity, ID: id})
	return err == nil
}

// Load reads a session's transcript.
func (p *Provider) Load(ctx context.Context, ref model.SessionRef) (*model.Transcript, error) {
	result, err := p.LoadIncremental(ctx, ref, nil)
	return result.Transcript, err
}

// LoadIncremental continues the previous parse when the transcript is the
// same append-only file, reading only bytes after its last complete line.
func (p *Provider) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	if work := provider.LoadWorkFromContext(ctx); work != nil {
		work.Calls.Add(1)
	}
	// Resolution is repeated on every call; state never authorizes a path.
	src, err := p.resolve(ref)
	if err != nil {
		return provider.IncrementalResult{}, err
	}
	fail := func(err error) (provider.IncrementalResult, error) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return provider.IncrementalResult{}, ctxErr
		}
		return provider.IncrementalResult{}, fmt.Errorf("antigravity: load %s: %w", src.Path, err)
	}
	f, err := provider.OpenLoadFile(ctx, src.Path)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = f.Close() }()

	// prev is consumed here: committed state is advanced in place, and the
	// builder copies anything an earlier snapshot can still see.
	st, _ := prev.(*loadState)
	resumed := false
	if st != nil && st.path == src.Path {
		if resumed, err = st.mark.ValidateFile(ctx, f); err != nil {
			return fail(err)
		}
	}
	if !resumed {
		st = &loadState{
			path: src.Path,
			cp:   checkpoint{Version: checkpointVersion, ID: src.ID, path: src.Path},
			l:    &loader{exists: p.exists},
		}
	}
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	r, err := provider.LoadReader(ctx, f, st.offset)
	if err != nil {
		return fail(err)
	}

	var diag provider.Diagnostics // the scan reports them
	base := st.cp.LinesRead
	var tail []byte
	for n := 0; ; n++ {
		if n%128 == 0 {
			if err := ctx.Err(); err != nil {
				return provider.IncrementalResult{}, err
			}
		}
		line, start, err := r.Next()
		if errors.Is(err, io.EOF) {
			// A valid but incomplete final line is shown from a fork. It
			// never updates the meta or the state.
			if trailing := r.Trailing(); json.Valid(trailing) {
				tail = trailing
			}
			break
		}
		if errors.Is(err, jsonl.ErrLineTooLong) {
			continue
		}
		if err != nil {
			return fail(err)
		}
		st.cp.lineNo = base + r.LineNo()
		st.cp.consume(line, &diag)
		st.l.step(line, start)
	}
	if err := ctx.Err(); err != nil {
		return provider.IncrementalResult{}, err
	}
	st.cp.LinesRead = base + r.LineNo()
	st.offset = r.Offset()
	keep := true
	if st.mark, err = provider.MarkContinuity(ctx, f, st.offset); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return provider.IncrementalResult{}, ctxErr
		}
		keep = false // e.g. truncated while reading: publish, but rebuild next time
	}
	if after, err := f.Stat(); err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		keep = false // do not certify a parse spanning a concurrent write
	}
	committed, parserFirst := st.l.store.Freeze()
	messages, tailFirst := committed, len(committed)
	if tail != nil {
		fork := st.l.fork()
		fork.step(tail, st.offset)
		tailFirst = fork.store.Changed()
		messages, _ = fork.store.Freeze()
	}

	// shortcut: reads the whole index for one row; it holds one small row per
	// session. Query by ID if stores grow to thousands of sessions.
	rows, err := p.readSummaries(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return provider.IncrementalResult{}, ctxErr
		}
		rows = nil // keep the last row read, as Scan does
	}
	p.settle(&st.cp, rows, info.ModTime())
	meta := p.sessionMeta(&st.cp, src, info.ModTime())
	if err := ctx.Err(); err != nil {
		return provider.IncrementalResult{}, err
	}

	result := provider.IncrementalResult{
		Transcript: &model.Transcript{Meta: meta, Messages: messages},
		Resumed:    resumed,
	}
	if resumed {
		result.FirstChanged = min(parserFirst, st.tailFirst, tailFirst, len(messages))
	}
	st.tailFirst = tailFirst
	st.retained = st.retainedBytes(committed, messages)
	if keep && st.retained <= stateCap {
		result.State = st
	}
	return result, nil
}

// retainedBytes estimates storage the state holds beyond the published
// snapshot: parser bookkeeping, plus the committed messages a tail fork
// stopped sharing.
func (s *loadState) retainedBytes(committed, published []model.Message) int64 {
	n := s.l.store.Overhead() + int64(unsafe.Sizeof(*s)) + int64(unsafe.Sizeof(*s.l))
	n += int64(len(s.l.waiting)) * 32
	n += int64(len(s.cp.FirstPrompt) + len(s.cp.Model))
	if r := s.cp.Row; r != nil {
		n += int64(unsafe.Sizeof(*r)) + int64(len(r.Title)+len(r.Preview)+len(r.Status)+len(r.CWD)+len(r.AgentName)+len(r.ParentID))
	}
	return n + transcriptbuild.Unshared(committed, published)
}

// Blob returns a tool's full output. Keys are rec:<lineStart>:<stepIndex>,
// naming the GENERIC result step; nothing else is ever read.
func (p *Provider) Blob(ctx context.Context, ref model.SessionRef, key string) ([]byte, error) {
	offText, indexText, ok := strings.Cut(strings.TrimPrefix(key, "rec:"), ":")
	offset, err1 := strconv.ParseInt(offText, 10, 64)
	index, err2 := strconv.ParseInt(indexText, 10, 64)
	if !strings.HasPrefix(key, "rec:") || !ok || err1 != nil || err2 != nil || offset < 0 {
		return nil, fmt.Errorf("antigravity: Blob: malformed key %q", key)
	}
	src, err := p.resolve(ref)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := jsonl.Open(src.Path, offset)
	if err != nil {
		return nil, fmt.Errorf("antigravity: Blob %q: %w", key, err)
	}
	defer func() { _ = r.Close() }()
	line, _, err := r.Next()
	if errors.Is(err, io.EOF) && json.Valid(r.Trailing()) {
		line, err = r.Trailing(), nil // the valid partial last line Load shows
	}
	if err != nil {
		return nil, fmt.Errorf("antigravity: Blob %q: read step: %w", key, err)
	}
	var s step
	if json.Unmarshal(line, &s) != nil || s.Type != stepGeneric || s.Index != index {
		return nil, fmt.Errorf("antigravity: Blob %q: not a tool result step", key)
	}
	return []byte(resultText(s)), nil
}

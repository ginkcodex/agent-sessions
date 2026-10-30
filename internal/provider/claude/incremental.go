package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"reflect"
	"slices"
	"time"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/jsonl"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/pathutil"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/transcriptbuild"
)

// stateCap bounds parser state retained in addition to the published
// transcript. Larger states are dropped and the next load rebuilds.
var stateCap int64 = 64 << 20

// toolEffect is publish-time tool finalization, never committed parser state.
type toolEffect struct {
	unknown bool   // pending call of an ended session
	child   string // linked subagent session ID
}

// loadState is the committed parse of every complete line through offset.
type loadState struct {
	path   string
	mark   provider.ContinuityMark
	offset int64
	lines  int // physical lines before offset, the diagnostics line base
	cp     checkpoint
	l      *loader
	// The last publication: the earliest message only its discarded tail fork
	// changed, and the finalization applied over parser state.
	tailFirst int
	effects   map[transcriptbuild.ToolLoc]toolEffect
	retained  int64
}

func (s *loadState) RetainedBytes() int64 { return s.retained }

func newLoadState(s source) *loadState {
	return &loadState{
		path: s.Path,
		cp: checkpoint{
			Meta: model.SessionMeta{Ref: sessionRef(s), SourcePath: s.Path, ParentID: s.ParentID},
			path: s.Path,
		},
		l: &loader{path: s.Path, compactMsg: -1},
	}
}

// fork copies parser state for a disposable best-effort tail.
func (l *loader) fork() *loader {
	f := *l
	f.store = *l.store.Fork()
	f.tools = maps.Clone(l.tools)
	return &f
}

// LoadIncremental continues the previous parse when the source is the same
// append-only file, reading only bytes after its last complete line.
func (p *Provider) LoadIncremental(ctx context.Context, ref model.SessionRef, prev provider.TranscriptState) (provider.IncrementalResult, error) {
	if work := provider.LoadWorkFromContext(ctx); work != nil {
		work.Calls.Add(1)
	}
	// One discovery pass both resolves the source and finds subagent sidecars.
	sources, err := p.discover(ctx)
	if err != nil {
		return provider.IncrementalResult{}, err
	}
	s, err := findSource(sources, ref)
	if err != nil {
		return provider.IncrementalResult{}, err
	}
	result, _, err := p.loadIncrementalFile(ctx, s, sources, prev)
	return result, err
}

// loadIncrementalFile also returns the pass's diagnostics, which only tests use;
// scanning owns the reported diagnostics.
func (p *Provider) loadIncrementalFile(ctx context.Context, s source, sources []source, prev provider.TranscriptState) (provider.IncrementalResult, *provider.Diagnostics, error) {
	fail := func(err error) (provider.IncrementalResult, *provider.Diagnostics, error) {
		return provider.IncrementalResult{}, nil, fmt.Errorf("claude: load %s: %w", s.Path, err)
	}
	f, err := provider.OpenLoadFile(ctx, s.Path)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = f.Close() }()

	// prev is consumed here: committed state is advanced in place, and the
	// builder copies anything an earlier snapshot can still see.
	st, _ := prev.(*loadState)
	resumed := false
	if st != nil && st.path == s.Path {
		if resumed, err = st.mark.ValidateFile(ctx, f); err != nil {
			return fail(err)
		}
	}
	if !resumed {
		st = newLoadState(s)
	}
	info, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	r, err := provider.LoadReader(ctx, f, st.offset)
	if err != nil {
		return fail(err)
	}

	l := st.l
	l.diag = &provider.Diagnostics{}
	// The checkpoint keeps its own diagnostics: unknown-type counting in
	// consume would otherwise double every entry the loader also records.
	scanDiag := &provider.Diagnostics{}
	var tail []byte
	for n := 0; ; n++ {
		if n%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
		}
		line, start, err := r.Next()
		if errors.Is(err, io.EOF) {
			// A live process may leave a valid JSON object without a newline.
			// It is shown best-effort from a fork and never committed.
			if trailing := r.Trailing(); json.Valid(trailing) {
				tail = trailing
			}
			break
		}
		if errors.Is(err, jsonl.ErrLineTooLong) {
			l.diag.Warn(s.Path, st.lines+r.LineNo(), "line too long, skipped")
			continue
		}
		if err != nil {
			return provider.IncrementalResult{}, nil, fmt.Errorf("claude: read %s: %w", s.Path, err)
		}
		st.cp.lineNo = st.lines + r.LineNo()
		st.cp.consume(line, scanDiag)
		if json.Valid(line) {
			l.record(line, start, st.cp.lineNo)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	st.lines += r.LineNo()
	st.offset = r.Offset()
	keep := true
	after, statErr := f.Stat()
	if statErr != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		keep = false // do not certify a parse spanning a concurrent write
	}
	if st.mark, err = provider.MarkContinuity(ctx, f, st.offset); err != nil {
		if ctx.Err() != nil {
			return fail(err)
		}
		keep = false // e.g. truncated while reading: publish, but rebuild next time
	}
	if after, err := f.Stat(); err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
		keep = false
	}
	committed, parserFirst := l.store.Freeze()

	pubCp := st.cp
	pub := &l.store
	tailL := l
	tailFirst := len(committed)
	if tail != nil {
		tailL = l.fork()
		pub = &tailL.store
		pubCp.PendingTools = slices.Clone(pubCp.PendingTools)
		pubCp.lineNo = st.lines + 1
		pubCp.consume(tail, l.diag)
		tailL.record(tail, st.offset, pubCp.lineNo)
		tailFirst = pub.Changed()
	}

	meta, ended := p.publishMeta(ctx, s, pubCp, info.ModTime())
	effects := p.toolEffects(s, sources, tailL, ended)
	if len(effects) > 0 && pub == &l.store {
		pub = l.store.Fork()
	}
	for loc, e := range effects {
		tc := pub.Tool(loc)
		if e.unknown {
			tc.Status = model.ToolUnknown
		}
		if e.child != "" {
			tc.Child = &model.SessionRef{Agent: model.AgentClaude, ID: e.child}
		}
	}
	messages, _ := pub.Freeze()
	if err := ctx.Err(); err != nil {
		return fail(err)
	}

	result := provider.IncrementalResult{
		Transcript: &model.Transcript{Meta: meta, Messages: messages},
		Resumed:    resumed,
	}
	if resumed {
		first := min(parserFirst, st.tailFirst, tailFirst, effectDiff(st.effects, effects))
		result.FirstChanged = min(first, len(messages))
	}
	st.tailFirst, st.effects = tailFirst, effects
	diag := l.diag
	l.diag = nil // diagnostics belong to this pass, not resumable parser state
	st.retained = st.retainedBytes(committed, messages, meta)
	if keep && st.retained <= stateCap {
		result.State = st
	}
	return result, diag, nil
}

// publishMeta finalizes metadata from current sidecar, live and CWD state.
func (p *Provider) publishMeta(ctx context.Context, s source, cp checkpoint, modTime time.Time) (model.SessionMeta, bool) {
	if s.MetaPath != "" {
		if sub, err := readSubMeta(s.MetaPath); err == nil {
			cp.Meta.AgentName = sub.AgentType
			cp.Meta.ParentToolCallID = sub.ToolUseID
			cp.SubagentDescription = sub.Description
		}
	}
	meta := cp.finalize(modTime)
	meta.Ref = sessionRef(s)
	meta.SourcePath = s.Path
	meta.ParentID = s.ParentID

	liveID := meta.Ref.ID
	if s.ParentID != "" {
		liveID = s.ParentID
	}
	var liveStatus string
	if live, err := p.Live(ctx); err == nil {
		if info, ok := live[liveID]; ok {
			liveStatus = info.Status
			if s.ParentID == "" {
				meta.Live = true
				meta.LiveStatus = info.Status
			}
		}
	}

	if meta.CWD != "" {
		norm := pathutil.NormalizeDir(meta.CWD)
		meta.CWD = norm
		meta.CWDMissing = !pathutil.Exists(norm)
		if repo, ok := p.git.Resolve(norm); ok {
			meta.RepoRoot = repo.MainRoot
		}
	}
	return meta, liveStatus == ""
}

// toolEffects decides finalization for parser tools. Unanswered calls are
// pending while the session is live, otherwise their outcome is unknown. Main
// sessions link calls that spawned subagents, the first sidecar winning.
func (p *Provider) toolEffects(s source, sources []source, l *loader, ended bool) map[transcriptbuild.ToolLoc]toolEffect {
	var effects map[transcriptbuild.ToolLoc]toolEffect
	set := func(loc transcriptbuild.ToolLoc, e toolEffect) {
		if effects == nil {
			effects = make(map[transcriptbuild.ToolLoc]toolEffect)
		}
		effects[loc] = e
	}
	if ended {
		for _, loc := range l.tools {
			if l.store.At(loc.Message).Parts[loc.Part].Tool.Status == model.ToolPending {
				set(loc, toolEffect{unknown: true})
			}
		}
	}
	if s.ParentID != "" {
		return effects
	}
	parentID := sessionRef(s).ID
	for _, sub := range sources {
		if sub.ParentID != parentID {
			continue
		}
		m, err := readSubMeta(sub.MetaPath)
		if err != nil || m.ToolUseID == "" {
			continue
		}
		loc, ok := l.tools[m.ToolUseID]
		if !ok || effects[loc].child != "" {
			continue
		}
		e := effects[loc]
		e.child = sub.ParentID + "/agent-" + sub.AgentID
		set(loc, e)
	}
	return effects
}

// effectDiff is the earliest message whose finalization changed.
func effectDiff(old, cur map[transcriptbuild.ToolLoc]toolEffect) int {
	first := math.MaxInt
	for loc, e := range old {
		if cur[loc] != e {
			first = min(first, loc.Message)
		}
	}
	for loc, e := range cur {
		if old[loc] != e {
			first = min(first, loc.Message)
		}
	}
	return first
}

// retainedBytes estimates storage the state holds beyond the published
// snapshot: parser bookkeeping, plus the committed array and messages when a
// tail or finalization fork stopped sharing them.
func (s *loadState) retainedBytes(committed, published []model.Message, meta model.SessionMeta) int64 {
	n := s.l.store.Overhead() + int64(unsafe.Sizeof(*s)) + int64(unsafe.Sizeof(*s.l))
	// Count checkpoint strings not already exposed in published metadata.
	shared := make(map[*byte]bool)
	stringsIn(reflect.ValueOf(meta), func(v string) { shared[unsafe.StringData(v)] = true })
	stringsIn(reflect.ValueOf(s.cp), func(v string) {
		if !shared[unsafe.StringData(v)] {
			n += int64(len(v))
		}
	})
	for _, id := range s.cp.PendingTools {
		n += int64(len(id)) + 16
	}
	for id := range s.l.tools {
		n += int64(len(id)) + 48
	}
	n += int64(len(s.effects)) * 64
	for _, e := range s.effects {
		n += int64(len(e.child))
	}
	return n + transcriptbuild.Unshared(committed, published)
}

// stringsIn walks value fields only; checkpoint mutable slices are charged above.
func stringsIn(v reflect.Value, visit func(string)) {
	switch v.Kind() {
	case reflect.String:
		visit(v.String())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				stringsIn(v.Field(i), visit)
			}
		}
	}
}

package engine

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
)

// transcriptLoadLimit bounds provider I/O, including superseded loads that have
// not returned yet. Waiting for a slot is cancellable and holds no cache lock.
const transcriptLoadLimit = 4

// transcriptByteBudget bounds the estimated bytes of resident entries. IT-01
// measured the estimate at ~95–99% of retained heap: two 100 MB snapshots fit,
// four do not. The most recently used entry is exempt.
const transcriptByteBudget = 256 << 20

// transcriptBackgroundBudget caps the one entry kept for full-text indexing of
// sessions not open in the foreground; it fits a 100 MB session. A larger one
// is not kept, and the next index load parses cold.
const transcriptBackgroundBudget = 128 << 20

// transcriptHistory is how many publishes of an entry changedSince can span.
const transcriptHistory = 16

// transcriptLoader loads a transcript, continuing prev when it is non-nil.
type transcriptLoader func(ctx context.Context, prev provider.TranscriptState) (provider.IncrementalResult, error)

// transcriptCache coordinates the LRU and current flights. A flight's pointer
// is its unique token: invalidation detaches it without denying existing
// readers their snapshot. Only the current flight may publish or remove itself.
//
// Invalidation keeps an entry as stale: it is never returned, but its provider
// state lets the next load resume instead of parsing cold. Provider state is
// consumed by one load, so each key has at most one owner flight, which holds
// the state or will produce it. A new flight resumes from its detached
// predecessor's result, waiting for it without holding a load slot.
//
// Entries are bounded by count and by estimated bytes. Each publish gets an
// opaque revision, epoch.generation.sequence. A stale entry keeps its
// generation, so a resumed publish continues its sequence and records the
// earliest message it may have changed; any other publish starts a generation.
//
// Index loads never serve a cached snapshot or join an earlier flight: they
// resume to the source's current end. They advance a foreground entry without
// changing its recency; otherwise their resume point goes to a single
// background slot outside the LRU, which a foreground load promotes.
type transcriptCache struct {
	mu     sync.Mutex
	lru    *LRU[string, *transcriptEntry]
	active map[string]*transcriptLoad
	owner  map[string]*transcriptLoad
	owned  map[*transcriptLoad]struct{}
	ctx    context.Context
	cancel context.CancelFunc
	slots  chan struct{}
	budget int64
	epoch  string // new on clear and restart, so no older revision matches
	gens   uint64
	// The background slot: bgKey's resume point, never served.
	bgKey    string
	bg       resumePoint
	bgBudget int64
}

// resumePoint is what a load continues: provider state, the snapshot it was
// produced with and that snapshot's estimate, and the earliest message that
// may differ from the entry's latest revision. It moves whole between an
// entry, the flight resuming it and that flight's successor.
type resumePoint struct {
	state   provider.TranscriptState
	tr      *model.Transcript
	bytes   int64
	changed int // math.MaxInt: tr is the latest revision's snapshot
}

// next is the resume point of result, a load that continued rp's state.
func (rp resumePoint) next(result provider.IncrementalResult) resumePoint {
	next := resumePoint{state: result.State, tr: result.Transcript}
	if rp.state != nil && result.Resumed {
		next.bytes = model.TranscriptBytesSince(rp.tr, rp.bytes, result.Transcript, result.FirstChanged)
		next.changed = min(rp.changed, result.FirstChanged)
	} else {
		next.bytes = model.TranscriptBytes(result.Transcript)
	}
	return next
}

// transcriptEntry fields are guarded by transcriptCache.mu.
type transcriptEntry struct {
	rp    resumePoint // a fresh entry serves rp.tr; a flight takes a stale one's
	stale bool
	gen   uint64 // zero until the entry is published
	seq   uint64
	// history is a ring indexed by sequence: the earliest message each of the
	// last n publishes after the generation's first may have changed.
	history [transcriptHistory]int
	n       int
}

func (e *transcriptEntry) bytes() int64 {
	n := e.rp.bytes
	if e.rp.state != nil {
		n += e.rp.state.RetainedBytes()
	}
	return n
}

type transcriptLoad struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int // readers, plus a successor waiting on this flight's result
	// foreground publishes to the LRU rather than the background slot; touch
	// also makes the entry most recently used. Both are set by a foreground
	// reader, and foreground by an index load of a foreground key.
	foreground, touch bool
	// pred is the detached flight whose result this flight resumes. Guarded by
	// the cache lock: prev is the resume point this flight holds, used is set
	// once the provider has consumed it, and rp is the result for a successor.
	pred *transcriptLoad
	prev resumePoint
	used bool
	tr   *model.Transcript
	rev  string // empty unless this flight published tr
	rp   resumePoint
	err  error
}

func newTranscriptCache(capacity int, budget, bgBudget int64) *transcriptCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &transcriptCache{
		lru:      NewLRU[string, *transcriptEntry](capacity),
		active:   make(map[string]*transcriptLoad),
		owner:    make(map[string]*transcriptLoad),
		owned:    make(map[*transcriptLoad]struct{}),
		ctx:      ctx,
		cancel:   cancel,
		slots:    make(chan struct{}, transcriptLoadLimit),
		budget:   budget,
		bgBudget: bgBudget,
		epoch:    rand.Text(),
	}
}

// start gives a restarted engine a fresh lifetime. Standalone services use a
// background lifetime; their last departing waiter still cancels orphan work.
func (c *transcriptCache) start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
	c.ctx, c.cancel = context.WithCancel(ctx)
}

func (c *transcriptCache) stopLocked() {
	c.cancel()
	for f := range c.owned {
		f.cancel()
	}
	c.clearLocked()
}

func (c *transcriptCache) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopLocked()
}

// load returns the cached snapshot or joins the current flight, with the
// snapshot's revision. A snapshot that was never published, because its
// flight was detached, has no revision. resumable says the loader can continue
// provider state; otherwise a flight never waits for a predecessor whose
// result it could not use.
func (c *transcriptCache) load(ctx context.Context, key string, resumable bool, load transcriptLoader) (*model.Transcript, string, error) {
	return c.wait(ctx, key, resumable, false, load)
}

// loadForIndex returns a snapshot no older than the source when it was
// called. It detaches the current flight, which may have read the source
// earlier, and continues its result.
func (c *transcriptCache) loadForIndex(ctx context.Context, key string, load transcriptLoader) (*model.Transcript, error) {
	tr, _, err := c.wait(ctx, key, true, true, load)
	return tr, err
}

func (c *transcriptCache) wait(ctx context.Context, key string, resumable, index bool, load transcriptLoader) (*model.Transcript, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	c.mu.Lock()
	if err := c.ctx.Err(); err != nil {
		c.mu.Unlock()
		return nil, "", err
	}
	var f *transcriptLoad
	if index {
		// A foreground entry, or a foreground flight this one detaches,
		// makes the result foreground too.
		old := c.active[key]
		_, resident := c.lru.Peek(key)
		c.invalidateLocked(key)
		f = c.flightLocked(key, resumable)
		f.foreground = resident || old != nil && old.foreground
		go c.run(key, f, load)
	} else {
		if e, ok := c.lru.Get(key); ok && !e.stale {
			rev := c.revisionLocked(e, e.seq)
			c.mu.Unlock()
			return e.rp.tr, rev, nil
		}
		if f = c.active[key]; f == nil {
			f = c.flightLocked(key, resumable)
			go c.run(key, f, load)
		}
		f.foreground, f.touch = true, true
	}
	f.waiters++
	c.mu.Unlock()
	select {
	case <-ctx.Done():
		c.mu.Lock()
		c.releaseLocked(key, f)
		c.mu.Unlock()
		return nil, "", ctx.Err()
	case <-f.ctx.Done():
		// Shutdown must release readers even if a provider ignores cancellation.
		select {
		case <-f.done:
			return f.tr, f.rev, f.err
		default:
			return nil, "", f.ctx.Err()
		}
	case <-f.done:
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		return f.tr, f.rev, f.err
	}
}

// flightLocked starts the key's new current flight and makes it the owner of
// the key's resume point: a stale entry's, a detached predecessor's, or the
// background slot's.
func (c *transcriptCache) flightLocked(key string, resumable bool) *transcriptLoad {
	workCtx, cancel := context.WithCancel(c.ctx)
	f := &transcriptLoad{ctx: workCtx, cancel: cancel, done: make(chan struct{})}
	if pred := c.owner[key]; pred != nil {
		switch {
		case pred.ctx.Err() != nil:
			// A cancelled predecessor publishes nothing, and an uncooperative
			// provider must not delay the retry: take only its unused state.
			f.prev, pred.prev = pred.prev, resumePoint{}
		case resumable:
			f.pred = pred
			pred.waiters++
		}
	} else if e, ok := c.lru.Peek(key); ok {
		f.prev, e.rp = e.rp, resumePoint{}
	} else if c.bgKey == key {
		f.prev, c.bgKey, c.bg = c.bg, "", resumePoint{}
	}
	c.active[key] = f
	c.owner[key] = f
	c.owned[f] = struct{}{}
	return f
}

// releaseLocked drops one waiter. The last one cancels unfinished work.
func (c *transcriptCache) releaseLocked(key string, f *transcriptLoad) {
	f.waiters--
	if f.waiters == 0 {
		f.cancel()
		if c.active[key] == f {
			delete(c.active, key)
		}
	}
}

func (c *transcriptCache) run(key string, f *transcriptLoad, load transcriptLoader) {
	if pred := f.pred; pred != nil {
		select {
		case <-pred.done:
		case <-f.ctx.Done():
		}
		c.mu.Lock()
		c.releaseLocked(key, pred)
		if !closed(pred.done) {
			// Cancelled first: finishing under the same lock hold returns
			// ownership to the predecessor before it can complete.
			c.finishLocked(key, f, resumePoint{}, f.ctx.Err())
			c.mu.Unlock()
			return
		}
		// Continue the predecessor's result. A failed or cancelled one leaves
		// no state, so this load is cold.
		f.prev, pred.rp = pred.rp, resumePoint{}
		f.pred = nil
		c.mu.Unlock()
	}
	var next resumePoint
	err := f.ctx.Err()
	if err == nil {
		select {
		case c.slots <- struct{}{}:
			if err = f.ctx.Err(); err == nil {
				c.mu.Lock()
				prev := f.prev
				f.prev, f.used = resumePoint{}, true
				c.mu.Unlock()
				var result provider.IncrementalResult
				if result, err = load(f.ctx, prev.state); err == nil {
					next = prev.next(result)
				}
			}
			<-c.slots
		case <-f.ctx.Done():
			err = f.ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.finishLocked(key, f, next, err)
}

// finishLocked publishes a current flight's result, or keeps the state it owns
// as stale, and releases its readers.
func (c *transcriptCache) finishLocked(key string, f *transcriptLoad, rp resumePoint, err error) {
	if err == nil {
		err = f.ctx.Err()
	}
	var tr *model.Transcript
	if err != nil {
		rp = resumePoint{}
	} else {
		tr = rp.tr
	}
	if !f.used {
		// The provider never consumed the state, so it passes on intact.
		rp = f.prev
	}
	f.prev = resumePoint{}
	current := c.active[key] == f
	if current {
		delete(c.active, key)
	}
	if c.owner[key] == f {
		delete(c.owner, key)
		_, resident := c.lru.Peek(key)
		switch {
		case f.pred != nil:
			// The predecessor still runs and owns the state again.
			c.owner[key] = f.pred
		case !f.foreground && !resident:
			c.keepBackgroundLocked(key, rp)
		case current && err == nil:
			f.rev = c.publishLocked(key, rp, f.touch)
		default:
			// Detached or failed: unpublished, but resumable.
			c.keepLocked(key, rp)
		}
		rp = resumePoint{}
	}
	// A non-owner's result goes to the successor waiting on it.
	f.tr, f.rp, f.err = tr, rp, err
	delete(c.owned, f)
	close(f.done)
	f.cancel()
}

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// publishLocked makes rp the key's fresh entry and returns its revision. A
// stale entry continues its generation; rp.changed is relative to its latest
// revision because only the key's owner moves its resume point. touch makes
// the entry most recently used; a new entry always is.
func (c *transcriptCache) publishLocked(key string, rp resumePoint, touch bool) string {
	e, ok := c.lru.Peek(key)
	switch {
	case ok && e.seq > 0:
		e.seq++
		e.history[e.seq%transcriptHistory] = rp.changed
		e.n = min(e.n+1, transcriptHistory)
	case ok:
		c.gens++
		e.gen, e.seq = c.gens, 1
	default:
		c.gens++
		e = &transcriptEntry{gen: c.gens, seq: 1}
	}
	rp.changed = math.MaxInt
	e.rp, e.stale = rp, false
	if touch || !ok {
		c.lru.Put(key, e)
	}
	c.fitLocked()
	return c.revisionLocked(e, e.seq)
}

// keepBackgroundLocked keeps an index load's resume point in the background
// slot, replacing another session's, unless it exceeds the slot's budget.
func (c *transcriptCache) keepBackgroundLocked(key string, rp resumePoint) {
	if rp.state == nil || rp.bytes+rp.state.RetainedBytes() > c.bgBudget {
		return
	}
	c.bgKey, c.bg = key, rp
}

// keepLocked returns an owner's resume point to the key's stale entry, or
// records it as a new one. Without state, a stale entry has nothing to resume
// and nothing a later publish could be compared with, so it is dropped.
func (c *transcriptCache) keepLocked(key string, rp resumePoint) {
	e, ok := c.lru.Peek(key)
	switch {
	case rp.state == nil:
		if ok && e.stale {
			c.lru.Evict(key)
		}
		return
	case !ok:
		// No revision to compare with: the next publish starts a generation.
		c.lru.Put(key, &transcriptEntry{rp: rp, stale: true})
	case e.stale:
		e.rp = rp
	}
	c.fitLocked()
}

// fitLocked evicts least recently used entries until the estimated total fits
// the budget. The most recently used entry stays even when it alone exceeds it.
func (c *transcriptCache) fitLocked() {
	var total int64
	c.lru.Range(func(_ string, e *transcriptEntry) bool {
		total += e.bytes()
		return true
	})
	for total > c.budget && c.lru.Len() > 1 {
		key, e, _ := c.lru.Oldest()
		total -= e.bytes()
		c.lru.Evict(key)
	}
}

func (c *transcriptCache) revisionLocked(e *transcriptEntry, seq uint64) string {
	return fmt.Sprintf("%s.%d.%d", c.epoch, e.gen, seq)
}

// seqLocked returns rev's sequence if it is a revision of e's generation.
func (c *transcriptCache) seqLocked(e *transcriptEntry, rev string) (uint64, bool) {
	rest, ok := strings.CutPrefix(rev, c.epoch+".")
	if !ok {
		return 0, false
	}
	gen, seq, ok := strings.Cut(rest, ".")
	if !ok || gen != strconv.FormatUint(e.gen, 10) {
		return 0, false
	}
	n, err := strconv.ParseUint(seq, 10, 64)
	return n, err == nil && n > 0 && n <= e.seq
}

// changedSince returns the earliest message index that may differ between the
// key's snapshots at revisions base and target, or math.MaxInt if none does;
// callers clamp it to their window. ok is false when base is unknown, from an
// earlier generation or epoch, evicted, or older than the retained history:
// the caller must then treat every message as changed.
func (c *transcriptCache) changedSince(key, base, target string) (from int, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.lru.Peek(key)
	if !ok {
		return 0, false
	}
	bs, okBase := c.seqLocked(e, base)
	ts, okTarget := c.seqLocked(e, target)
	if !okBase || !okTarget || bs > ts || bs+uint64(e.n) < e.seq {
		return 0, false
	}
	from = math.MaxInt
	for s := bs + 1; s <= ts; s++ {
		from = min(from, e.history[s%transcriptHistory])
	}
	return from, true
}

// invalidate marks entries stale and detaches their flights: no snapshot
// older than an invalidation is returned after it, but the next load resumes.
// The background slot is never served, so it is kept.
func (c *transcriptCache) invalidate(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		c.invalidateLocked(key)
	}
}

func (c *transcriptCache) invalidateLocked(key string) {
	delete(c.active, key)
	if e, ok := c.lru.Peek(key); ok {
		if e.rp.state == nil && c.owner[key] == nil {
			// Nothing to resume from: free the slot. An owner still running
			// holds the state, so the entry keeps its history for that
			// owner's result.
			c.lru.Evict(key)
		} else {
			e.stale = true
		}
	}
}

// evict drops entries and their provider state; the next load is cold.
func (c *transcriptCache) evict(keys ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.active, key)
		delete(c.owner, key)
		c.lru.Evict(key)
		if c.bgKey == key {
			c.bgKey, c.bg = "", resumePoint{}
		}
	}
}

func (c *transcriptCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearLocked()
}

func (c *transcriptCache) clearLocked() {
	clear(c.active)
	clear(c.owner)
	c.lru.Clear()
	c.bgKey, c.bg = "", resumePoint{}
	c.epoch = rand.Text()
}

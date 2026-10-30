package engine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ginkcode/agent-sessions/internal/files"
)

const (
	projectActiveLimit   = 4
	projectQueueLimit    = 16
	projectFlightTimeout = 2 * time.Minute
	// Stop waits briefly for loaders so shutdown normally leaves no filesystem
	// work behind, but a hung mount cannot block Engine.Close indefinitely.
	projectStopGrace = time.Second
)

var (
	ErrProjectBusy    = errors.New("project listing queue is full; retry")
	ErrProjectChanged = errors.New("project changed during listing; retry")
)

type projectFlight struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	epoch   uint64
	result  files.Listing
	err     error
}

// projectIndex shares only overlapping reconciliations, never completed status.
// owned includes canceled work until its loader returns, so cancellation cannot
// bypass admission limits. Flight identity protects replacement/eviction ABA.
type projectIndex struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	active    map[string]*projectFlight
	owned     map[*projectFlight]struct{}
	slots     chan struct{}
	epoch     uint64
	load      func(context.Context, string) (files.Listing, error)
	snapshots []projectSnapshot
	retained  int64
}

const (
	projectSnapshotBudget = 16 << 20
	projectSnapshotLimit  = 4
)

type projectSnapshot struct {
	paths      files.Listing
	bytes      int64
	generation uint64
}

func listingBytes(paths files.Listing) int64 {
	n := int64(512 + len(paths.Root))
	for _, list := range [][]string{paths.Files, paths.Directories, paths.Ignored, paths.IgnoredDirectories, paths.Repositories} {
		n += int64(cap(list) * 16)
		for _, path := range list {
			n += int64(len(path))
		}
	}
	return n
}

// retainLocked owns strings as well as slice storage: Git output paths can be
// short substrings of a much larger command buffer. Status is never retained.
func (p *projectIndex) retainLocked(paths files.Listing) {
	paths.Status = nil
	n := listingBytes(paths)
	if n > projectSnapshotBudget {
		p.removeLocked(paths.Root)
		return
	}
	paths = cloneListing(paths)
	paths.Root = strings.Clone(paths.Root)
	for _, list := range [][]string{paths.Files, paths.Directories, paths.Ignored, paths.IgnoredDirectories, paths.Repositories} {
		for i, path := range list {
			list[i] = strings.Clone(path)
		}
	}
	n = listingBytes(paths)
	p.removeLocked(paths.Root)
	for len(p.snapshots) >= projectSnapshotLimit || p.retained+n > projectSnapshotBudget {
		p.retained -= p.snapshots[0].bytes
		copy(p.snapshots, p.snapshots[1:])
		p.snapshots[len(p.snapshots)-1] = projectSnapshot{}
		p.snapshots = p.snapshots[:len(p.snapshots)-1]
	}
	p.snapshots = append(p.snapshots, projectSnapshot{paths: paths, bytes: n, generation: p.epoch})
	p.retained += n
}

func (p *projectIndex) removeLocked(root string) {
	for i := range p.snapshots {
		if p.snapshots[i].paths.Root == root {
			p.retained -= p.snapshots[i].bytes
			copy(p.snapshots[i:], p.snapshots[i+1:])
			p.snapshots[len(p.snapshots)-1] = projectSnapshot{}
			p.snapshots = p.snapshots[:len(p.snapshots)-1]
			return
		}
	}
}

func (p *projectIndex) snapshot(root string) (files.Listing, uint64, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ctx.Err() != nil {
		return files.Listing{}, 0, false
	}
	for i, entry := range p.snapshots {
		if entry.paths.Root == root {
			copy(p.snapshots[i:], p.snapshots[i+1:])
			p.snapshots[len(p.snapshots)-1] = entry
			return cloneListing(entry.paths), entry.generation, true
		}
	}
	return files.Listing{}, 0, false
}

// A global epoch avoids unbounded per-root tombstones. Unrelated snapshots can
// remain unverified, but every in-flight reconciliation retries conservatively.
// Superseded loaders are canceled so they free their slot for the retry; they
// stay owned, and so admitted, until the loader returns.
func (p *projectIndex) invalidate(root string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epoch++
	for _, f := range p.active {
		f.cancel()
	}
	p.active = make(map[string]*projectFlight)
	kept := make([]projectSnapshot, 0, projectSnapshotLimit)
	p.retained = 0
	for _, entry := range p.snapshots {
		if root == "" || rootsOverlap(root, entry.paths.Root) {
			continue
		}
		kept = append(kept, entry)
		p.retained += entry.bytes
	}
	p.snapshots = kept
}

// withProjectMutation invalidates around a write whose outcome may be
// uncertain: success, error, partial write (files.ErrSaveIncomplete) or panic.
func withProjectMutation[T any](p *projectIndex, root string, write func() (T, error)) (T, error) {
	p.invalidate(root)
	defer p.invalidate(root)
	return write()
}

func rootsOverlap(a, b string) bool {
	contains := func(root, path string) bool {
		rel, err := filepath.Rel(root, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	return contains(a, b) || contains(b, a)
}

func newProjectIndex() *projectIndex {
	ctx, cancel := context.WithCancel(context.Background())
	return &projectIndex{
		ctx: ctx, cancel: cancel, active: make(map[string]*projectFlight),
		owned: make(map[*projectFlight]struct{}), slots: make(chan struct{}, projectActiveLimit), load: files.List,
	}
}

func (p *projectIndex) start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancel()
	p.ctx, p.cancel = context.WithCancel(ctx)
	p.epoch++
	p.active = make(map[string]*projectFlight)
	p.snapshots, p.retained = nil, 0
}

func (p *projectIndex) stop() {
	p.mu.Lock()
	p.cancel()
	p.epoch++
	p.active = make(map[string]*projectFlight)
	p.snapshots, p.retained = nil, 0
	done := make([]<-chan struct{}, 0, len(p.owned))
	for f := range p.owned {
		done = append(done, f.done)
	}
	p.mu.Unlock()
	grace := time.NewTimer(projectStopGrace)
	defer grace.Stop()
	for _, ch := range done {
		select {
		case <-ch:
		case <-grace.C:
			return
		}
	}
}

// listing retries superseded shared flights, then falls back to a private
// flight that mutations do not cancel. Its result is as current as an uncached
// files.List under the same concurrent writes, and is never retained, so
// callers do not see ErrProjectChanged when edits keep arriving.
func (p *projectIndex) listing(ctx context.Context, root string) (files.Listing, error) {
	for attempt := 0; attempt < 2; attempt++ {
		out, err := p.once(ctx, root, false)
		if err != ErrProjectChanged {
			return out, err
		}
	}
	return p.once(ctx, root, true)
}

func (p *projectIndex) once(ctx context.Context, root string, private bool) (files.Listing, error) {
	if err := ctx.Err(); err != nil {
		return files.Listing{}, err
	}
	p.mu.Lock()
	if err := p.ctx.Err(); err != nil {
		p.mu.Unlock()
		return files.Listing{}, err
	}
	var f *projectFlight
	if !private {
		f = p.active[root]
	}
	if f == nil {
		if len(p.owned) >= projectActiveLimit+projectQueueLimit {
			p.mu.Unlock()
			return files.Listing{}, ErrProjectBusy
		}
		flightCtx, cancel := context.WithTimeout(p.ctx, projectFlightTimeout)
		f = &projectFlight{ctx: flightCtx, cancel: cancel, done: make(chan struct{}), epoch: p.epoch}
		if !private {
			p.active[root] = f
		}
		p.owned[f] = struct{}{}
		go p.run(root, f)
	}
	f.waiters++
	serviceCtx := p.ctx
	p.mu.Unlock()
	select {
	case <-serviceCtx.Done():
		p.leave(root, f)
		return files.Listing{}, serviceCtx.Err()
	case <-ctx.Done():
		p.leave(root, f)
		return files.Listing{}, ctx.Err()
	case <-f.done:
		p.leave(root, f)
		if err := ctx.Err(); err != nil {
			return files.Listing{}, err
		}
		p.mu.Lock()
		changed := !private && f.epoch != p.epoch
		p.mu.Unlock()
		if changed {
			return files.Listing{}, ErrProjectChanged
		}
		return cloneListing(f.result), f.err
	}
}

func (p *projectIndex) leave(root string, f *projectFlight) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f.waiters--
	if f.waiters == 0 {
		f.cancel()
		if p.active[root] == f {
			delete(p.active, root)
		}
	}
}

func (p *projectIndex) run(root string, f *projectFlight) {
	defer f.cancel()
	select {
	case p.slots <- struct{}{}:
		if err := f.ctx.Err(); err != nil {
			f.err = err
		} else {
			f.result, f.err = p.load(f.ctx, root)
			if err := f.ctx.Err(); err != nil {
				f.result, f.err = files.Listing{}, err
			}
		}
		<-p.slots
	case <-f.ctx.Done():
		f.err = f.ctx.Err()
	}
	p.mu.Lock()
	if p.active[root] == f {
		if f.err == nil && f.ctx.Err() == nil && f.epoch == p.epoch {
			p.retainLocked(f.result)
		}
		delete(p.active, root)
	}
	delete(p.owned, f)
	close(f.done)
	p.mu.Unlock()
}

func cloneListing(in files.Listing) files.Listing {
	clone := func(v []string) []string {
		if v == nil {
			return nil
		}
		return append(make([]string, 0, len(v)), v...)
	}
	in.Files, in.Directories = clone(in.Files), clone(in.Directories)
	in.Ignored, in.Repositories = clone(in.Ignored), clone(in.Repositories)
	in.IgnoredDirectories = clone(in.IgnoredDirectories)
	if in.Status != nil {
		status := make(map[string]files.Status, len(in.Status))
		for path, value := range in.Status {
			status[path] = value
		}
		in.Status = status
	}
	return in
}

package index

import (
	"context"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

type blockedScan struct {
	*providertest.Fake
	entered chan struct{}
	release chan struct{}
}

func (p *blockedScan) Scan(ctx context.Context, prev provider.ScanState) (provider.ScanResult, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return provider.ScanResult{}, ctx.Err()
	}
	return p.Fake.Scan(ctx, prev)
}

type joinedContext struct {
	context.Context
	once   sync.Once
	joined *sync.WaitGroup
}

func (c *joinedContext) Done() <-chan struct{} {
	c.once.Do(c.joined.Done)
	return c.Context.Done()
}

func TestRefreshCoalescesBurst(t *testing.T) {
	p := &blockedScan{Fake: providertest.NewFake(model.AgentClaude, "Claude"), entered: make(chan struct{}, 2), release: make(chan struct{})}
	r, _ := newTestRefresher(t, p)
	var calls sync.WaitGroup
	refresh := func(ctx context.Context) {
		defer calls.Done()
		if _, _, err := r.Refresh(ctx, p); err != nil {
			t.Errorf("Refresh: %v", err)
		}
	}
	calls.Add(1)
	go refresh(t.Context())
	<-p.entered
	var joined sync.WaitGroup
	joined.Add(50)
	for range 50 {
		calls.Add(1)
		go refresh(&joinedContext{Context: t.Context(), joined: &joined})
	}
	joined.Wait()
	close(p.release)
	calls.Wait()
	if p.ScanCalled != 2 {
		t.Fatalf("burst ran %d scans, want 2", p.ScanCalled)
	}
	r.Retire()
	if _, _, err := r.Refresh(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if p.ScanCalled != 2 {
		t.Fatal("retired refresher scanned")
	}
}

func TestDBConcurrentCloseAndReads(t *testing.T) {
	r, db := newTestRefresher(t)
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 10 {
				_, _ = db.LoadCatalog(t.Context())
				_, _ = db.LoadState(t.Context(), model.AgentClaude)
				_, _ = db.pendingFTSJobs(t.Context())
				_ = db.maintainFTS(t.Context())
			}
		})
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if _, err := db.LoadCatalog(t.Context()); err == nil {
		t.Fatal("closed database accepted read")
	}
}

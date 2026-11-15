package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/provider/providertest"
)

func TestRefreshChangesWhileClosing(t *testing.T) {
	p := providertest.NewFake(model.AgentClaude, "Claude")
	p.ScanDelay = time.Hour
	p.ScanStarted = make(chan struct{}, 1)
	e := sharedEngine(t, t.TempDir(), p)
	<-p.ScanStarted
	r := e.Refresher()
	var changes sync.WaitGroup
	for range 50 {
		changes.Go(func() { e.refreshAgent(model.AgentClaude) })
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	changes.Wait()
	e.refreshAgent(model.AgentClaude)
	if err := r.DB().SQLDB().Ping(); err == nil {
		t.Fatal("database still open after Close")
	}
}

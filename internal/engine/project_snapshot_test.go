package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
)

func TestProjectSnapshotFailedCanceledAndSuperseded(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	fail := errors.New("failed")
	p.load = func(context.Context, string) (files.Listing, error) {
		return files.Listing{Root: "root", Files: []string{"partial"}}, fail
	}
	if _, err := p.listing(context.Background(), "root"); err != fail {
		t.Fatal(err)
	}
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("error published snapshot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.listing(ctx, "root"); err != context.Canceled {
		t.Fatal(err)
	}
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("canceled request published")
	}
	p.load = func(context.Context, string) (files.Listing, error) {
		p.invalidate("root")
		return files.Listing{Root: "root", Files: []string{"old"}}, nil
	}
	if _, err := p.listing(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("superseded flight published")
	}
}

func TestProjectSnapshotBoundsOwnership(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	p.load = func(_ context.Context, root string) (files.Listing, error) {
		return files.Listing{Root: root, Files: []string{"file"}, Truncated: true, Status: map[string]files.Status{"file": files.Modified}}, nil
	}
	for i := range 5 {
		if _, err := p.listing(context.Background(), fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, ok := p.snapshot("0"); ok {
		t.Fatal("entry cap")
	}
	got, _, ok := p.snapshot("1")
	if !ok || got.Status != nil || !got.Truncated {
		t.Fatal("snapshot contract")
	}
	got.Files[0] = "changed"
	again, _, _ := p.snapshot("1")
	if again.Files[0] != "file" {
		t.Fatal("mutable snapshot")
	}
	if _, err := p.listing(context.Background(), "5"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := p.snapshot("2"); ok {
		t.Fatal("LRU not touched")
	}
	p.mu.Lock()
	p.retainLocked(files.Listing{Root: "huge", Files: []string{strings.Repeat("x", projectSnapshotBudget)}})
	p.mu.Unlock()
	if _, _, ok := p.snapshot("huge"); ok {
		t.Fatal("byte cap")
	}
	if p.retained > projectSnapshotBudget {
		t.Fatal("budget exceeded")
	}
	p.invalidate("")
	if p.retained != 0 || len(p.snapshots) != 0 {
		t.Fatal("reset retained entries")
	}
}

func TestProjectSnapshotReconciliationAndEpoch(t *testing.T) {
	p := newProjectIndex()
	defer p.stop()
	calls := 0
	p.load = func(_ context.Context, root string) (files.Listing, error) {
		calls++
		return files.Listing{Root: root, Files: []string{fmt.Sprint(calls)}}, nil
	}
	for range 2 {
		if _, err := p.listing(context.Background(), "root"); err != nil {
			t.Fatal(err)
		}
	}
	got, gen, ok := p.snapshot("root")
	if !ok || got.Files[0] != "2" || calls != 2 {
		t.Fatal("authoritative request reused stale data")
	}
	p.invalidate("root")
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("invalidated snapshot served")
	}
	if _, err := p.listing(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	_, next, _ := p.snapshot("root")
	if next == gen {
		t.Fatal("generation ABA")
	}
	p.stop()
	p.start(context.Background())
	if _, _, ok := p.snapshot("root"); ok {
		t.Fatal("restart retained snapshot")
	}
}

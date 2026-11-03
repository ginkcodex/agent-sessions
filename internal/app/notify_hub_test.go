package app

import (
	"errors"
	"sync"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/engine"
)

// countingNotifier records how often the shared service is started and
// released.
type countingNotifier struct {
	fakeNotifier
	mu    sync.Mutex
	inits int
	fails int // Initialize fails this many times first
	clean int
}

func (c *countingNotifier) Initialize() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inits++
	if c.fails > 0 {
		c.fails--
		return errors.New("not yet")
	}
	return nil
}

func (c *countingNotifier) Cleanup() {
	c.mu.Lock()
	c.clean++
	c.mu.Unlock()
}

// windowApp is an App as one window of the hub sees it.
func windowApp(hub *NotificationHub, opened *[]string, mu *sync.Mutex) *App {
	return &App{
		notifierOverride: hub.View(),
		notifyRaise:      func() {},
		desktopEvents: engine.EmitterFunc(func(name string, payload any) {
			if name == notifyOpenEvent {
				mu.Lock()
				*opened = append(*opened, payload.(map[string]string)["id"])
				mu.Unlock()
			}
		}),
	}
}

func TestNotificationHubSharesOneService(t *testing.T) {
	real := &countingNotifier{fakeNotifier: fakeNotifier{available: true, authorized: true}, fails: 1}
	hub := NewNotificationHub(real)
	var mu sync.Mutex
	var openedA, openedB []string
	a := windowApp(hub, &openedA, &mu)
	b := windowApp(hub, &openedB, &mu)

	// A failed start is retried by the next window that needs it.
	if s := a.PrepareNotifications(); s.Available {
		t.Fatalf("status after a failed start = %+v", s)
	}
	if s := b.PrepareNotifications(); !s.Available {
		t.Fatalf("second window status = %+v", s)
	}
	if s := a.PrepareNotifications(); !s.Available {
		t.Fatalf("retried status = %+v", s)
	}
	if real.inits != 2 {
		t.Fatalf("service started %d times, want 2 (one failure, one success)", real.inits)
	}

	// Both windows have a tab t1; their notifications stay apart.
	idA, idB := a.notificationID("t1"), b.notificationID("t1")
	if idA == idB {
		t.Fatalf("both windows name their t1 notification %q", idA)
	}
	if err := a.Notify(AgentNotification{Tab: "t1", Title: "A"}); err != nil {
		t.Fatal(err)
	}
	before := len(real.removed)
	if err := b.Notify(AgentNotification{Tab: "t1", Title: "B"}); err != nil {
		t.Fatal(err)
	}
	for _, removed := range real.removed[before:] {
		if removed == idA {
			t.Fatalf("window B's alert removed window A's: removed %q", real.removed)
		}
	}

	// A click reaches only the window it belongs to.
	real.click(idB)
	if len(openedA) != 0 || len(openedB) != 1 || openedB[0] != "t1" {
		t.Fatalf("click on B's alert opened A %q, B %q", openedA, openedB)
	}

	// Closing window B keeps the service for window A.
	b.closeNotifications()
	if real.clean != 0 {
		t.Fatal("closing one window released the shared service")
	}
	real.click(idA)
	real.click(idB)
	if len(openedA) != 1 || len(openedB) != 1 {
		t.Fatalf("after B closed: opened A %q, B %q", openedA, openedB)
	}

	hub.Close()
	hub.Close()
	if real.clean != 1 {
		t.Fatalf("service released %d times, want 1", real.clean)
	}
	if err := hub.View().Initialize(); err == nil {
		t.Fatal("a closed hub started the service again")
	}
}

package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// eventLog records the events one window receives.
type eventLog struct {
	mu     sync.Mutex
	events []string
}

func (l *eventLog) emit(name string, _ any) {
	l.mu.Lock()
	l.events = append(l.events, name)
	l.mu.Unlock()
}

func (l *eventLog) count(name string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, e := range l.events {
		if e == name {
			n++
		}
	}
	return n
}

// startedApp opens and starts a window and returns its name and App.
func startedApp(t *testing.T, w *Windows, desktop Desktop) (string, *App) {
	t.Helper()
	name := w.Reserve()
	w.Start(name, t.Context(), desktop)
	e := w.lookup(name)
	if e == nil {
		t.Fatalf("window %s is gone", name)
	}
	return name, e.app
}

func TestWindowsKeepEachAppsEventsInItsWindow(t *testing.T) {
	w := testWindows(t)
	var logA, logB eventLog
	nameA, a := startedApp(t, w, Desktop{Emit: logA.emit})
	_, b := startedApp(t, w, Desktop{Emit: logB.emit})

	// Both Apps number their terminals and prompts from the start, so the
	// same event must only reach the window whose App sent it.
	a.desktopEvents.Emit("terminal:output:t1", "from a")
	b.desktopEvents.Emit("terminal:output:t1", "from b")
	b.desktopEvents.Emit("terminal:output:t1", "from b")
	if logA.count("terminal:output:t1") != 1 || logB.count("terminal:output:t1") != 2 {
		t.Fatalf("events crossed windows: a=%v b=%v", logA.events, logB.events)
	}

	// Once its window closes, an App's late events go nowhere.
	w.Closed(nameA)
	a.desktopEvents.Emit("terminal:output:t1", "late")
	if logA.count("terminal:output:t1") != 1 {
		t.Fatalf("closed window got an event: %v", logA.events)
	}
}

func TestWindowsTearDownEachAppOnce(t *testing.T) {
	w := testWindows(t)
	name, a := startedApp(t, w, Desktop{})
	appCtx := a.ctx
	e := w.lookup(name)

	w.Closed(name)
	w.Closed(name)
	if release := w.detach(e); release != nil {
		t.Fatal("a closed window was detached again")
	}
	w.Shutdown()
	select {
	case <-appCtx.Done():
	default:
		t.Fatal("Shutdown returned before the closed window's App was released")
	}
	if w.lookup(name) != nil {
		t.Fatal("the closed window is still registered")
	}
}

func TestWindowsShutdownReleasesEveryApp(t *testing.T) {
	real := &countingNotifier{fakeNotifier: fakeNotifier{available: true, authorized: true}}
	hub := NewNotificationHub(real)
	w := testWindows(t)
	w.hub = hub
	var ctxs []context.Context
	for range 3 {
		_, a := startedApp(t, w, Desktop{})
		ctxs = append(ctxs, a.ctx)
	}
	if s := w.lookup("window-1").app.PrepareNotifications(); !s.Available {
		t.Fatalf("notifications through the hub: %+v", s)
	}

	w.Shutdown()
	for i, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Fatalf("window %d still running after Shutdown", i+1)
		}
	}
	real.mu.Lock()
	clean := real.clean
	real.mu.Unlock()
	if clean != 1 {
		t.Fatalf("notification service released %d times, want 1", clean)
	}
	if err := hub.View().Initialize(); !errors.Is(err, errNotificationsClosed) {
		t.Fatalf("hub after Shutdown: %v", err)
	}
}

func TestWindowsShouldQuitClosesWindowsThroughTheirGuards(t *testing.T) {
	w := testWindows(t)
	if !w.ShouldQuit() {
		t.Fatal("no windows: the app must quit")
	}

	// A window that has not started has nothing to guard.
	w.Reserve()
	if !w.ShouldQuit() {
		t.Fatal("only an unstarted window: the app must quit")
	}

	closes := make(chan string, 4)
	requests := make(chan uint64, 4)
	var name string
	name, _ = startedApp(t, w, Desktop{
		Emit: func(event string, payload any) {
			if event == "files:close-request" {
				requests <- payload.(uint64)
			}
		},
		RequestClose: func(context.Context) { closes <- name },
	})
	waitClose := func() {
		t.Helper()
		select {
		case got := <-closes:
			if got != name {
				t.Fatalf("closed %s, want %s", got, name)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the window was not asked to close")
		}
	}
	if w.ShouldQuit() {
		t.Fatal("an open window must close before the app quits")
	}
	waitClose()

	// The window has files open for editing: its guard asks the page first
	// and keeps the window open meanwhile.
	app := w.lookup(name).app
	app.EnableFileEditing()
	if !w.BeforeClose(name) {
		t.Fatal("an armed guard let the window close")
	}
	if w.ShouldQuit() {
		t.Fatal("a window kept open by its guard let the app quit")
	}
	waitClose()

	// The page approves: the guard closes the window again and lets it go,
	// so the app may quit before the window is gone.
	app.CompleteFileClose(<-requests, true)
	waitClose()
	if w.BeforeClose(name) {
		t.Fatal("an approved close was kept open")
	}
	if !w.ShouldQuit() {
		t.Fatal("every window approved: the app must quit")
	}
}

func TestWindowsStartAfterCloseDoesNothing(t *testing.T) {
	w := testWindows(t)
	name := w.Reserve()
	w.Closed(name)
	var started atomic.Bool
	w.Start(name, t.Context(), Desktop{Emit: func(string, any) { started.Store(true) }})
	if w.BeforeClose(name) {
		t.Fatal("a closed window was kept open")
	}
	if started.Load() {
		t.Fatal("a closed window's App started")
	}
}

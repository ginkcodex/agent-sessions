package app

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
)

// quitWindow is a started window whose close requests, cancellations and
// closes the test can watch.
type quitWindow struct {
	name     string
	app      *App
	requests chan uint64
	cancels  chan struct{}
	closes   chan struct{}
}

func openQuitWindow(t *testing.T, w *Windows, armed bool) *quitWindow {
	t.Helper()
	q := &quitWindow{
		requests: make(chan uint64, 8),
		cancels:  make(chan struct{}, 8),
		closes:   make(chan struct{}, 8),
	}
	q.name, q.app = startedApp(t, w, Desktop{
		Emit: func(event string, payload any) {
			switch event {
			case "files:close-request":
				q.requests <- payload.(uint64)
			case "files:close-cancelled":
				q.cancels <- struct{}{}
			}
		},
		RequestClose: func(context.Context) { q.closes <- struct{}{} },
	})
	if armed && !q.app.EnableFileEditing() {
		t.Fatal("cannot enable file editing")
	}
	return q
}

// quitCounter replaces the desktop's quit.
func quitCounter(w *Windows) chan struct{} {
	quits := make(chan struct{}, 8)
	w.quit = func() { quits <- struct{}{} }
	return quits
}

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func none[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("unexpected %s", what)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestQuitWithoutOpenWindows(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	if !w.ShouldQuit() {
		t.Fatal("no windows: the app must quit")
	}
	// A window that has not started has nothing to guard.
	w.Reserve()
	if !w.ShouldQuit() {
		t.Fatal("only an unstarted window: the app must quit")
	}
	none(t, quits, "quit")
}

func TestQuitWithCleanWindows(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, false)
	b := openQuitWindow(t, w, false)

	if w.ShouldQuit() {
		t.Fatal("open windows must agree before the app quits")
	}
	receive(t, quits, "quit")
	if !w.ShouldQuit() {
		t.Fatal("every window agreed: the app must quit")
	}
	for _, q := range []*quitWindow{a, b} {
		none(t, q.requests, "close request in a clean window")
		none(t, q.closes, "window close")
	}
}

func TestQuitAsksTheDirtyWindow(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	openQuitWindow(t, w, false)
	dirty := openQuitWindow(t, w, true)

	if w.ShouldQuit() {
		t.Fatal("a dirty window let the app quit unasked")
	}
	id := receive(t, dirty.requests, "close request")
	// Quitting again while the user decides asks nothing more.
	if w.ShouldQuit() {
		t.Fatal("the app quit while the user decides")
	}
	none(t, dirty.requests, "second close request")
	none(t, quits, "quit before the answer")

	dirty.app.CompleteFileClose(id, true)
	receive(t, quits, "quit")
	if !w.ShouldQuit() {
		t.Fatal("approved quit refused")
	}
	// The window stays open, frozen, until the app quits.
	none(t, dirty.closes, "window close")
}

func TestQuitRefusedThawsTheWindowsThatAgreed(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, true)
	b := openQuitWindow(t, w, true)

	w.ShouldQuit()
	a.app.CompleteFileClose(receive(t, a.requests, "close request in a"), true)
	b.app.CompleteFileClose(receive(t, b.requests, "close request in b"), false)
	receive(t, a.cancels, "cancellation in a")
	none(t, b.cancels, "cancellation in the window that refused")
	none(t, quits, "quit")
	if !a.app.EnableFileEditing() || !b.app.EnableFileEditing() {
		t.Fatal("editing stayed blocked after the quit was called off")
	}

	// The next quit asks again from the start.
	if w.ShouldQuit() {
		t.Fatal("a refused quit was remembered as approved")
	}
	a.app.CompleteFileClose(receive(t, a.requests, "new close request in a"), true)
	b.app.CompleteFileClose(receive(t, b.requests, "new close request in b"), true)
	receive(t, quits, "quit")
}

func TestQuitSkipsAWindowThatClosesMeanwhile(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, true)

	w.ShouldQuit()
	receive(t, a.requests, "close request")
	w.Closed(a.name)
	receive(t, quits, "quit")
}

func TestQuitJoinsAPendingWindowClose(t *testing.T) {
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, true)

	// The user closed the window and is deciding about its files.
	if !w.BeforeClose(a.name) {
		t.Fatal("an armed guard let the window close")
	}
	id := receive(t, a.requests, "close request")
	w.ShouldQuit()
	none(t, a.requests, "second close request")

	// Approving the close closes the window and lets the quit go on.
	a.app.CompleteFileClose(id, true)
	receive(t, a.closes, "window close")
	receive(t, quits, "quit")
}

// stubRestart replaces the restart's process start. It must be called before
// testWindows so the real one is back only after the windows' cleanup.
func stubRestart(t *testing.T, start func() error) {
	t.Helper()
	oldStart, oldCommand := startAnotherCopy, newWindowCommand
	t.Cleanup(func() { startAnotherCopy, newWindowCommand = oldStart, oldCommand })
	startAnotherCopy = start
	newWindowCommand = func() (*exec.Cmd, error) { return exec.Command("agent-sessions"), nil }
}

func TestRestartStartsAgainOnceEveryAppClosed(t *testing.T) {
	var apps []*App
	starts := make(chan bool, 4)
	stubRestart(t, func() error {
		closed := true
		for _, a := range apps {
			closed = closed && a.ctx.Err() != nil
		}
		starts <- closed
		return nil
	})
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, true)
	b := openQuitWindow(t, w, false)
	apps = []*App{a.app, b.app}

	if err := b.app.RestartApp(); err != nil {
		t.Fatal(err)
	}
	id := receive(t, a.requests, "close request")
	if err := a.app.RestartApp(); !errors.Is(err, errQuitting) {
		t.Fatalf("second restart: %v", err)
	}
	if w.ShouldQuit() {
		t.Fatal("the app quit during the restart's questions")
	}
	a.app.CompleteFileClose(id, true)
	receive(t, quits, "quit")
	none(t, starts, "start before the app quit")
	if err := a.app.RestartApp(); !errors.Is(err, errQuitting) {
		t.Fatalf("restart while quitting: %v", err)
	}

	w.Shutdown()
	if closed := receive(t, starts, "new copy"); !closed {
		t.Fatal("the new copy started before every App closed")
	}
	w.Shutdown()
	none(t, starts, "second new copy")
}

func TestRestartRefused(t *testing.T) {
	starts := make(chan bool, 4)
	stubRestart(t, func() error { starts <- true; return nil })
	w := testWindows(t)
	quits := quitCounter(w)
	a := openQuitWindow(t, w, true)

	if err := a.app.RestartApp(); err != nil {
		t.Fatal(err)
	}
	a.app.CompleteFileClose(receive(t, a.requests, "close request"), false)
	// Once the refusal is handled, a later quit is not a restart.
	waitFor(t, "quit round to end", func() bool {
		w.quitMu.Lock()
		defer w.quitMu.Unlock()
		return !w.quitting.running
	})
	w.ShouldQuit()
	a.app.CompleteFileClose(receive(t, a.requests, "quit request"), true)
	receive(t, quits, "quit")
	w.Shutdown()
	none(t, starts, "new copy after a refused restart")
	none(t, a.cancels, "cancellation in the window that refused")
}

func TestRestartFailsWithoutTheApp(t *testing.T) {
	stubRestart(t, func() error { return nil })
	newWindowCommand = func() (*exec.Cmd, error) { return nil, errors.New("no executable") }
	w := testWindows(t)
	a := openQuitWindow(t, w, true)

	if err := a.app.RestartApp(); err == nil {
		t.Fatal("restart without the app's executable succeeded")
	}
	none(t, a.requests, "close request for a restart that cannot start")
	w.quitMu.Lock()
	state := w.quitting
	w.quitMu.Unlock()
	if state != (quitState{}) {
		t.Fatalf("failed restart left %+v", state)
	}
}

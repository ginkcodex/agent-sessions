package app

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestFileCloseGuard(t *testing.T) {
	var a App
	if prevent, id := a.fileClose.request(); prevent || id != 0 {
		t.Fatal("clean startup should close")
	}
	if !a.EnableFileEditing() {
		t.Fatal("cannot arm guard")
	}
	prevent, id := a.fileClose.request()
	if !prevent || id == 0 {
		t.Fatal("armed window must defer close")
	}
	if a.EnableFileEditing() {
		t.Fatal("editing cannot begin during close")
	}
	if prevent, duplicate := a.fileClose.request(); !prevent || duplicate != 0 {
		t.Fatal("duplicate request")
	}
	if a.fileClose.complete(id+1, true) {
		t.Fatal("stale approval")
	}
	if a.fileClose.complete(id, false) {
		t.Fatal("cancel must not authorize")
	}
	if !a.EnableFileEditing() {
		t.Fatal("cancel should unblock editing")
	}
	prevent, next := a.fileClose.request()
	if !prevent || next == id || next == 0 {
		t.Fatal("fresh request required")
	}
	if a.fileClose.complete(id, true) {
		t.Fatal("old approval")
	}
	if !a.fileClose.complete(next, true) {
		t.Fatal("approval failed")
	}
	if a.EnableFileEditing() {
		t.Fatal("input must stay blocked before authorized quit")
	}
	if prevent, id := a.fileClose.request(); prevent || id != 0 {
		t.Fatal("authorized quit should pass")
	}
	if prevent, id := a.fileClose.request(); !prevent || id == 0 {
		t.Fatal("authorization must be one-shot")
	}
}

func TestFileCloseGuardAsksWhileTerminalsRun(t *testing.T) {
	running := false
	g := fileCloseGuard{busy: func() bool { return running }}
	if prevent, id := g.request(); prevent || id != 0 {
		t.Fatal("a window with nothing running should close")
	}
	if id, answer := g.requestHeld(); id != 0 || answer != nil {
		t.Fatal("a window with nothing running should agree to a quit")
	}
	running = true
	prevent, id := g.request()
	if !prevent || id == 0 {
		t.Fatal("a running terminal must defer the close to the frontend")
	}
	if !g.complete(id, true) {
		t.Fatal("approval should close the window")
	}
	if prevent, again := g.request(); prevent || again != 0 {
		t.Fatal("the approved close should pass")
	}
	held, answer := g.requestHeld()
	if held == 0 || answer == nil {
		t.Fatal("a running terminal must ask before a quit")
	}
	g.complete(held, false)
	running = false
	if prevent, id := g.request(); prevent || id != 0 {
		t.Fatal("once the terminals end the window should close unasked")
	}
}

func TestFileCloseGuardHeldRequest(t *testing.T) {
	var g fileCloseGuard
	if id, answer := g.requestHeld(); id != 0 || answer != nil {
		t.Fatal("an unarmed window must agree at once")
	}
	g.armed = true
	id, answer := g.requestHeld()
	if id == 0 || answer == nil {
		t.Fatal("an armed window must ask its editors")
	}
	// A close gesture meanwhile waits for the same answer.
	if prevent, dup := g.request(); !prevent || dup != 0 {
		t.Fatal("a close during a held request opened another request")
	}
	if joined, again := g.requestHeld(); joined != 0 || again != answer {
		t.Fatal("a second quit must join the pending request")
	}
	// An approved held request never closes the window by itself, and never
	// lets a later close through unasked.
	if g.complete(id, true) {
		t.Fatal("an approved held request closed the window")
	}
	select {
	case <-answer.done:
	default:
		t.Fatal("the held request was not answered")
	}
	if !answer.allow || g.authorized {
		t.Fatalf("allow=%v authorized=%v", answer.allow, g.authorized)
	}
	if prevent, next := g.request(); !prevent || next == 0 {
		t.Fatal("a close after a held approval skipped the editors")
	}
}

func TestFileCloseGuardQuitJoinsPendingClose(t *testing.T) {
	var g fileCloseGuard
	g.armed = true
	_, id := g.request()
	joined, answer := g.requestHeld()
	if joined != 0 || answer == nil {
		t.Fatal("a quit must join the pending close")
	}
	// The close was the user's: approving it still closes the window.
	if !g.complete(id, true) || !answer.allow {
		t.Fatal("the joined close must close the window and approve the quit")
	}
}

func TestFileCloseGuardConcurrentRequests(t *testing.T) {
	var g fileCloseGuard
	g.armed = true
	var wg sync.WaitGroup
	ids := make(chan uint64, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			prevent, id := g.request()
			if !prevent {
				t.Error("unguarded request")
			}
			if id != 0 {
				ids <- id
			}
		}()
	}
	wg.Wait()
	close(ids)
	count := 0
	for range ids {
		count++
	}
	if count != 1 {
		t.Fatalf("emitted %d requests", count)
	}
}

// fakeWindow models the desktop shell's close path: RequestClose (and an OS
// quit routed to it) runs the window's closing hook, which calls OnBeforeClose
// exactly once and closes unless it is cancelled.
type fakeWindow struct {
	a        *App
	hooks    int
	closed   int
	events   []any
	messages []MessageDialogOptions
}

func (w *fakeWindow) desktop() Desktop {
	return Desktop{
		Emit: func(name string, payload any) {
			w.events = append(w.events, name, payload)
		},
		Message: func(_ context.Context, opts MessageDialogOptions) (string, error) {
			w.messages = append(w.messages, opts)
			return "", nil
		},
		RequestClose: func(context.Context) { w.close() },
	}
}

// close is window.Close: the WindowClosing hook runs the guard once.
func (w *fakeWindow) close() {
	w.hooks++
	if !OnBeforeClose(w.a) {
		w.closed++
	}
}

func newFakeWindow(t *testing.T) (*App, *fakeWindow) {
	t.Helper()
	a := &App{}
	w := &fakeWindow{a: a}
	a.ctx = withDesktop(context.Background(), w.desktop())
	return a, w
}

func TestNativeCloseThroughDesktop(t *testing.T) {
	a, w := newFakeWindow(t)
	w.close()
	if w.closed != 1 || len(w.events) != 0 {
		t.Fatalf("unarmed close: closed=%d events=%v", w.closed, w.events)
	}

	a, w = newFakeWindow(t)
	a.EnableFileEditing()
	// An OS quit (ShouldQuit) is routed to window.Close, so it is the same
	// single gesture as clicking the close button.
	w.close()
	if w.closed != 0 || len(w.events) != 2 || w.events[0] != "files:close-request" {
		t.Fatalf("armed close must be cancelled with a request: closed=%d events=%v", w.closed, w.events)
	}
	id := w.events[1].(uint64)
	w.close() // Duplicate gesture while the frontend decides.
	if w.closed != 0 || len(w.events) != 2 {
		t.Fatalf("duplicate close: closed=%d events=%v", w.closed, w.events)
	}
	a.CompleteFileClose(id+1, true) // Stale approval.
	if w.hooks != 2 || w.closed != 0 {
		t.Fatalf("stale approval closed the window: hooks=%d closed=%d", w.hooks, w.closed)
	}
	a.CompleteFileClose(id, true)
	if w.hooks != 3 || w.closed != 1 {
		t.Fatalf("approval must re-request a guarded close once: hooks=%d closed=%d", w.hooks, w.closed)
	}
	a.CompleteFileClose(id, true) // Replayed approval.
	if w.hooks != 3 {
		t.Fatalf("replayed approval requested another close: hooks=%d", w.hooks)
	}
	// The approval was consumed by exactly one closing hook.
	w.close()
	if w.closed != 1 || len(w.events) != 4 {
		t.Fatalf("approval reused: closed=%d events=%v", w.closed, w.events)
	}
}

func TestNativeCloseCancelled(t *testing.T) {
	a, w := newFakeWindow(t)
	a.EnableFileEditing()
	w.close()
	id := w.events[1].(uint64)
	a.CompleteFileClose(id, false)
	if w.hooks != 1 || w.closed != 0 {
		t.Fatalf("cancel closed the window: hooks=%d closed=%d", w.hooks, w.closed)
	}
	if !a.EnableFileEditing() {
		t.Fatal("cancel left editing blocked")
	}
}

func TestDesktopDefaultsWithoutShell(t *testing.T) {
	var a App
	// No desktop: closing requests and events are no-ops, never panics.
	if err := a.RestartApp(); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("restart without desktop: %v", err)
	}
	if _, err := saveFileDialog(context.Background(), SaveDialogOptions{}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("save dialog without desktop: %v", err)
	}
	if _, err := openFileDialog(context.Background(), OpenDialogOptions{}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("open dialog without desktop: %v", err)
	}
	a.ctx = context.Background()
	if err := a.PrintWindow(); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("print without desktop: %v", err)
	}
	if err := a.ToggleFullscreen(); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("fullscreen without desktop: %v", err)
	}
	if _, err := openDirectoryDialog(a.ctx, OpenDialogOptions{}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("directory dialog without desktop: %v", err)
	}
	if _, err := messageDialog(a.ctx, MessageDialogOptions{}); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("message dialog without desktop: %v", err)
	}
	raiseWindow(a.ctx)
	emitterFrom(a.ctx).Emit("probe", nil)
}

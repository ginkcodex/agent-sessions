package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestRestartAppCloseApproval(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "approve"}[allow], func(t *testing.T) {
			var a App
			oldQuit, oldCommand, oldReport := quitApp, newWindowCommand, reportRestartError
			t.Cleanup(func() {
				quitApp, newWindowCommand, reportRestartError = oldQuit, oldCommand, oldReport
			})
			quits, launches, reports := 0, 0, 0
			quitApp = func(context.Context) {
				// Acquiring the mutex proves Quit is not invoked under it.
				a.fileClose.mu.Lock()
				a.fileClose.mu.Unlock()
				quits++
			}
			newWindowCommand = func() (*exec.Cmd, error) {
				a.fileClose.mu.Lock()
				a.fileClose.mu.Unlock()
				launches++
				if !OnBeforeClose(&a) {
					t.Error("duplicate close passed during launch")
				}
				return nil, errors.New("test launch failure")
			}
			reportRestartError = func(_ context.Context, err error) {
				if !strings.Contains(err.Error(), "test launch failure") {
					t.Error(err)
				}
				reports++
			}
			a.EnableFileEditing()
			if err := a.RestartApp(); err != nil {
				t.Fatal(err)
			}
			if quits != 1 || launches != 0 {
				t.Fatal("restart must request Quit without spawning")
			}
			if err := a.RestartApp(); err == nil {
				t.Fatal("duplicate restart accepted")
			}
			prevent, id := a.fileClose.request()
			if !prevent || id == 0 {
				t.Fatal("restart did not defer to file guard")
			}
			a.CompleteFileClose(id+1, false) // Stale cancellation must not clear restart.
			a.CompleteFileClose(id, allow)
			if allow {
				if quits != 2 || launches != 0 {
					t.Fatal("approval must re-request Quit before spawning")
				}
				if !OnBeforeClose(&a) {
					t.Fatal("launch failure must prevent quitting")
				}
				if launches != 1 || reports != 1 {
					t.Fatalf("launches=%d reports=%d", launches, reports)
				}
				if !a.EnableFileEditing() {
					t.Fatal("launch failure left backend editing blocked")
				}
			} else if quits != 1 {
				t.Fatal("cancel requested another Quit")
			}
			// A later ordinary close must never retry a canceled/failed restart.
			_, next := a.fileClose.request()
			a.CompleteFileClose(next, true)
			if OnBeforeClose(&a) {
				t.Fatal("ordinary approved close prevented")
			}
			if launches != map[bool]int{false: 0, true: 1}[allow] {
				t.Fatal("restart remained pending")
			}
		})
	}
}

func TestRestartAppApprovedLaunch(t *testing.T) {
	var a App
	oldQuit, oldCommand := quitApp, newWindowCommand
	t.Cleanup(func() { quitApp, newWindowCommand = oldQuit, oldCommand })
	quitApp = func(context.Context) {}
	// Run the Go test binary with no matching tests: portable, no shell needed.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	newWindowCommand = func() (*exec.Cmd, error) {
		launches++
		return exec.Command(exe, "-test.run=^$"), nil
	}
	if err := a.RestartApp(); err != nil {
		t.Fatal(err)
	}
	if OnBeforeClose(&a) || launches != 1 {
		t.Fatal("approved restart should launch and allow close")
	}
	if !OnBeforeClose(&a) || launches != 1 {
		t.Fatal("duplicate native close relaunched")
	}
}

func TestRestartAppStartFailure(t *testing.T) {
	var a App
	oldQuit, oldCommand, oldReport := quitApp, newWindowCommand, reportRestartError
	t.Cleanup(func() {
		quitApp, newWindowCommand, reportRestartError = oldQuit, oldCommand, oldReport
	})
	quitApp = func(context.Context) {}
	newWindowCommand = func() (*exec.Cmd, error) {
		return exec.Command(filepath.Join(t.TempDir(), "missing-app")), nil
	}
	reported := false
	reportRestartError = func(context.Context, error) { reported = true }
	if err := a.RestartApp(); err != nil {
		t.Fatal(err)
	}
	if !OnBeforeClose(&a) || !reported {
		t.Fatal("start failure must report an error and keep current app")
	}
	if OnBeforeClose(&a) {
		t.Fatal("failed restart blocked a later ordinary close")
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

func TestRestartThroughDesktop(t *testing.T) {
	oldCommand := newWindowCommand
	t.Cleanup(func() { newWindowCommand = oldCommand })
	launches := 0
	newWindowCommand = func() (*exec.Cmd, error) {
		launches++
		return nil, errors.New("test launch failure")
	}

	a, w := newFakeWindow(t)
	if err := a.RestartApp(); err != nil {
		t.Fatal(err)
	}
	// Restart goes through window.Close, so the failed launch cancels it and
	// the current process stays alive with an error dialog.
	if w.hooks != 1 || w.closed != 0 || launches != 1 {
		t.Fatalf("failed restart: hooks=%d closed=%d launches=%d", w.hooks, w.closed, launches)
	}
	if len(w.messages) != 1 || w.messages[0].Type != ErrorDialog || !strings.Contains(w.messages[0].Message, "test launch failure") {
		t.Fatalf("restart error not reported: %+v", w.messages)
	}
	if len(w.events) != 2 || w.events[0] != "files:close-cancelled" {
		t.Fatalf("restart failure events = %v", w.events)
	}
	// A later ordinary close is not a restart.
	w.close()
	if w.closed != 1 || launches != 1 {
		t.Fatalf("ordinary close after failed restart: closed=%d launches=%d", w.closed, launches)
	}
}

func TestRestartThroughDesktopWithDirtyFiles(t *testing.T) {
	oldCommand := newWindowCommand
	t.Cleanup(func() { newWindowCommand = oldCommand })
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launches := 0
	newWindowCommand = func() (*exec.Cmd, error) {
		launches++
		return exec.Command(exe, "-test.run=^$"), nil
	}

	a, w := newFakeWindow(t)
	a.EnableFileEditing()
	if err := a.RestartApp(); err != nil {
		t.Fatal(err)
	}
	if w.hooks != 1 || w.closed != 0 || launches != 0 || len(w.events) != 2 {
		t.Fatalf("restart must defer to the editors: hooks=%d closed=%d launches=%d events=%v", w.hooks, w.closed, launches, w.events)
	}
	a.CompleteFileClose(w.events[1].(uint64), true)
	if w.hooks != 2 || w.closed != 1 || launches != 1 {
		t.Fatalf("approved restart: hooks=%d closed=%d launches=%d", w.hooks, w.closed, launches)
	}
	// While the replacement is starting, more closes are rejected, not relaunched.
	w.close()
	if w.closed != 1 || launches != 1 {
		t.Fatalf("duplicate close during restart: closed=%d launches=%d", w.closed, launches)
	}
}

func TestDesktopDefaultsWithoutShell(t *testing.T) {
	var a App
	// No desktop: closing requests and events are no-ops, never panics.
	if err := a.RestartApp(); err != nil {
		t.Fatal(err)
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

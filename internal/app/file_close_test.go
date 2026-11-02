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
				if !a.OnBeforeClose(context.Background()) {
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
				if !a.OnBeforeClose(context.Background()) {
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
			if a.OnBeforeClose(context.Background()) {
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
	if a.OnBeforeClose(context.Background()) || launches != 1 {
		t.Fatal("approved restart should launch and allow close")
	}
	if !a.OnBeforeClose(context.Background()) || launches != 1 {
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
	if !a.OnBeforeClose(context.Background()) || !reported {
		t.Fatal("start failure must report an error and keep current app")
	}
	if a.OnBeforeClose(context.Background()) {
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

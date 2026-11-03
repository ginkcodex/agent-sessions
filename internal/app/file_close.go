package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Once editing has been enabled, native close requests always go through the
// frontend. Keeping this armed for the window lifetime avoids a first-keystroke
// race between asynchronous dirty notifications and the native UI thread.
// The frontend must freeze editor input while approving a close.
type fileCloseGuard struct {
	mu         sync.Mutex
	armed      bool
	sequence   uint64
	pending    uint64
	authorized bool
	// Restart intent shares the close guard's mutex so a valid cancellation
	// clears it atomically. launching also rejects duplicate native closes while
	// starting the replacement, without holding the mutex across process launch.
	restartPending bool
	launching      bool
}

// quitApp asks the window to close through its guarded close path, so the
// resulting closing hook runs OnBeforeClose; tests replace it.
var quitApp = requestWindowClose

var reportRestartError = func(ctx context.Context, err error) {
	emitterFrom(ctx).Emit("files:close-cancelled", nil)
	_, _ = messageDialog(ctx, MessageDialogOptions{
		Type: ErrorDialog, Title: "Restart failed", Message: err.Error(),
	})
}

// RestartApp requests a normal, guarded close before starting a replacement.
// The caller must first warn that restarting ends this window's terminal tabs.
// Success means the close was requested, not that it was approved or restarted.
func (a *App) RestartApp() error {
	g := &a.fileClose
	g.mu.Lock()
	if g.restartPending || g.launching || g.pending != 0 || g.authorized {
		g.mu.Unlock()
		return errors.New("a close or restart is already in progress")
	}
	g.restartPending = true
	g.mu.Unlock()
	quitApp(a.appCtx())
	return nil
}

// EnableFileEditing must complete before the first editable document mounts.
// False means a native close is already being coordinated.
func (a *App) EnableFileEditing() bool {
	a.fileClose.mu.Lock()
	defer a.fileClose.mu.Unlock()
	if a.fileClose.pending != 0 || a.fileClose.authorized || a.fileClose.launching {
		return false
	}
	a.fileClose.armed = true
	return true
}

// onBeforeClose never waits for the webview: it runs on the UI thread.
// Instead it prevents the close and emits a request. Approval requests another
// window close, whose hook consumes the one-shot authorization.
func (a *App) onBeforeClose(ctx context.Context) bool {
	prevent, id := a.fileClose.request()
	if id != 0 {
		emitterFrom(ctx).Emit("files:close-request", id)
	}
	if prevent {
		return true
	}
	return a.restartAfterApproval(ctx)
}

// restartAfterApproval runs only after request has approved the native close.
func (a *App) restartAfterApproval(ctx context.Context) bool {
	g := &a.fileClose
	g.mu.Lock()
	if !g.restartPending {
		g.mu.Unlock()
		return false
	}
	g.restartPending = false
	g.launching = true
	g.mu.Unlock()

	// NewWindow reuses the executable command, askpass environment filtering,
	// and detached process handling. Never launch or show a dialog under mu.
	err := a.NewWindow()
	if err != nil {
		reportRestartError(ctx, fmt.Errorf("restart the app: %w", err))
		g.mu.Lock()
		g.launching = false
		g.mu.Unlock()
		return true // Keep the current process alive if its replacement failed.
	}
	// Leave launching set until shutdown to reject duplicate native closes.
	return false
}

func (g *fileCloseGuard) request() (bool, uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.launching {
		return true, 0
	}
	if g.authorized {
		g.authorized = false
		return false, 0
	}
	if !g.armed {
		return false, 0
	}
	if g.pending != 0 {
		return true, 0
	}
	g.sequence++
	g.pending = g.sequence
	return true, g.pending
}

func (g *fileCloseGuard) complete(id uint64, allow bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if id == 0 || id != g.pending {
		return false
	}
	g.pending = 0
	if !allow {
		g.restartPending = false
	}
	g.authorized = allow
	return allow
}

// CompleteFileClose answers the pending native close request. Replayed or stale
// approvals cannot close the window. The frontend must keep editing frozen after
// approval until this window has exited.
func (a *App) CompleteFileClose(id uint64, allow bool) {
	if a.fileClose.complete(id, allow) {
		quitApp(a.appCtx())
	}
}

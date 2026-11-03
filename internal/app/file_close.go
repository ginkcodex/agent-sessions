package app

import (
	"context"
	"fmt"
	"sync"
)

// Once editing has been enabled, native close requests always go through the
// frontend. Keeping this armed for the window lifetime avoids a first-keystroke
// race between asynchronous dirty notifications and the native UI thread.
// The frontend must freeze editor input while approving a close.
//
// A request either closes the window (the user closes it) or is held: an app
// quit or restart asks every window first and closes none of them until all
// have agreed (see Windows.ShouldQuit).
type fileCloseGuard struct {
	mu         sync.Mutex
	armed      bool
	sequence   uint64
	pending    uint64
	held       bool       // the pending request is part of a quit
	answer     *closeWait // answered when the pending request completes
	authorized bool
}

// closeWait is the frontend's answer to one close request.
type closeWait struct {
	done  chan struct{}
	allow bool // set before done is closed
}

// quitApp asks the window to close through its guarded close path, so the
// resulting closing hook runs OnBeforeClose; tests replace it.
var quitApp = requestWindowClose

// RestartApp quits the app, asking every window about its unsaved files
// first, and then starts it again with one window. The caller must first
// warn that restarting ends the terminal tabs. Success means the restart was
// requested, not that the windows agreed to it.
func (a *App) RestartApp() error {
	if fn := desktopFrom(a.ctx).Restart; fn != nil {
		return fn()
	}
	return fmt.Errorf("restart: %w", ErrDesktopUnavailable)
}

// EnableFileEditing must complete before the first editable document mounts.
// False means a native close is already being coordinated.
func (a *App) EnableFileEditing() bool {
	a.fileClose.mu.Lock()
	defer a.fileClose.mu.Unlock()
	if a.fileClose.pending != 0 || a.fileClose.authorized {
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
	return prevent
}

// approveQuit asks this window whether the app may quit, without closing
// it. It waits for the frontend's answer, or until gone is closed: a closed
// window has nothing left to lose. A window with a close request already
// open answers that request instead. held reports that this window approved
// and now waits for the quit: if the quit is called off, cancelQuit must
// tell it.
func (a *App) approveQuit(gone <-chan struct{}) (allow, held bool) {
	ctx := a.appCtx()
	id, answer := a.fileClose.requestHeld()
	if answer == nil {
		return true, false
	}
	if id != 0 {
		emitterFrom(ctx).Emit("files:close-request", id)
	}
	raiseWindow(ctx)
	select {
	case <-answer.done:
		return answer.allow, id != 0 && answer.allow
	case <-gone:
		return true, false
	}
}

// cancelQuit tells the window that a quit it approved was called off, so
// its editors thaw.
func (a *App) cancelQuit() {
	emitterFrom(a.appCtx()).Emit("files:close-cancelled", nil)
}

func (g *fileCloseGuard) request() (bool, uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
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
	return true, g.open(false)
}

// requestHeld opens a held request and returns its ID, or joins the pending
// request with ID 0. A nil answer means the window may close as it is.
func (g *fileCloseGuard) requestHeld() (uint64, *closeWait) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.armed || g.authorized {
		return 0, nil
	}
	if g.pending != 0 {
		return 0, g.answer
	}
	id := g.open(true)
	return id, g.answer
}

// open starts a new request; g.mu is held.
func (g *fileCloseGuard) open(held bool) uint64 {
	g.sequence++
	g.pending = g.sequence
	g.held = held
	g.answer = &closeWait{done: make(chan struct{})}
	return g.pending
}

// complete answers the pending request. It reports whether the window must
// now close: never for a held request, whose window closes with the app.
func (g *fileCloseGuard) complete(id uint64, allow bool) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if id == 0 || id != g.pending {
		return false
	}
	g.pending = 0
	held := g.held
	g.held = false
	if g.answer != nil {
		g.answer.allow = allow
		close(g.answer.done)
		g.answer = nil
	}
	if held {
		return false
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

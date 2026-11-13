package app

import (
	"context"
	"errors"
	"log"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ginkcode/agent-sessions/internal/update"
)

// ErrWindowClosed is returned by calls from a window that is closed or
// closing.
var ErrWindowClosed = errors.New("this window is closed")

// Windows is the app's open windows, all in one process. Each window has an
// App of its own, as if it were a separate app: its own engine, connection,
// host, terminals and close guard. Only the desktop's notifications are
// shared (see NotificationHub).
//
// The desktop shell drives it: Reserve names a new window and creates its
// App, Start starts the App once the window exists, BeforeClose and Closed
// follow the window's close, ShouldQuit decides whether the app may quit and
// Shutdown closes what is left when it does. The shell binds Router, which
// sends each frontend call to the App of the window that made it.
type Windows struct {
	hub  *NotificationHub
	open func() error // opens another window
	quit func()       // quits the app through ShouldQuit
	// newApp creates a window's App; tests replace it.
	newApp func() *App
	// updater is shared by every window's App; Shutdown hands it the
	// quit.
	updater *update.Manager

	mu      sync.Mutex
	entries map[string]*windowEntry
	order   []*windowEntry // in the order the windows opened
	next    uint64

	// closing counts window teardowns in progress; Shutdown waits for them.
	closing sync.WaitGroup

	quitMu   sync.Mutex
	quitting quitState
}

type windowEntry struct {
	name string
	app  *App

	mu      sync.Mutex
	started bool
	closed  bool // set once teardown begins: no more calls or events
	desktop Desktop
	cancel  context.CancelFunc
	ready   chan struct{} // closed once the App has started
	gone    chan struct{} // closed once the window is closed

	// closeApproved is set when the close guard lets the window close, before
	// the window is gone; the app may then quit without asking it again.
	closeApproved atomic.Bool
}

// NewWindows creates the registry. hub shares notifications among the
// windows' Apps, open opens another window and quit quits the app through
// ShouldQuit; any may be nil.
func NewWindows(hub *NotificationHub, open func() error, quit func()) *Windows {
	return &Windows{
		hub:     hub,
		open:    open,
		quit:    quit,
		newApp:  NewApp,
		updater: newUpdateManager(),
		entries: map[string]*windowEntry{},
	}
}

// Reserve creates the App of a new window and returns the window's name. The
// shell must give the window exactly this name: frontend calls find their
// App by it. Names are never reused, so a late call from a closed window
// cannot reach a newer one.
func (w *Windows) Reserve() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.next++
	e := &windowEntry{
		name:  "window-" + strconv.FormatUint(w.next, 10),
		app:   w.newApp(),
		ready: make(chan struct{}),
		gone:  make(chan struct{}),
	}
	e.app.updater = w.updater
	w.entries[e.name] = e
	w.order = append(w.order, e)
	return e.name
}

// Start starts the App of a reserved window, which then gets the window's
// own context: cancelled when the window closes, not when the app quits.
// desktop is the window's adapter; Start adds the shared notifications and
// New Window and Restart to it. Calls from the window wait until Start has returned.
func (w *Windows) Start(name string, parent context.Context, desktop Desktop) {
	e := w.lookup(name)
	if e == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	emit := desktop.Emit
	desktop.Emit = func(name string, payload any) {
		// A closing window's App is shutting down; its events go nowhere.
		if emit != nil && !e.isClosed() {
			emit(name, payload)
		}
	}
	if w.hub != nil {
		desktop.Notifications = w.hub.View()
	}
	if desktop.OpenWindow == nil {
		desktop.OpenWindow = w.open
	}
	if desktop.Restart == nil {
		desktop.Restart = w.requestRestart
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		cancel()
		return
	}
	e.started = true
	e.desktop = desktop
	e.cancel = cancel
	e.mu.Unlock()

	OnStartup(e.app, ctx, desktop)
	close(e.ready)
}

// BeforeClose runs the close guard of a window the user is closing. It
// returns true to keep the window open; the guard then asks the frontend
// about unsaved files and closes the window again once they are handled.
func (w *Windows) BeforeClose(name string) bool {
	e := w.lookup(name)
	if e == nil || !e.isStarted() {
		return false
	}
	if OnBeforeClose(e.app) {
		return true
	}
	e.closeApproved.Store(true)
	return false
}

// Closed tears down the App of a window that has closed. Calls and events
// of the window stop at once; the rest runs in the background, and Shutdown
// waits for it.
func (w *Windows) Closed(name string) {
	e := w.lookup(name)
	if e == nil {
		return
	}
	if release := w.detach(e); release != nil {
		w.closing.Add(1)
		go func() {
			defer w.closing.Done()
			release()
		}()
	}
}

// FocusNextWindow brings the next app window to the front, after window
// name in the ring of windows, across processes.
func (w *Windows) FocusNextWindow(name string, forward bool) error {
	e := w.lookup(name)
	if e == nil || !e.isStarted() {
		return nil
	}
	return e.app.FocusNextWindow(forward)
}

// shutdownTimeout bounds how long Shutdown waits for the windows' Apps.
const shutdownTimeout = 10 * time.Second

// Shutdown closes the Apps of every window still open and waits for those
// already closing. The shell calls it once, when the app quits; it never
// touches the windows themselves, which the toolkit is destroying.
func (w *Windows) Shutdown() {
	w.mu.Lock()
	open := append([]*windowEntry(nil), w.order...)
	w.mu.Unlock()
	for _, e := range open {
		if release := w.detach(e); release != nil {
			w.closing.Add(1)
			go func() {
				defer w.closing.Done()
				release()
			}()
		}
	}
	done := make(chan struct{})
	go func() {
		w.closing.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		log.Printf("agent-sessions: windows still closing after %s; quitting anyway", shutdownTimeout)
	}
	if w.hub != nil {
		w.hub.Close()
	}
	// Every window is closed, so nothing the installer replaces is in use.
	if w.finishUpdate() {
		return
	}
	w.restartIfApproved()
}

// detach stops a window's calls and events and forgets the window. It
// returns the rest of the teardown, which releases the App's terminals,
// connection, engine and notifications, or nil when the window was already
// detached.
func (w *Windows) detach(e *windowEntry) (release func()) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	started, cancel := e.started, e.cancel
	e.mu.Unlock()
	close(e.gone)

	w.mu.Lock()
	delete(w.entries, e.name)
	if i := slices.Index(w.order, e); i >= 0 {
		w.order = slices.Delete(w.order, i, i+1)
	}
	w.mu.Unlock()

	return func() {
		if started {
			// Start may still be running; the App shuts down once it is up.
			<-e.ready
			OnShutdown(e.app)
		} else {
			// Start will not run now; release calls waiting for it.
			close(e.ready)
		}
		if cancel != nil {
			cancel()
		}
	}
}

func (w *Windows) lookup(name string) *windowEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.entries[name]
}

func (e *windowEntry) isClosed() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.closed
}

func (e *windowEntry) isStarted() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started
}

// app returns the started App of window name, waiting for it to start.
func (w *Windows) app(ctx context.Context, name string) (*App, error) {
	e := w.lookup(name)
	if e == nil || e.isClosed() {
		return nil, ErrWindowClosed
	}
	select {
	case <-e.ready:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if e.isClosed() {
		return nil, ErrWindowClosed
	}
	return e.app, nil
}

package app

import (
	"errors"
	"fmt"
	"log"
)

// errQuitting refuses a restart while the app is already quitting.
var errQuitting = errors.New("the app is already quitting")

// quitState is the app's quit, coordinated across its windows. Guarded by
// Windows.quitMu.
type quitState struct {
	running  bool // windows are being asked
	approved bool // every window agreed; the app is quitting
	restart  bool // start the app again once it has quit
}

// ShouldQuit reports whether the app may quit now. The toolkit calls it on
// the UI thread for every quit, so it never waits: unless every window has
// already agreed, it asks them in the background and returns false. Once
// they all agree, the app quits again and ShouldQuit returns true.
//
// The windows are asked before any closes because the toolkit closes the
// windows left at quit without their close guards.
func (w *Windows) ShouldQuit() bool {
	w.quitMu.Lock()
	defer w.quitMu.Unlock()
	if w.quitting.approved || !w.anyToAsk() {
		return true
	}
	if !w.quitting.running {
		w.quitting.running = true
		go w.runQuit()
	}
	return false
}

// requestRestart quits the app like ShouldQuit and then starts it again
// with one window.
func (w *Windows) requestRestart() error {
	if _, err := newWindowCommand(); err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	w.quitMu.Lock()
	defer w.quitMu.Unlock()
	if w.quitting.running || w.quitting.approved {
		return fmt.Errorf("restart: %w", errQuitting)
	}
	w.quitting.running = true
	w.quitting.restart = true
	go w.runQuit()
	return nil
}

// runQuit asks the windows, in the order they opened, whether the app may
// quit. Each window with unsaved files asks the user and holds until the
// quit, without closing. One refusal calls the quit off and thaws the
// windows that agreed; otherwise the app quits.
func (w *Windows) runQuit() {
	asked := map[*windowEntry]bool{}
	var held []*App
	for {
		e := w.nextToAsk(asked)
		if e == nil {
			break
		}
		asked[e] = true
		select {
		case <-e.ready:
		case <-e.gone:
			continue
		}
		allow, holds := e.app.approveQuit(e.gone)
		if holds {
			held = append(held, e.app)
		}
		if !allow {
			for _, a := range held {
				a.cancelQuit()
			}
			w.quitMu.Lock()
			w.quitting = quitState{}
			w.quitMu.Unlock()
			return
		}
	}
	w.quitMu.Lock()
	w.quitting.approved = true
	w.quitMu.Unlock()
	if w.quit != nil {
		w.quit()
	}
}

// anyToAsk reports whether a window must still agree to a quit: one that
// has started and is not already closing.
func (w *Windows) anyToAsk() bool {
	return w.nextToAsk(nil) != nil
}

// nextToAsk is the first open window, in the order they opened, that must
// agree to a quit and was not asked yet.
func (w *Windows) nextToAsk(asked map[*windowEntry]bool) *windowEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, e := range w.order {
		if asked[e] || e.closeApproved.Load() || !e.isStarted() || e.isClosed() {
			continue
		}
		return e
	}
	return nil
}

// restartIfApproved starts the app again after an approved restart. Shutdown
// calls it once every window's App has closed, so the new copy finds the
// hosts, indexes and window ring free.
func (w *Windows) restartIfApproved() {
	w.quitMu.Lock()
	restart := w.quitting.approved && w.quitting.restart
	w.quitting.restart = false
	w.quitMu.Unlock()
	if !restart {
		return
	}
	if err := startAnotherCopy(); err != nil {
		log.Printf("agent-sessions: restart: %v", err)
	}
}

//go:build desktop

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// mouseNavigateEvent carries "back" or "forward" to the frontend when the
// mouse's side buttons are handled natively (see hookMouseNavigation).
const mouseNavigateEvent = "nav:mouse"

// swipeNavigateEvent carries "back" or "forward" for a two-finger touchpad
// swipe, spotted natively on every platform. Unlike a button, the swipe was
// also a scroll: the frontend drops it when the scroll moved something.
const swipeNavigateEvent = "nav:swipe"

// AGENT_SESSIONS_TRACE_NAV=1 traces the native side: each event sent to a
// window, and the touchpad gestures (mousenav_linux.c,
// mousenav_windows.go). Windows apps have no console, so there it goes to a
// file (see hookMouseNavigation).
var traceNavigation = os.Getenv("AGENT_SESSIONS_TRACE_NAV") != "" && os.Getenv("AGENT_SESSIONS_TRACE_NAV") != "0"

var navTraceOut io.Writer = os.Stderr

func traceNav(format string, args ...any) {
	if traceNavigation {
		fmt.Fprintf(navTraceOut, "nav-trace "+format+"\n", args...)
	}
}

// dispatchNavigation sends event with direction to target, if any. matched
// says whether target was found from a native window, for the trace.
func dispatchNavigation(target application.Window, event, direction string, matched bool) {
	traceNav("dispatch %s %s to window %v (matched %v)", event, direction, target != nil, matched)
	if target != nil {
		target.DispatchWailsEvent(&application.CustomEvent{Name: event, Data: direction})
	}
}

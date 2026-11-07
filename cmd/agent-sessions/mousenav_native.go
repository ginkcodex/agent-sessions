//go:build (linux || darwin) && desktop

package main

/*
#cgo linux pkg-config: gtk4 webkitgtk-6.0
#cgo darwin LDFLAGS: -framework Cocoa
void hook_side_buttons(void);
void hook_window_side_buttons(void *window);
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// hookMouseNavigation makes the mouse's side buttons go back and forward in
// the app. WebKitGTK and WKWebView do not report them to the page as buttons
// 3 and 4, so the page cannot tell them apart; they are taken natively
// before the web view sees them (mousenav_linux.c, mousenav_darwin.m) and
// sent to the window they happened in as a mouseNavigateEvent. On macOS one
// hook serves every window; on Linux each window is hooked as it opens (see
// hookWindowMouseNavigation).
func hookMouseNavigation() {
	C.hook_side_buttons()
}

// hookWindowMouseNavigation hooks the side buttons of one window's web view
// on Linux; macOS needs nothing per window. It may be called again for the
// same window.
func hookWindowMouseNavigation(window *application.WebviewWindow) {
	application.InvokeAsync(func() {
		if native := window.NativeWindow(); native != nil {
			C.hook_window_side_buttons(native)
		}
	})
}

//export goMouseNavigate
func goMouseNavigate(window unsafe.Pointer, forward C.int) {
	// Off the UI main loop: emitting runs script in the web view.
	go navigateWindow(window, mouseNavigateEvent, navigateDirection(forward))
}

// goSwipeNavigate is a two-finger touchpad swipe (macOS and Linux; on
// Windows the page spots swipes itself).
//
//export goSwipeNavigate
func goSwipeNavigate(window unsafe.Pointer, forward C.int) {
	go navigateWindow(window, swipeNavigateEvent, navigateDirection(forward))
}

// AGENT_SESSIONS_TRACE_NAV=1 also prints each event sent to a window
// (mousenav_linux.c traces the gestures).
var traceNavigation = os.Getenv("AGENT_SESSIONS_TRACE_NAV") != "" && os.Getenv("AGENT_SESSIONS_TRACE_NAV") != "0"

func navigateDirection(forward C.int) string {
	if forward != 0 {
		return "forward"
	}
	return "back"
}

// navigateWindow sends event with direction to the app window whose native
// window is native, or to the focused window when none matches.
func navigateWindow(native unsafe.Pointer, event, direction string) {
	app := application.Get()
	if app == nil {
		return
	}
	var target application.Window
	if native != nil {
		for _, w := range app.Window.GetAll() {
			if ww, ok := w.(*application.WebviewWindow); ok && ww.NativeWindow() == native {
				target = ww
				break
			}
		}
	}
	if target == nil {
		target = app.Window.Current()
	}
	if traceNavigation {
		fmt.Fprintf(os.Stderr, "nav-trace dispatch %s %s to window %v (matched %v)\n", event, direction, target != nil, native != nil)
	}
	if target != nil {
		target.DispatchWailsEvent(&application.CustomEvent{Name: event, Data: direction})
	}
}

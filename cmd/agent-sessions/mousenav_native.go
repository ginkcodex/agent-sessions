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
	direction := "back"
	if forward != 0 {
		direction = "forward"
	}
	// Off the UI main loop: emitting runs script in the web view.
	go navigateWindow(window, direction)
}

// navigateWindow sends direction to the app window whose native window is
// native, or to the focused window when none matches.
func navigateWindow(native unsafe.Pointer, direction string) {
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
	if target != nil {
		target.DispatchWailsEvent(&application.CustomEvent{Name: mouseNavigateEvent, Data: direction})
	}
}

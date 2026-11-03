//go:build (linux || darwin) && desktop

package main

/*
#cgo linux pkg-config: gtk4 webkitgtk-6.0
#cgo darwin LDFLAGS: -framework Cocoa
void hook_side_buttons(void);
*/
import "C"

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// hookMouseNavigation makes the mouse's side buttons go back and forward in
// the app. WebKitGTK and WKWebView do not report them to the page as buttons
// 3 and 4, so the page cannot tell them apart; they are taken natively
// before the web view sees them (mousenav_linux.c, mousenav_darwin.m) and
// sent to the frontend as a mouseNavigateEvent.
func hookMouseNavigation(_ context.Context) {
	C.hook_side_buttons()
}

//export goMouseNavigate
func goMouseNavigate(forward C.int) {
	app := application.Get()
	if app == nil {
		return
	}
	direction := "back"
	if forward != 0 {
		direction = "forward"
	}
	// Off the UI main loop: emitting runs script in the web view.
	go app.Event.Emit(mouseNavigateEvent, direction)
}

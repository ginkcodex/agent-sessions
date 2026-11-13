//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void window_zoom_now(void *window);
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// restoreMaximised maximises a window that was closed maximised, before it is
// first shown, without the resize animation Wails' Maximise runs: that
// animation only advances while the window is on screen, so a pre-show
// Maximise shows the window at its normal size and then grows it. AppKit's
// own zoom lands the maximised frame at once, and un-maximising still
// returns to the frame the window had. Must not run on the main thread.
func restoreMaximised(window *application.WebviewWindow) {
	application.InvokeSync(func() {
		if native := window.NativeWindow(); native != nil {
			C.window_zoom_now(native)
		}
	})
}

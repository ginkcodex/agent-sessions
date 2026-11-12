//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void make_window_opaque(void *window);
*/
import "C"

import (
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// opaqueWindowExperimentEnabled permits a same-binary comparison against Wails'
// original non-opaque window. This temporary switch is not a persisted setting.
func opaqueWindowExperimentEnabled() bool {
	return os.Getenv("AGENT_SESSIONS_MACOS_OPAQUE") != "0"
}

// configureWindowOpacity gives the solid main window an opaque native backing.
// The runtime-ready event means the native window exists; resolve its handle on
// the UI thread so a queued call after window teardown safely becomes a no-op.
// No webview/child-layer opacity or transparency is changed.
func configureWindowOpacity(window *application.WebviewWindow) {
	if !opaqueWindowExperimentEnabled() {
		return
	}
	application.InvokeAsync(func() {
		if native := window.NativeWindow(); native != nil {
			C.make_window_opaque(native)
		}
	})
}

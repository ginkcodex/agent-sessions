//go:build !darwin && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// restoreMaximised maximises a window that was closed maximised. Only macOS,
// which does it before the window is first shown, needs a path of its own
// (see windowzoom_darwin.go).
func restoreMaximised(window *application.WebviewWindow) {
	window.Maximise()
}

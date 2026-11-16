//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void set_window_theme(void *window, int appearance);
*/
import "C"

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func setWindowTheme(window *application.WebviewWindow, mode string) error {
	appearance := C.int(0)
	switch mode {
	case "light":
		appearance = 1
	case "dark":
		appearance = 2
	}
	var err error
	application.InvokeSync(func() {
		// Resolve on the UI thread: the window may have closed while queued.
		if native := window.NativeWindow(); native != nil {
			C.set_window_theme(native, appearance)
		} else {
			err = errors.New("window theme: the window is not ready")
		}
	})
	return err
}

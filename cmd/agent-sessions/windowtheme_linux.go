//go:build linux && desktop

package main

/*
#cgo pkg-config: gtk4
int set_linux_window_theme(void *window, int appearance);
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
		if native := window.NativeWindow(); native == nil || C.set_linux_window_theme(native, appearance) == 0 {
			err = errors.New("window theme: the GTK window is not ready")
		}
	})
	return err
}

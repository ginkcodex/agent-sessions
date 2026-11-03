//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit
int print_web_view(void *nsWindow);
*/
import "C"

import (
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// printWindow prints the window in portrait with margins (print_darwin.m);
// Wails' own Print forces landscape with no margins on macOS.
func printWindow(window *application.WebviewWindow) error {
	switch C.print_web_view(window.NativeWindow()) {
	case 0:
		return nil
	case -2:
		return errors.New("print: the window is closed")
	default:
		return errors.New("print: printing needs macOS 11 or later")
	}
}

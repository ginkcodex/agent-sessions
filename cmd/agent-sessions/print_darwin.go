//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa -framework WebKit
int print_web_view(void *nsWindow, int letter, int landscape, double margin_mm);
*/
import "C"

import (
	"errors"

	"github.com/ginkcode/agent-sessions/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails' own Print forces landscape with no margins on macOS.
func printWindow(window *application.WebviewWindow, setup app.PrintPageSetup) error {
	return application.InvokeSyncWithError(func() error {
		var letter, landscape C.int
		if setup.Paper == "letter" {
			letter = 1
		}
		if setup.Orientation == "landscape" {
			landscape = 1
		}
		switch C.print_web_view(window.NativeWindow(), letter, landscape, C.double(setup.MarginMm)) {
		case 0:
			return nil
		case -2:
			return errors.New("print: the window is closed")
		case -3:
			return errors.New("print: no web view to print")
		default:
			return errors.New("print: printing needs macOS 11 or later")
		}
	})
}

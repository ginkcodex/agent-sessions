package app

import (
	"errors"
	"fmt"
)

// nativePrint replaces the desktop's Print where the desktop app prints
// natively (macOS); nil elsewhere.
var nativePrint func() error

// SetNativePrint sets how PrintWindow opens the print dialog; nil uses
// Desktop.Print. It is a package function so it is not bound to JS.
func SetNativePrint(fn func() error) {
	nativePrint = fn
}

// PrintWindow opens the system print dialog for the window, where it can
// also be saved as a PDF. The page's print styles decide what is printed.
// macOS needs this: its web view ignores window.print().
func (a *App) PrintWindow() error {
	if a.ctx == nil {
		return errors.New("print: the window is not ready")
	}
	if nativePrint != nil {
		return nativePrint()
	}
	if fn := desktopFrom(a.ctx).Print; fn != nil {
		return fn(a.ctx)
	}
	return fmt.Errorf("print: %w", ErrDesktopUnavailable)
}

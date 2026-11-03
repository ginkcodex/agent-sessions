package app

import (
	"errors"
	"fmt"
)

// PrintWindow opens the system print dialog for the window, where it can
// also be saved as a PDF. The page's print styles decide what is printed.
// macOS needs this: its web view ignores window.print().
func (a *App) PrintWindow() error {
	if a.ctx == nil {
		return errors.New("print: the window is not ready")
	}
	if fn := desktopFrom(a.ctx).Print; fn != nil {
		return fn(a.ctx)
	}
	return fmt.Errorf("print: %w", ErrDesktopUnavailable)
}

package app

import (
	"context"
	"errors"

	"github.com/ginkcode/agent-sessions/internal/engine"
)

// ErrDesktopUnavailable is returned by native operations, such as dialogs and
// printing, when no desktop shell has been injected.
var ErrDesktopUnavailable = errors.New("this action needs the desktop app")

// FileFilter is one entry of a native file dialog's type list. Pattern holds
// semicolon-separated globs such as "*.agent-session.zip;*.zip".
type FileFilter struct {
	DisplayName string
	Pattern     string
}

// OpenDialogOptions describes a native picker for one file or one directory.
type OpenDialogOptions struct {
	Title                string
	DefaultDirectory     string
	DefaultFilename      string
	Filters              []FileFilter
	CanCreateDirectories bool
}

// SaveDialogOptions describes a native save dialog.
type SaveDialogOptions struct {
	Title            string
	DefaultDirectory string
	DefaultFilename  string
	Filters          []FileFilter
}

// DialogType is the kind of a native message dialog.
type DialogType string

// ErrorDialog shows an error message with a single OK button.
const ErrorDialog DialogType = "error"

// MessageDialogOptions describes a native message dialog.
type MessageDialogOptions struct {
	Type    DialogType
	Title   string
	Message string
}

// Desktop is everything this package needs from the desktop shell. The shell
// builds it from its GUI toolkit and passes it to OnStartup; this package never
// imports the toolkit, so default builds stay GUI-free.
//
// Every field may be nil. Missing event and window callbacks do nothing,
// missing dialogs and Print return ErrDesktopUnavailable, and a nil
// Notifications reports alerts as unavailable.
//
// Dialogs return "" with a nil error when the user cancels. RequestClose must
// ask the window to close through its normal, guarded close path (the one
// that runs OnBeforeClose); it must not force the application to quit.
type Desktop struct {
	// Emit sends a named event with a JSON-serialisable payload to the frontend.
	Emit func(name string, payload any)

	OpenFile      func(ctx context.Context, opts OpenDialogOptions) (string, error)
	OpenDirectory func(ctx context.Context, opts OpenDialogOptions) (string, error)
	SaveFile      func(ctx context.Context, opts SaveDialogOptions) (string, error)
	Message       func(ctx context.Context, opts MessageDialogOptions) (string, error)

	IsFullscreen func(ctx context.Context) bool
	IsMaximised  func(ctx context.Context) bool
	Fullscreen   func(ctx context.Context)
	Unfullscreen func(ctx context.Context)
	Maximise     func(ctx context.Context)
	Unmaximise   func(ctx context.Context)

	// Raise brings the window to the front, restoring it when minimised. It is
	// called from background goroutines; the shell dispatches to the UI thread.
	Raise func(ctx context.Context)
	// RequestClose asks the window to close as if the user closed it.
	RequestClose func(ctx context.Context)
	// Print opens the system print dialog for the window.
	Print func(ctx context.Context) error
	// OpenWindow opens another app window in this process.
	OpenWindow func() error
	// ShowAbout opens the About window, or brings the open one to the front.
	ShowAbout func()
	// Restart quits the app, once every window agrees, and starts it again.
	Restart func() error

	Notifications Notifier
}

type desktopContextKey struct{}

// withDesktop returns ctx carrying desktop. OnStartup stores the result as the
// app context, so everything reached through appCtx sees the same adapter.
func withDesktop(ctx context.Context, desktop Desktop) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, desktopContextKey{}, desktop)
}

// desktopFrom returns the adapter in ctx, or an empty one (all defaults).
func desktopFrom(ctx context.Context) Desktop {
	if ctx != nil {
		if d, ok := ctx.Value(desktopContextKey{}).(Desktop); ok {
			return d
		}
	}
	return Desktop{}
}

// desktopEmitter forwards engine events to the injected Desktop.Emit.
type desktopEmitter struct{ emit func(string, any) }

func (e desktopEmitter) Emit(name string, payload any) {
	if e.emit != nil {
		e.emit(name, payload)
	}
}

// emitterFrom is the explicitly injected event emitter in ctx; it does
// nothing without a desktop.
func emitterFrom(ctx context.Context) engine.Emitter {
	return desktopEmitter{emit: desktopFrom(ctx).Emit}
}

func openFileDialog(ctx context.Context, opts OpenDialogOptions) (string, error) {
	if fn := desktopFrom(ctx).OpenFile; fn != nil {
		return fn(ctx, opts)
	}
	return "", ErrDesktopUnavailable
}

func openDirectoryDialog(ctx context.Context, opts OpenDialogOptions) (string, error) {
	if fn := desktopFrom(ctx).OpenDirectory; fn != nil {
		return fn(ctx, opts)
	}
	return "", ErrDesktopUnavailable
}

func saveFileDialog(ctx context.Context, opts SaveDialogOptions) (string, error) {
	if fn := desktopFrom(ctx).SaveFile; fn != nil {
		return fn(ctx, opts)
	}
	return "", ErrDesktopUnavailable
}

func messageDialog(ctx context.Context, opts MessageDialogOptions) (string, error) {
	if fn := desktopFrom(ctx).Message; fn != nil {
		return fn(ctx, opts)
	}
	return "", ErrDesktopUnavailable
}

// requestWindowClose asks the window to close through the guarded path.
func requestWindowClose(ctx context.Context) {
	if fn := desktopFrom(ctx).RequestClose; fn != nil {
		fn(ctx)
	}
}

// raiseWindow brings this app window to the front, restoring it when
// minimised. The platform differences live in the shell's Raise.
func raiseWindow(ctx context.Context) {
	if fn := desktopFrom(ctx).Raise; fn != nil {
		fn(ctx)
	}
}

// Lifecycle entry points for the desktop shell. They are package functions,
// not App methods, so the binding generator never exposes them to JS.

// OnStartup starts the local engine and desktop event routing. The shell calls
// it once, before the frontend makes any binding call.
func OnStartup(a *App, ctx context.Context, desktop Desktop) {
	a.onStartup(withDesktop(ctx, desktop))
}

// OnBeforeClose coordinates a native window close with unsaved editors. It
// returns true to cancel the close. The shell calls it exactly once per close
// attempt, from the window's closing hook, and never blocks on the frontend.
func OnBeforeClose(a *App) bool {
	return a.onBeforeClose(a.appCtx())
}

// OnShutdown releases notifications, terminals, connections, watchers and the
// engine, in that order. The shell calls it once when the app terminates.
func OnShutdown(a *App) {
	a.onShutdown()
}

// Close stops background scans, watchers, indexers and the database without
// touching desktop notifications. Non-desktop callers use it instead of
// OnShutdown.
func Close(a *App) error {
	return a.close()
}

//go:build desktop

package main

import (
	"context"
	"runtime"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/ginkcode/agent-sessions/internal/app"
)

// newNotificationHub shares the desktop's notifications, a process-wide
// Wails service, among the windows. ctx returns the application's context;
// it is first called when a window sends a notification.
func newNotificationHub(ctx func() context.Context) *app.NotificationHub {
	return app.NewNotificationHub(&desktopNotifier{service: notifications.New(), ctx: ctx})
}

// newDesktop adapts the Wails v3 application and one window to what
// internal/app needs from the desktop. app.Windows adds the shared
// notifications and New Window.
func newDesktop(desktop *application.App, window *application.WebviewWindow) app.Desktop {
	return app.Desktop{
		// Each window's events go to that window only: the App behind it
		// numbers its terminals and prompts on its own.
		Emit: func(name string, payload any) {
			window.DispatchWailsEvent(&application.CustomEvent{Name: name, Data: payload})
		},

		OpenFile: func(_ context.Context, opts app.OpenDialogOptions) (string, error) {
			return desktop.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				CanChooseFiles:       true,
				CanCreateDirectories: opts.CanCreateDirectories,
				Title:                opts.Title,
				Directory:            opts.DefaultDirectory,
				Filters:              fileFilters(opts.Filters),
				Window:               window,
			}).PromptForSingleSelection()
		},
		OpenDirectory: func(_ context.Context, opts app.OpenDialogOptions) (string, error) {
			return desktop.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
				CanChooseDirectories: true,
				CanCreateDirectories: opts.CanCreateDirectories,
				Title:                opts.Title,
				Directory:            opts.DefaultDirectory,
				Window:               window,
			}).PromptForSingleSelection()
		},
		SaveFile: func(_ context.Context, opts app.SaveDialogOptions) (string, error) {
			return desktop.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
				CanCreateDirectories: true,
				Title:                opts.Title,
				Directory:            opts.DefaultDirectory,
				Filename:             opts.DefaultFilename,
				Filters:              fileFilters(opts.Filters),
				Window:               window,
			}).PromptForSingleSelection()
		},
		Message: func(_ context.Context, opts app.MessageDialogOptions) (string, error) {
			dialog := desktop.Dialog.Info()
			if opts.Type == app.ErrorDialog {
				dialog = desktop.Dialog.Error()
			}
			dialog.SetTitle(opts.Title).SetMessage(opts.Message).AttachToWindow(window).Show()
			return "", nil
		},

		IsFullscreen: func(context.Context) bool { return window.IsFullscreen() },
		IsMaximised:  func(context.Context) bool { return window.IsMaximised() },
		Fullscreen:   func(context.Context) { window.Fullscreen() },
		Unfullscreen: func(context.Context) { window.UnFullscreen() },
		Maximise:     func(context.Context) { window.Maximise() },
		Unmaximise:   func(context.Context) { window.UnMaximise() },
		Raise:        func(context.Context) { raise(window) },
		// Close emits WindowClosing, so the close guard decides.
		RequestClose: func(context.Context) { window.Close() },
		Print:        func(context.Context) error { return printWindow(window) },
	}
}

// raise brings the window to the front. On Linux unminimising presents the
// window; elsewhere a maximised window must keep its size.
func raise(window *application.WebviewWindow) {
	if runtime.GOOS == "linux" || window.IsMinimised() {
		window.UnMinimise()
	}
	window.Show()
	window.Focus()
}

func fileFilters(filters []app.FileFilter) []application.FileFilter {
	out := make([]application.FileFilter, 0, len(filters))
	for _, f := range filters {
		out = append(out, application.FileFilter{DisplayName: f.DisplayName, Pattern: f.Pattern})
	}
	return out
}

// desktopNotifier runs the Wails notifications service without registering it
// as a bound service, so the page cannot send notifications itself.
type desktopNotifier struct {
	service *notifications.NotificationService
	ctx     func() context.Context

	mu      sync.Mutex
	started bool
}

func (n *desktopNotifier) Available() bool { return n.service != nil }

func (n *desktopNotifier) Initialize() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started {
		return nil
	}
	if err := n.service.ServiceStartup(n.ctx(), application.ServiceOptions{}); err != nil {
		return err
	}
	n.started = true
	return nil
}

func (n *desktopNotifier) Authorized() (bool, error) {
	return n.service.CheckNotificationAuthorization()
}

func (n *desktopNotifier) RequestAuthorization() (bool, error) {
	return n.service.RequestNotificationAuthorization()
}

func (n *desktopNotifier) Send(id, title, body string) error {
	return n.service.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: body})
}

func (n *desktopNotifier) Remove(id string) error { return n.service.RemoveNotification(id) }

func (n *desktopNotifier) OnClick(fn func(id string)) {
	n.service.OnNotificationResponse(func(result notifications.NotificationResult) {
		if result.Error == nil {
			fn(result.Response.ID)
		}
	})
}

func (n *desktopNotifier) Cleanup() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started {
		_ = n.service.ServiceShutdown()
		n.started = false
	}
}

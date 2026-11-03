//go:build desktop

package main

import (
	"context"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ginkcode/agent-sessions/frontend"
	"github.com/ginkcode/agent-sessions/internal/app"
)

// shell opens the app's windows and connects each one to its own App in
// app.Windows.
type shell struct {
	desktop *application.App
	windows *app.Windows

	mu sync.Mutex
	// running is set once the application has started. Windows opened
	// before then wait in pending and start with it.
	running bool
	pending []shellWindow
}

type shellWindow struct {
	name   string
	window *application.WebviewWindow
}

// callingWindow names the window that made a bound call; Wails puts it in
// the call's context.
func callingWindow(ctx context.Context) string {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok && w != nil {
		return w.Name()
	}
	return ""
}

// openWindow opens a window with an App of its own.
func (s *shell) openWindow() error {
	name := s.windows.Reserve()
	window := s.desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      name,
		Title:     "Agent Sessions",
		Width:     1280,
		Height:    820,
		MinWidth:  960,
		MinHeight: 600,
		// Files dropped on the terminal panel arrive as paths. Only elements
		// marked data-file-drop-target receive them.
		EnableFileDrop: true,
		Mac: application.MacWindow{
			// Escape belongs to in-app dialogs, not to fullscreen.
			DisableEscapeExitsFullscreen: true,
		},
		Linux: application.LinuxWindow{
			Icon:             frontend.AppIcon,
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	})
	// The hook runs before the window closes and may keep it open; the
	// listener runs only once the window really closes.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if s.windows.BeforeClose(name) {
			event.Cancel()
		}
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.windows.Closed(name)
	})

	// Wails hands dropped paths only to Go; pass them on to the page, which
	// works out the terminal under the CSS-pixel drop point.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		target := event.Context().DropTargetDetails()
		if len(files) == 0 || target == nil {
			return
		}
		window.DispatchWailsEvent(&application.CustomEvent{
			Name: filesDroppedEvent,
			Data: map[string]any{"filenames": files, "x": target.X, "y": target.Y},
		})
	})

	w := shellWindow{name: name, window: window}
	s.mu.Lock()
	running := s.running
	if !running {
		s.pending = append(s.pending, w)
	}
	s.mu.Unlock()
	if running {
		go s.startWindow(w)
	}
	return nil
}

// start starts the windows opened before the application ran. The shell
// calls it once the application has started.
func (s *shell) start() {
	s.mu.Lock()
	s.running = true
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()
	for _, w := range pending {
		s.startWindow(w)
	}
}

func (s *shell) startWindow(w shellWindow) {
	s.windows.Start(w.name, s.desktop.Context(), newDesktop(s.desktop, w.window))
}

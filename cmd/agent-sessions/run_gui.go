//go:build desktop

package main

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ginkcode/agent-sessions/frontend"
	"github.com/ginkcode/agent-sessions/internal/app"
)

// desktopMarker tells the frontend, before any module runs, that it is in
// the desktop app and must use the native bridge rather than demo data.
const desktopMarker = `<script>window.__AGENT_SESSIONS_DESKTOP__ = true;</script>`

// filesDroppedEvent carries files dropped on a terminal to the frontend
// (see FILES_DROPPED_EVENT in frontend/src/lib/desktopRuntime.ts).
const filesDroppedEvent = "files:dropped"

// run launches the Wails v3 desktop application. It is only compiled with the
// desktop build tag so default builds and tests never need GTK or WebKit.
func run() {
	service := app.NewApp()
	app.SetNativePrint(nativePrinter())
	app.SetNativeTerminalFonts(nativeTerminalFonts)
	hideSiriAffordance()

	// The embedded FS is rooted at "dist"; the asset server expects
	// index.html at the FS root, so serve the "dist" subtree.
	assets, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}

	var window *application.WebviewWindow
	var hub *app.NotificationHub
	// closeApproved lets the app quit after the close guard has approved
	// closing the only window. Before that, an OS quit becomes a window close
	// so it goes through the same guard.
	var closeApproved atomic.Bool
	desktop := application.New(application.Options{
		Name:        "Agent Sessions",
		Description: "Browse and manage agentic coding tool sessions",
		Icon:        frontend.AppIcon,
		Services:    []application.Service{application.NewService(service)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: injectDesktopMarker,
		},
		ShouldQuit: func() bool {
			if closeApproved.Load() || window == nil {
				return true
			}
			go window.Close()
			return false
		},
		OnShutdown: func() {
			app.OnShutdown(service)
			hub.Close()
		},
		Mac: application.MacOptions{
			// Each process owns one window. After the close guard approves,
			// terminate instead of leaving a windowless process in the Dock.
			// This also completes Cmd+Q redirected through ShouldQuit above.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			ProgramName: "agent-sessions",
		},
	})

	hub = newNotificationHub(desktop)
	window = desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
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
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if app.OnBeforeClose(service) {
			event.Cancel()
			return
		}
		closeApproved.Store(true)
	})

	// Wails hands dropped paths only to Go; pass them on to the page, which
	// works out the terminal under the CSS-pixel drop point.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		target := event.Context().DropTargetDetails()
		if len(files) == 0 || target == nil {
			return
		}
		desktop.Event.Emit(filesDroppedEvent, map[string]any{
			"filenames": files, "x": target.X, "y": target.Y,
		})
	})

	var started atomic.Bool
	desktop.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if !started.CompareAndSwap(false, true) {
			return
		}
		// GTK is initialized by now and the window exists.
		disableFontHinting()
		hookMouseNavigation(context.Background())
		app.OnStartup(service, desktop.Context(), newDesktop(desktop, window, hub.View()))
		hookWindowCycling(func(forward bool) {
			_ = service.FocusNextWindow(forward)
		})
	})

	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}

// injectDesktopMarker adds desktopMarker to the app's HTML page.
func injectDesktopMarker(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		body := rec.body.Bytes()
		if rec.status == http.StatusOK && strings.HasPrefix(rec.header.Get("Content-Type"), "text/html") {
			body = withDesktopMarker(body)
			rec.header.Del("Content-Length")
			rec.header.Set("Content-Length", strconv.Itoa(len(body)))
		}
		for k, values := range rec.header {
			w.Header()[k] = values
		}
		w.WriteHeader(rec.status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
	})
}

// withDesktopMarker inserts desktopMarker at the start of <head>, before the
// page's module scripts.
func withDesktopMarker(page []byte) []byte {
	head := bytes.Index(bytes.ToLower(page), []byte("<head>"))
	if head < 0 {
		return append([]byte(desktopMarker), page...)
	}
	at := head + len("<head>")
	out := make([]byte, 0, len(page)+len(desktopMarker))
	out = append(out, page[:at]...)
	out = append(out, desktopMarker...)
	return append(out, page[at:]...)
}

type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) { b.status = status }

func (b *bufferedResponse) Write(p []byte) (int, error) { return b.body.Write(p) }

var _ io.Writer = (*bufferedResponse)(nil)

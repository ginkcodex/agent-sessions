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
	app.SetNativeTerminalFonts(nativeTerminalFonts)
	hideSiriAffordance()

	// The embedded FS is rooted at "dist"; the asset server expects
	// index.html at the FS root, so serve the "dist" subtree.
	assets, err := fs.Sub(frontend.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}

	s := &shell{}
	hub := newNotificationHub(func() context.Context { return s.desktop.Context() })
	s.windows = app.NewWindows(hub, s.openWindow, func() { s.desktop.Quit() })
	s.desktop = application.New(application.Options{
		Name:        "Agent Sessions",
		Description: "Browse and manage agentic coding tool sessions",
		Icon:        frontend.AppIcon,
		// The frontend calls every App method through the router, which
		// finds the App of the calling window.
		Services: []application.Service{application.NewService(app.NewRouter(s.windows, callingWindow))},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: injectDesktopMarker,
		},
		// A quit first asks every window about its unsaved files; the
		// windows agree, then the app quits again.
		ShouldQuit: s.windows.ShouldQuit,
		OnShutdown: s.windows.Shutdown,
		Mac: application.MacOptions{
			// Once the close guards let the last window close, terminate
			// instead of leaving a windowless app in the Dock.
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			ApplicationID: linuxAppID,
			ProgramName:   linuxProgramName,
		},
	})
	if err := s.openWindow(); err != nil {
		log.Fatal(err)
	}

	var started atomic.Bool
	s.desktop.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		if !started.CompareAndSwap(false, true) {
			return
		}
		// GTK is initialized by now and the window exists.
		disableFontHinting()
		hookMouseNavigation()
		s.start()
		hookWindowCycling(func(forward bool) {
			if w := s.desktop.Window.Current(); w != nil {
				_ = s.windows.FocusNextWindow(w.Name(), forward)
			}
		})
	})

	if err := s.desktop.Run(); err != nil {
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

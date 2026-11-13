//go:build desktop

package main

import (
	"log"
	"net/url"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ginkcode/agent-sessions/frontend"
	"github.com/ginkcode/agent-sessions/internal/version"
)

// aboutPage is the About window's page, built by Vite next to index.html.
const aboutPage = "/about.html"

// aboutWindowName names the About window; app windows are named by
// app.Windows.
const aboutWindowName = "about"

// aboutURL is where the About window opens, with the version for the page
// to show.
func aboutURL(v string) string {
	return aboutPage + "?" + url.Values{"version": {v}}.Encode()
}

// checkUpdatesEvent asks a window to open Settings and check for updates. It
// is the macOS app menu's Check for Updates… item (see useAboutWindow).
const checkUpdatesEvent = "update:check"

// useAboutWindow points the macOS app menu's About item at the About window
// instead of the standard About panel, keeping the rest of the default
// menu, and adds Check for Updates… below it. Windows and Linux have no app
// menu: the sidebar's logo opens About there (App.ShowAbout), and Settings
// has the update check.
func (s *shell) useAboutWindow() {
	if runtime.GOOS != "darwin" {
		return
	}
	menu := application.DefaultApplicationMenu()
	if replaceAboutItem(menu, s.showAbout, s.checkForUpdates) {
		s.desktop.Menu.Set(menu)
	}
}

// replaceAboutItem swaps the app menu's About item for a plain one that calls
// show, followed by a Check for Updates… item that calls check. The
// toolkit's About item sends AppKit's orderFrontStandardAboutPanel: and never
// runs its click handler, so setting one on it changes nothing.
func replaceAboutItem(menu *application.Menu, show, check func()) bool {
	about := menu.FindByRole(application.About)
	if about == nil {
		return false
	}
	for i := 0; menu.ItemAt(i) != nil; i++ {
		appMenu := menu.ItemAt(i).GetSubmenu()
		if appMenu == nil || appMenu.ItemAt(0) != about {
			continue
		}
		appMenu.RemoveMenuItem(about)
		items := application.NewMenu()
		items.Add(about.Label()).OnClick(func(*application.Context) { show() })
		items.Add(checkUpdatesLabel).OnClick(func(*application.Context) { check() })
		appMenu.Prepend(items)
		return true
	}
	return false
}

const checkUpdatesLabel = "Check for Updates…"

// updateWindow picks the window that shows the update check: the focused
// one, else any other app window. It is nil when only the About window is
// open.
func updateWindow(current application.Window, all []application.Window) application.Window {
	if current != nil && current.Name() != aboutWindowName {
		return current
	}
	for _, w := range all {
		if w.Name() != aboutWindowName {
			return w
		}
	}
	return nil
}

// checkForUpdates asks an app window to open Settings and check. With no app
// window open (only About is), it opens one: that window's own startup check
// follows, so the request itself is dropped.
func (s *shell) checkForUpdates() {
	target := updateWindow(s.desktop.Window.Current(), s.desktop.Window.GetAll())
	if target == nil {
		if err := s.openWindow(); err != nil {
			log.Printf("agent-sessions: open a window to check for updates: %v", err)
		}
		return
	}
	if w, ok := target.(*application.WebviewWindow); ok {
		raise(w)
	}
	target.DispatchWailsEvent(&application.CustomEvent{Name: checkUpdatesEvent})
}

// showAbout opens the About window, or brings the open one to the front.
// Each menu click runs on a goroutine of its own, so aboutMu is held
// throughout: two quick clicks must not open two windows.
func (s *shell) showAbout() {
	s.aboutMu.Lock()
	defer s.aboutMu.Unlock()
	if s.about != nil {
		raise(s.about)
		return
	}
	window := s.desktop.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                aboutWindowName,
		Title:               "About Agent Sessions",
		URL:                 aboutURL(version.Current()),
		Width:               380,
		Height:              360,
		DisableResize:       true,
		InitialPosition:     application.WindowCentered,
		MinimiseButtonState: application.ButtonDisabled,
		MaximiseButtonState: application.ButtonDisabled,
		Mac: application.MacWindow{
			DisableEscapeExitsFullscreen: true,
		},
		Linux: application.LinuxWindow{
			Icon:             frontend.AppIcon,
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	})
	// The listener runs on its own goroutine once the window has closed.
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.aboutMu.Lock()
		defer s.aboutMu.Unlock()
		if s.about == window {
			s.about = nil
		}
	})
	s.about = window
	window.Show()
}

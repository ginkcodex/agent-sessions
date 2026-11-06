//go:build desktop

package main

import (
	"net/url"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ginkcode/agent-sessions/frontend"
	"github.com/ginkcode/agent-sessions/internal/version"
)

// aboutPage is the About window's page, built by Vite next to index.html.
const aboutPage = "/about.html"

// aboutURL is where the About window opens, with the version for the page
// to show.
func aboutURL(v string) string {
	return aboutPage + "?" + url.Values{"version": {v}}.Encode()
}

// useAboutWindow points the macOS app menu's About item at the About window
// instead of the standard About panel, keeping the rest of the default
// menu. Windows and Linux have no app menu: the sidebar's logo opens it there
// (App.ShowAbout).
func (s *shell) useAboutWindow() {
	if runtime.GOOS != "darwin" {
		return
	}
	menu := application.DefaultApplicationMenu()
	if replaceAboutItem(menu, s.showAbout) {
		s.desktop.Menu.Set(menu)
	}
}

// replaceAboutItem swaps the app menu's About item for a plain one that calls
// show. The toolkit's About item sends AppKit's
// orderFrontStandardAboutPanel: and never runs its click handler, so setting
// one on it changes nothing.
func replaceAboutItem(menu *application.Menu, show func()) bool {
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
		item := application.NewMenu()
		item.Add(about.Label()).OnClick(func(*application.Context) { show() })
		appMenu.Prepend(item)
		return true
	}
	return false
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
		Name:                "about",
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

//go:build desktop

package main

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestAboutURLCarriesTheVersion(t *testing.T) {
	if got, want := aboutURL("1.2.2"), "/about.html?version=1.2.2"; got != want {
		t.Fatalf("aboutURL = %q, want %q", got, want)
	}
	if got, want := aboutURL("1.3.0-rc.1+abc"), "/about.html?version=1.3.0-rc.1%2Babc"; got != want {
		t.Fatalf("aboutURL = %q, want %q", got, want)
	}
}

func TestDesktopMarkerReachesTheAboutPage(t *testing.T) {
	page := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><head><title>x</title></head></html>"))
	})
	for path, want := range map[string]bool{"/": true, "/index.html": true, "/about.html": true, "/other.html": false} {
		rec := httptest.NewRecorder()
		injectDesktopMarker(page).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := strings.Contains(rec.Body.String(), desktopMarker); got != want {
			t.Errorf("%s: marker injected = %v, want %v", path, got, want)
		}
	}
}

func TestAppMenuAboutOpensTheAboutWindow(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS has an app menu")
	}
	application.New(application.Options{Name: "Agent Sessions"})
	menu := application.DefaultApplicationMenu()
	if !replaceAboutItem(menu, func() {}) {
		t.Fatal("the app menu has no About item to replace")
	}
	// The About role sends AppKit's standard panel and skips click handlers.
	if menu.FindByRole(application.About) != nil {
		t.Fatal("the About role item is still in the menu")
	}
	appMenu := menu.ItemAt(0).GetSubmenu()
	if got := appMenu.ItemAt(0).Label(); got != "About Agent Sessions" {
		t.Fatalf("first app menu item = %q, want About Agent Sessions", got)
	}
	if appMenu.FindByRole(application.Quit) == nil {
		t.Fatal("the rest of the app menu is gone")
	}
}

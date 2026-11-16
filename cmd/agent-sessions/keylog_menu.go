//go:build desktop

package main

import (
	"log"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ginkcode/agent-sessions/internal/keylog"
	"github.com/ginkcode/agent-sessions/internal/version"
)

const (
	recordKeyboardLabel  = "Record Keyboard Events"
	saveKeyboardLogLabel = "Save Keyboard Log"
	// learnMoreLabel is the toolkit's Help item; it would load the toolkit's
	// website into the app window.
	learnMoreLabel = "Learn More"
)

// addKeyboardLogItems puts the keyboard diagnostics controls in the Help menu
// (docs/plan/keyboard-diagnostics.md), in place of the toolkit's Learn More.
// It reports whether the menu changed.
func addKeyboardLogItems(menu *application.Menu, rec *keylog.Recorder, save func()) bool {
	help := helpMenu(menu)
	if help == nil {
		return false
	}
	if learn := help.FindByLabel(learnMoreLabel); learn != nil {
		help.RemoveMenuItem(learn)
	}
	help.AddCheckbox(recordKeyboardLabel, rec.Enabled()).OnClick(func(ctx *application.Context) {
		rec.SetEnabled(ctx.IsChecked())
	})
	help.Add(saveKeyboardLogLabel).OnClick(func(*application.Context) { save() })
	return true
}

// helpMenu is the menu bar's Help submenu, or nil.
func helpMenu(menu *application.Menu) *application.Menu {
	for i := 0; menu.ItemAt(i) != nil; i++ {
		item := menu.ItemAt(i)
		if item.Label() == "Help" && item.GetSubmenu() != nil {
			return item.GetSubmenu()
		}
	}
	return nil
}

// saveKeyboardLog writes the recorded events, with the text-input state as it
// is now, and tells the user where. It takes the state first: the dialog
// that follows changes the key window. It reveals nothing in Finder, which
// would make the app inactive and may clear the fault being studied.
func (s *shell) saveKeyboardLog() {
	state := keyboardStateSnapshot()
	dir, err := keylog.DefaultDir()
	path := ""
	if err == nil {
		path, err = s.windows.KeyLog().Save(dir, keylog.Header{
			App:   version.Current(),
			OS:    runtime.GOOS,
			Arch:  runtime.GOARCH,
			State: state,
		})
	}
	if err != nil {
		log.Printf("agent-sessions: save keyboard log: %v", err)
		s.desktop.Dialog.Error().SetTitle("Keyboard Log Not Saved").SetMessage(err.Error()).Show()
		return
	}
	s.desktop.Dialog.Info().SetTitle("Keyboard Log Saved").SetMessage(path).Show()
}

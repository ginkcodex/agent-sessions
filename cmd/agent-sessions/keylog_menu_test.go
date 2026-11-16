//go:build desktop

package main

import (
	"runtime"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/ginkcode/agent-sessions/internal/keylog"
)

func TestHelpMenuHasKeyboardLogControls(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS has the menu bar")
	}
	application.New(application.Options{Name: "Agent Sessions"})
	menu := application.DefaultApplicationMenu()
	rec := keylog.New(4)
	saved := 0
	if !addKeyboardLogItems(menu, rec, func() { saved++ }) {
		t.Fatal("the menu has no Help menu")
	}
	help := helpMenu(menu)
	if help.FindByLabel(learnMoreLabel) != nil {
		t.Fatal("Learn More would load the toolkit's website into the app window")
	}
	toggle := help.FindByLabel(recordKeyboardLabel)
	if toggle == nil || !toggle.IsCheckbox() || toggle.Checked() {
		t.Fatalf("record item = %+v, want an unchecked checkbox", toggle)
	}
	if help.FindByLabel(saveKeyboardLogLabel) == nil {
		t.Fatal("no save item")
	}
	if rec.Enabled() {
		t.Fatal("recording is on before the user turned it on")
	}
}

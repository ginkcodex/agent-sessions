package main

import "fmt"

type wizardPrompt int

const (
	promptConsent wizardPrompt = iota
	promptClose
)

// All calls originate on the setup worker; the view marshals them to its UI thread.
type wizardView interface {
	Show(title, body string)
	Ask(kind wizardPrompt, text string) bool
}

// guidedPlatform changes presentation only. runSetup remains responsible for
// consent, revalidation and the separate uninstall/install transactions.
type guidedPlatform struct {
	platform
	ui wizardView
}

func (p *guidedPlatform) Prepare() (payload, error) {
	p.ui.Show("Preparing setup", "Checking the setup package and installation requirements…")
	return p.platform.Prepare()
}

func (p *guidedPlatform) Confirm(text string) bool {
	return p.ui.Ask(promptConsent, text)
}

func (p *guidedPlatform) RetryClose() bool {
	return p.ui.Ask(promptClose, "Close every Agent Sessions window normally before continuing. Setup will not terminate the app.\n\nClick Retry after closing the app, or Cancel to stop. If a previous installation has already been removed, setup will not restore it automatically.")
}

func (p *guidedPlatform) Execute(operation, target, logPath string) (uint32, error) {
	title := "Installing Agent Sessions"
	if operation == "/x" {
		title = "Removing the previous installation"
	}
	p.ui.Show(title, "Please wait while Windows Installer completes this step.\n\nSetup cannot be closed during this operation. Settings and session data are kept.")
	status, err := p.platform.Execute(operation, target, logPath)
	p.ui.Show("Verifying installation state", "Checking the result before continuing…")
	return status, err
}

func resultTitle(out outcome) string {
	if out.Status == 3010 || out.Status == 1641 {
		return "Windows restart required"
	}
	if out.Success {
		return "Setup completed"
	}
	if out.Status == 1602 {
		return "Setup cancelled"
	}
	return "Setup could not complete"
}

func finishText(out outcome) string {
	text := fmt.Sprintf("%s\n\nOperation: %s\nWindows Installer status: %d", out.Message, out.Operation, out.Status)
	if out.Payload.Directory != "" {
		text += "\n\nLogs and recovery MSI (retained):\n" + out.Payload.Directory
		if !out.Success {
			text += "\n\nIf the old app was removed, resolve the error (and restart Windows if requested), then rerun setup or manually install payload.msi from this directory. Settings and session data have not been deleted by setup."
		}
	}
	return text
}

func exitCode(out outcome) int {
	if out.Success || out.Status != 0 {
		return int(out.Status)
	}
	return 1
}

package app

import "github.com/ginkcode/agent-sessions/internal/update"

func init() {
	// apt, dnf and the AUR bring Linux updates; the toast says which to run.
	installMethod = func() string { return update.LinuxInstall("/") }
}

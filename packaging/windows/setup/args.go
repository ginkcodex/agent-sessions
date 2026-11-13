package main

import "github.com/ginkcode/agent-sessions/internal/winmsi"

func installerArgs(operation, target, logPath string) []string {
	return winmsi.InstallerArgs(operation, target, logPath)
}

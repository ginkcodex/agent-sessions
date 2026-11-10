//go:build !darwin && !linux && desktop

package main

import (
	"github.com/ginkcode/agent-sessions/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WebView2 uses the shared CSS page setup through its interactive print path.
func printWindow(window *application.WebviewWindow, _ app.PrintPageSetup) error {
	return window.Print()
}

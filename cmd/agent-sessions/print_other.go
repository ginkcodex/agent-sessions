//go:build !darwin && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// printWindow uses Wails' Print off macOS: the WebView2 and WebKitGTK print
// dialogs already offer the orientation.
func printWindow(window *application.WebviewWindow) error { return window.Print() }

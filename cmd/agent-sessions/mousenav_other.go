//go:build !linux && !darwin && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// hookMouseNavigation is native on Linux and macOS only; WebView2 reports
// the side buttons to the page as buttons 3 and 4.
func hookMouseNavigation() {}

func hookWindowMouseNavigation(*application.WebviewWindow) {}

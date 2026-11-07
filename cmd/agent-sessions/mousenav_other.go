//go:build !linux && !darwin && !windows && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// hookMouseNavigation is native on Linux, macOS and Windows only.
func hookMouseNavigation() {}

func hookWindowMouseNavigation(*application.WebviewWindow) {}

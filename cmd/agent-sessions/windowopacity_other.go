//go:build !darwin && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// configureWindowOpacity is a macOS-only native compositing experiment.
func configureWindowOpacity(*application.WebviewWindow) {}

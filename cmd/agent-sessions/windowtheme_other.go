//go:build !darwin && !linux && desktop

package main

import "github.com/wailsapp/wails/v3/pkg/application"

func setWindowTheme(_ *application.WebviewWindow, _ string) error { return nil }

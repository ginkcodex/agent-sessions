//go:build windows

package main

import (
	"github.com/ginkcode/agent-sessions/internal/winmsi"
	"golang.org/x/sys/windows"
)

func wide(s string) *uint16 { return windows.StringToUTF16Ptr(s) }

// The MSI reading is shared with the in-app updater.
func validateMSI(path, expectedVersion string) error { return winmsi.Validate(path, expectedVersion) }
func installedProducts() ([]product, error)          { return winmsi.InstalledProducts() }
func payloadProductCode(path string) (string, error) { return winmsi.ProductCode(path) }

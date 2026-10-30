//go:build !darwin && (webkit2_41 || desktop)

package main

// hideSiriAffordance is for macOS only; no other platform has the button.
func hideSiriAffordance() {}

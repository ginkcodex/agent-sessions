//go:build !linux && desktop

package main

// disableFontHinting is Linux-only; other platforms render text natively.
func disableFontHinting() {}

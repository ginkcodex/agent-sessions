//go:build darwin && (webkit2_41 || desktop)

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void hide_siri_affordance(void);
*/
import "C"

// hideSiriAffordance keeps macOS 27's Siri button off the app's web view
// (siri_darwin.m). Call it before the window is created.
func hideSiriAffordance() {
	C.hide_siri_affordance()
}

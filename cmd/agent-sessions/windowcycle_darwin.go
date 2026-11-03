//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void hook_window_cycling(void);
*/
import "C"

import "sync/atomic"

var cycleWindow atomic.Pointer[func(forward bool)]

// hookWindowCycling makes Cmd+` bring the app's next window to the front,
// and Cmd+Shift+` the previous one, as in any macOS app. The key is taken
// natively (windowcycle_darwin.m) and passed to cycle, which moves through
// the window ring: in the order the windows opened, and into another
// running copy of the app too.
func hookWindowCycling(cycle func(forward bool)) {
	cycleWindow.Store(&cycle)
	C.hook_window_cycling()
}

//export goCycleWindow
func goCycleWindow(forward C.int) {
	cycle := cycleWindow.Load()
	if cycle == nil {
		return
	}
	// Off the main thread: raising a window may talk to another copy of the
	// app, which may be slow to answer.
	go (*cycle)(forward != 0)
}

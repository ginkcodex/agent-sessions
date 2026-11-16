//go:build darwin && desktop

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa -framework Carbon
#include <stdlib.h>
void hook_keylog(void);
void keylog_set_enabled(int on);
char *keylog_snapshot(void);
*/
import "C"

import (
	"encoding/json"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/keylog"
)

var keylogRecorder atomic.Pointer[keylog.Recorder]

// hookKeyboardLog feeds rec with AppKit's key events and text-input state
// changes while rec is on (keylog_darwin.m).
func hookKeyboardLog(rec *keylog.Recorder) {
	keylogRecorder.Store(rec)
	rec.OnChange(func(on bool) { C.keylog_set_enabled(cBool(on)) })
	C.keylog_set_enabled(cBool(rec.Enabled()))
	C.hook_keylog()
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// goKeylogEvent takes one native record. It runs on the main thread, inside
// the key event, so it only copies the record into the buffer.
//
//export goKeylogEvent
func goKeylogEvent(mark C.int, data *C.char, n C.int) {
	rec := keylogRecorder.Load()
	if rec == nil || n <= 0 {
		return
	}
	source := keylog.Native
	if mark != 0 {
		source = keylog.Mark
	}
	rec.Add(keylog.Event{Time: time.Now(), Source: source, Data: C.GoBytes(unsafe.Pointer(data), n)})
}

// keyboardStateSnapshot describes AppKit's text-input state now, for the
// saved log's header.
func keyboardStateSnapshot() json.RawMessage {
	p := C.keylog_snapshot()
	if p == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(p))
	return json.RawMessage(C.GoString(p))
}

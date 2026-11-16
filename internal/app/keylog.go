package app

import (
	"encoding/json"
	"math"
	"time"

	"github.com/ginkcode/agent-sessions/internal/keylog"
)

// KeyboardLogStateEvent tells every window's page whether keyboard events are
// being recorded, as a bool, each time that changes.
const KeyboardLogStateEvent = "keyboard-log:state"

// Limits on what one page can hand the recorder in one call.
const (
	maxKeyboardBatch  = 1000
	maxKeyboardRecord = 4 << 10
)

// keyLogger is a window's way into the shared keyboard recorder.
type keyLogger struct {
	rec    *keylog.Recorder
	window string
}

// KeyboardLogEnabled reports whether keyboard events are being recorded, so
// a page that opens while recording is on joins in.
func (a *App) KeyboardLogEnabled() bool {
	return a.keylog.rec != nil && a.keylog.rec.Enabled()
}

// RecordKeyboardEvents adds the page's keyboard and input records to the
// recorder. Each is a JSON object whose "t" is its time in milliseconds since
// the Unix epoch. Calls while recording is off, malformed and oversized
// records are dropped; recording is a diagnostic and never fails the page.
func (a *App) RecordKeyboardEvents(events []json.RawMessage) {
	rec := a.keylog.rec
	if rec == nil || !rec.Enabled() {
		return
	}
	if len(events) > maxKeyboardBatch {
		events = events[len(events)-maxKeyboardBatch:]
	}
	for _, raw := range events {
		if len(raw) > maxKeyboardRecord {
			continue
		}
		var head struct {
			T float64 `json:"t"`
		}
		if json.Unmarshal(raw, &head) != nil || head.T <= 0 {
			continue
		}
		rec.Add(keylog.Event{
			Time:   pageTime(head.T),
			Source: keylog.Web,
			Window: a.keylog.window,
			Data:   raw,
		})
	}
}

// pageTime converts a page's epoch milliseconds, with a fraction, to a time.
// The whole milliseconds are kept apart so the fraction keeps its precision.
func pageTime(ms float64) time.Time {
	whole := math.Floor(ms)
	return time.UnixMilli(int64(whole)).Add(time.Duration((ms - whole) * float64(time.Millisecond)))
}

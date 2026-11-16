package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/keylog"
)

func TestKeyboardLogStateReachesEveryWindow(t *testing.T) {
	w := testWindows(t)
	var mu sync.Mutex
	got := map[string][]bool{}
	for range 2 {
		name := w.Reserve()
		w.Start(name, t.Context(), Desktop{Emit: func(event string, payload any) {
			if event != KeyboardLogStateEvent {
				return
			}
			mu.Lock()
			got[name] = append(got[name], payload.(bool))
			mu.Unlock()
		}})
	}
	w.KeyLog().SetEnabled(true)
	w.KeyLog().SetEnabled(false)

	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"window-1", "window-2"} {
		if s := got[name]; len(s) != 2 || !s[0] || s[1] {
			t.Fatalf("%s got %v, want [true false]", name, s)
		}
	}
}

func TestRecordKeyboardEvents(t *testing.T) {
	w := testWindows(t)
	r := testRouter(w)
	name := w.Reserve()
	w.Start(name, t.Context(), Desktop{})
	ctx := fromWindow(name)

	at := time.Date(2026, 11, 16, 9, 30, 0, 250_000_000, time.UTC)
	ms := float64(at.UnixNano()) / float64(time.Millisecond)
	good := map[string]any{"t": ms, "type": "keydown", "key": "â"}
	batch := []any{
		good,
		map[string]any{"type": "keydown"}, // no time
		"not an object",                   // malformed
		map[string]any{"t": ms, "pad": strings.Repeat("x", maxKeyboardRecord)}, // too big
	}

	// Off: dropped.
	if _, err := r.Call(ctx, "RecordKeyboardEvents", rawArgs(t, batch)); err != nil {
		t.Fatal(err)
	}
	if on, _ := r.Call(ctx, "KeyboardLogEnabled", nil); on != false {
		t.Fatalf("KeyboardLogEnabled = %v while off", on)
	}
	if events, _, _ := w.KeyLog().Snapshot(); len(events) != 0 {
		t.Fatalf("recorded %d events while off", len(events))
	}

	w.KeyLog().SetEnabled(true)
	if on, _ := r.Call(ctx, "KeyboardLogEnabled", nil); on != true {
		t.Fatalf("KeyboardLogEnabled = %v while on", on)
	}
	if _, err := r.Call(ctx, "RecordKeyboardEvents", rawArgs(t, batch)); err != nil {
		t.Fatal(err)
	}
	events, _, _ := w.KeyLog().Snapshot()
	if len(events) != 1 {
		t.Fatalf("recorded %d events, want only the well-formed one", len(events))
	}
	e := events[0]
	if e.Source != "web" || e.Window != name || e.Time.Sub(at).Abs() > time.Microsecond {
		t.Fatalf("event = %+v, want web from %s at %v", e, name, at)
	}
	var data map[string]any
	if err := json.Unmarshal(e.Data, &data); err != nil || data["key"] != "â" {
		t.Fatalf("data = %s", e.Data)
	}
}

func TestRecordKeyboardEventsKeepsTheNewestOfABigBatch(t *testing.T) {
	rec := keylog.New(keylog.DefaultCapacity)
	a := &App{keylog: keyLogger{rec: rec, window: "window-1"}}
	rec.SetEnabled(true)

	batch := make([]json.RawMessage, maxKeyboardBatch+5)
	for i := range batch {
		batch[i] = json.RawMessage(fmt.Sprintf(`{"t":%d}`, i+1))
	}
	a.RecordKeyboardEvents(batch)
	events, _, _ := rec.Snapshot()
	if len(events) != maxKeyboardBatch {
		t.Fatalf("recorded %d, want %d", len(events), maxKeyboardBatch)
	}
	if string(events[0].Data) != string(batch[5]) {
		t.Fatalf("oldest kept = %s, want %s", events[0].Data, batch[5])
	}
}

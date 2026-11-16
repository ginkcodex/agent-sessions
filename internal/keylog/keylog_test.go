package keylog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func ev(sec int, data string) Event {
	return Event{Time: time.Unix(int64(sec), 0), Source: Native, Data: json.RawMessage(data)}
}

func datas(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = string(e.Data)
	}
	return out
}

func TestRecorderDropsEventsWhileOff(t *testing.T) {
	r := New(4)
	r.Add(ev(1, `1`))
	if events, _, _ := r.Snapshot(); len(events) != 0 {
		t.Fatalf("recorded while off: %v", datas(events))
	}
}

func TestRecorderKeepsNewestAndCountsDrops(t *testing.T) {
	r := New(3)
	r.SetEnabled(true)
	for i := 1; i <= 5; i++ {
		r.Add(ev(i, string(rune('0'+i))))
	}
	events, since, dropped := r.Snapshot()
	if got := datas(events); len(got) != 3 || got[0] != "3" || got[2] != "5" {
		t.Fatalf("events = %v, want 3 4 5", got)
	}
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2", dropped)
	}
	if since.IsZero() {
		t.Fatal("since not set")
	}
}

func TestRecorderOrdersByTimeStably(t *testing.T) {
	r := New(10)
	r.SetEnabled(true)
	r.Add(ev(5, `"late native"`))
	r.Add(ev(3, `"web a"`))
	r.Add(ev(3, `"web b"`))
	r.Add(ev(4, `"web c"`))
	got := datas(func() []Event { e, _, _ := r.Snapshot(); return e }())
	want := []string{`"web a"`, `"web b"`, `"web c"`, `"late native"`}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestRecorderOnClearsOffKeeps(t *testing.T) {
	r := New(4)
	var states []bool
	r.OnChange(func(on bool) { states = append(states, on) })
	r.SetEnabled(true)
	r.Add(ev(1, `1`))
	r.SetEnabled(false)
	r.SetEnabled(false) // no change, no call
	if events, _, _ := r.Snapshot(); len(events) != 1 {
		t.Fatalf("turning off lost the events: %v", datas(events))
	}
	r.SetEnabled(true)
	if events, _, _ := r.Snapshot(); len(events) != 0 {
		t.Fatalf("turning on kept old events: %v", datas(events))
	}
	if len(states) != 3 || !states[0] || states[1] || !states[2] {
		t.Fatalf("watch calls = %v, want [true false true]", states)
	}
}

func TestWriteToHeaderThenEvents(t *testing.T) {
	r := New(4)
	r.SetEnabled(true)
	r.Add(Event{Time: time.Unix(2, 0), Source: Web, Window: "window-1", Data: json.RawMessage(`{"type":"keydown","key":"<â>"}`)})
	r.Add(ev(1, `{"type":"keyDown"}`))
	var out bytes.Buffer
	if err := r.WriteTo(&out, Header{App: "1.0.0", OS: "darwin", State: json.RawMessage(`{"secure":false}`)}); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(&out)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3:\n%s", len(lines), out.String())
	}
	var h Header
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatal(err)
	}
	if h.Kind != "header" || h.Events != 2 || h.App != "1.0.0" || h.Saved.IsZero() || string(h.State) != `{"secure":false}` {
		t.Fatalf("header = %+v", h)
	}
	var first, second Event
	_ = json.Unmarshal([]byte(lines[1]), &first)
	_ = json.Unmarshal([]byte(lines[2]), &second)
	if first.Source != Native || second.Window != "window-1" {
		t.Fatalf("events out of order: %s / %s", lines[1], lines[2])
	}
	// Typed text stays readable: no < escapes.
	if !bytes.Contains([]byte(lines[2]), []byte(`"key":"<â>"`)) {
		t.Fatalf("event data was re-escaped: %s", lines[2])
	}
}

func TestSaveWritesAPrivateNewFile(t *testing.T) {
	r := New(4)
	r.SetEnabled(true)
	r.Add(ev(1, `{"type":"keyDown"}`))
	dir := filepath.Join(t.TempDir(), "logs")
	saved := time.Date(2026, 11, 16, 9, 30, 0, 0, time.Local)
	first, err := r.Save(dir, Header{Saved: saved})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Save(dir, Header{Saved: saved})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(first) != "keyboard-20261116-093000.jsonl" || filepath.Base(second) != "keyboard-20261116-093000-1.jsonl" {
		t.Fatalf("paths = %s, %s", first, second)
	}
	data, err := os.ReadFile(first)
	if err != nil || !bytes.Contains(data, []byte(`"kind":"header"`)) || !bytes.Contains(data, []byte(`"keyDown"`)) {
		t.Fatalf("log = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(first); info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
		}
	}
}

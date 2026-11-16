// Package keylog is an opt-in flight recorder for keyboard events. It keeps
// the most recent native and page events of the desktop app in memory, so a
// rare input fault can be saved and studied after it shows up (see
// docs/plan/keyboard-diagnostics.md). Nothing reaches disk until WriteTo.
package keylog

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

// Sources of events.
const (
	Native = "native" // the toolkit's own event stream (AppKit)
	Web    = "web"    // DOM events in a window's page
	Mark   = "mark"   // state changes: activation, focus, input source
)

// DefaultCapacity is how many events the app's recorder keeps.
const DefaultCapacity = 20000

// Event is one recorded event. Data is the source's own JSON record.
type Event struct {
	Time   time.Time       `json:"t"`
	Source string          `json:"src"`
	Window string          `json:"win,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Header is the first line of a saved log.
type Header struct {
	Kind    string          `json:"kind"` // always "header"
	App     string          `json:"app"`
	OS      string          `json:"os"`
	Arch    string          `json:"arch"`
	Since   time.Time       `json:"since"` // when recording last started
	Saved   time.Time       `json:"saved"`
	Events  int             `json:"events"`
	Dropped uint64          `json:"dropped"` // older events the buffer let go
	State   json.RawMessage `json:"state,omitempty"`
}

// Recorder is a ring buffer of the newest events, off until enabled. It is
// safe for concurrent use.
type Recorder struct {
	mu      sync.Mutex
	on      bool
	since   time.Time
	buf     []Event
	next    int // where the next event goes once buf is full
	dropped uint64
	watch   []func(on bool)
}

// New creates a recorder that keeps up to capacity events.
func New(capacity int) *Recorder {
	if capacity < 1 {
		capacity = 1
	}
	return &Recorder{buf: make([]Event, 0, capacity)}
}

// Enabled reports whether events are being recorded.
func (r *Recorder) Enabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.on
}

// SetEnabled turns recording on or off and tells the watchers when that
// changes. Turning it on starts a new log; turning it off keeps the events,
// so they can still be saved.
func (r *Recorder) SetEnabled(on bool) {
	r.mu.Lock()
	if r.on == on {
		r.mu.Unlock()
		return
	}
	r.on = on
	if on {
		r.buf = r.buf[:0]
		r.next = 0
		r.dropped = 0
		r.since = time.Now()
	}
	watch := append([]func(bool){}, r.watch...)
	r.mu.Unlock()
	for _, fn := range watch {
		fn(on)
	}
}

// OnChange calls fn with the new state each time recording turns on or off.
func (r *Recorder) OnChange(fn func(on bool)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.watch = append(r.watch, fn)
}

// Add records an event, or drops it while recording is off. A full buffer
// lets its oldest event go.
func (r *Recorder) Add(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.on {
		return
	}
	if len(r.buf) < cap(r.buf) {
		r.buf = append(r.buf, e)
		return
	}
	r.buf[r.next] = e
	r.next = (r.next + 1) % len(r.buf)
	r.dropped++
}

// Snapshot returns the recorded events ordered by time; events with the same
// time keep the order they arrived in. It also returns when recording
// started and how many events were dropped.
func (r *Recorder) Snapshot() (events []Event, since time.Time, dropped uint64) {
	r.mu.Lock()
	events = make([]Event, 0, len(r.buf))
	events = append(events, r.buf[r.next:]...)
	events = append(events, r.buf[:r.next]...)
	since, dropped = r.since, r.dropped
	r.mu.Unlock()
	// Page events arrive in batches, after native events that happened later.
	sort.SliceStable(events, func(i, j int) bool { return events[i].Time.Before(events[j].Time) })
	return events, since, dropped
}

// WriteTo writes the log as JSON Lines: h, filled with the counts, then the
// events in time order.
func (r *Recorder) WriteTo(w io.Writer, h Header) error {
	events, since, dropped := r.Snapshot()
	h.Kind = "header"
	h.Since = since
	h.Events = len(events)
	h.Dropped = dropped
	if h.Saved.IsZero() {
		h.Saved = time.Now()
	}
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(h); err != nil {
		return err
	}
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// DefaultDir is where saved logs go: ~/Library/Logs/agent-sessions on macOS,
// the user cache directory's agent-sessions/logs elsewhere.
func DefaultDir() (string, error) {
	if runtime.GOOS == "darwin" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Logs", "agent-sessions"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "agent-sessions", "logs"), nil
}

// Save writes the log to a new file in dir, named after h.Saved, readable by
// the user only, and returns its path.
func (r *Recorder) Save(dir string, h Header) (path string, err error) {
	if h.Saved.IsZero() {
		h.Saved = time.Now()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	base := "keyboard-" + h.Saved.Format("20060102-150405")
	var f *os.File
	for i := 0; ; i++ {
		path = filepath.Join(dir, base+".jsonl")
		if i > 0 {
			path = filepath.Join(dir, fmt.Sprintf("%s-%d.jsonl", base, i))
		}
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil || !errors.Is(err, fs.ErrExist) || i >= 99 {
			break
		}
	}
	if err != nil {
		return "", err
	}
	if err := r.WriteTo(f, h); err != nil {
		_ = f.Close()
		return "", err
	}
	return path, f.Close()
}

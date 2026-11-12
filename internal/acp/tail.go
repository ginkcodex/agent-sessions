package acp

import (
	"strings"
	"sync"
)

// tailBuffer keeps the last max bytes written to it: the end of an agent's
// stderr, for error reports.
type tailBuffer struct {
	mu      sync.Mutex
	buf     []byte
	max     int
	dropped bool
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
		t.dropped = true
	}
	return len(p), nil
}

// String returns the kept bytes. When older output was dropped, it starts
// at the first whole line.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := string(t.buf)
	if t.dropped {
		if i := strings.IndexByte(s, '\n'); i >= 0 && i < len(s)-1 {
			s = s[i+1:]
		}
	}
	return s
}

// lastLine is the last non-empty line of s.
func lastLine(s string) string {
	s = strings.TrimRight(s, "\r\n\t ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}

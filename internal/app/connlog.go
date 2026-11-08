package app

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// phaseLogger writes one line per connection phase change with the time
// spent in the previous phase, so a recovery can be timed from the log
// without a debugger:
//
//	agent-sessions: connection host: reconnecting after 21.3s connected (connection lost)
//
// Only the phase, host and error text are written; the error text is what
// the frontend already shows.
type phaseLogger struct {
	logf func(format string, args ...any)
	now  func() time.Time

	phase string
	host  string
	since time.Time
}

func newPhaseLogger() *phaseLogger {
	return &phaseLogger{logf: log.Printf, now: time.Now, phase: ConnLocal}
}

// maxLoggedError bounds the error text; remote stderr can be long.
const maxLoggedError = 300

// observe records st and logs it when the phase or host changed. Callers
// serialize it (connection.mu).
func (p *phaseLogger) observe(st ConnectionState) {
	if p == nil || (st.Phase == p.phase && st.Host == p.host) {
		return
	}
	now := p.now()
	host := st.Host
	if host == "" {
		host = p.host
	}
	if host != "" {
		var b strings.Builder
		fmt.Fprintf(&b, "agent-sessions: connection %s: %s", host, st.Phase)
		if !p.since.IsZero() {
			fmt.Fprintf(&b, " after %s %s", roundElapsed(now.Sub(p.since)), p.phase)
			if p.host != "" && p.host != host {
				fmt.Fprintf(&b, " %s", p.host)
			}
		}
		if msg := oneLine(st.Error, maxLoggedError); msg != "" {
			fmt.Fprintf(&b, " (%s)", msg)
		}
		p.logf("%s", b.String())
	}
	p.phase, p.host, p.since = st.Phase, st.Host, now
}

func roundElapsed(d time.Duration) time.Duration {
	if d < time.Second {
		return d.Round(time.Millisecond)
	}
	return d.Round(100 * time.Millisecond)
}

// oneLine joins s onto one line and cuts it to at most max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/remote"
)

type fakeLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *fakeLog) logf(format string, args ...any) {
	l.mu.Lock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
	l.mu.Unlock()
}

func (l *fakeLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func TestPhaseLogger_TimesEachPhase(t *testing.T) {
	out := &fakeLog{}
	clock := time.Unix(1000, 0)
	p := newPhaseLogger()
	p.logf = out.logf
	p.now = func() time.Time { return clock }
	step := func(d time.Duration, st ConnectionState) {
		clock = clock.Add(d)
		p.observe(st)
	}

	step(0, ConnectionState{Phase: ConnLocal}) // unchanged: nothing
	step(time.Second, ConnectionState{Phase: ConnConnecting, Host: "box", Generation: 1})
	step(2345*time.Millisecond, ConnectionState{Phase: ConnConnected, Host: "box", Generation: 1})
	step(time.Minute, ConnectionState{Phase: ConnConnected, Host: "box", Generation: 1}) // same phase: nothing
	step(21300*time.Millisecond, ConnectionState{Phase: ConnDisconnected, Host: "box", Generation: 1, Error: "connection lost"})
	step(3*time.Millisecond, ConnectionState{Phase: ConnReconnecting, Host: "box", Generation: 1, Error: "remote server handshake: boom\n(stderr: line one\r\nline two)"})
	step(4*time.Second, ConnectionState{Phase: ConnLocal, Generation: 2})
	step(time.Second, ConnectionState{Phase: ConnConnecting, Host: "a", Generation: 3})
	step(time.Second, ConnectionState{Phase: ConnConnecting, Host: "b", Generation: 4})

	want := []string{
		"agent-sessions: connection box: connecting",
		"agent-sessions: connection box: connected after 2.3s connecting",
		"agent-sessions: connection box: disconnected after 1m21.3s connected (connection lost)",
		"agent-sessions: connection box: reconnecting after 3ms disconnected (remote server handshake: boom (stderr: line one line two))",
		"agent-sessions: connection box: local after 4s reconnecting",
		"agent-sessions: connection a: connecting after 1s local",
		"agent-sessions: connection b: connecting after 1s connecting a",
	}
	if got := out.snapshot(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestPhaseLogger_FirstChangeHasNoElapsed(t *testing.T) {
	out := &fakeLog{}
	p := &phaseLogger{logf: out.logf, now: time.Now, phase: ConnLocal}
	p.observe(ConnectionState{Phase: ConnConnecting, Host: "box"})
	if got := out.snapshot(); len(got) != 1 || got[0] != "agent-sessions: connection box: connecting" {
		t.Fatalf("log = %q", got)
	}
}

func TestPhaseLogger_CutsLongErrors(t *testing.T) {
	out := &fakeLog{}
	p := newPhaseLogger()
	p.logf = out.logf
	p.observe(ConnectionState{Phase: ConnDisconnected, Host: "box", Error: strings.Repeat("é", maxLoggedError+50)})
	got := out.snapshot()
	if len(got) != 1 || !strings.HasSuffix(got[0], strings.Repeat("é", maxLoggedError)+"…)") {
		t.Fatalf("log = %q", got)
	}
}

func TestPhaseLogger_NilIsSilent(t *testing.T) {
	var p *phaseLogger
	p.observe(ConnectionState{Phase: ConnConnecting, Host: "box"})
}

// The connection logs through emit, so every published phase is timed,
// and the per-host environment never reaches the log.
func TestConnection_LogsPhaseChanges(t *testing.T) {
	out := &fakeLog{}
	c := newConnection(nil)
	c.phases.logf = out.logf
	c.setEnv(map[string]string{"CLAUDE_CONFIG_DIR": "/secret/place"})
	useReconnectBackoff(t, time.Hour)
	c.dial = func(ctx context.Context, alias string, opts remote.SSHOptions, env map[string]string, emitter engine.Emitter) (*remote.Session, error) {
		return nil, errors.New("ssh: connect to host box port 22: Connection refused")
	}
	if err := c.Connect(context.Background(), "box"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for c.snapshot().Phase != ConnReconnecting {
		if time.Now().After(deadline) {
			t.Fatalf("phase = %q", c.snapshot().Phase)
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.Disconnect()

	got := out.snapshot()
	phases := make([]string, 0, len(got))
	for _, line := range got {
		if !strings.HasPrefix(line, "agent-sessions: connection box: ") {
			t.Fatalf("line %q", line)
		}
		if strings.Contains(line, "/secret/place") {
			t.Fatalf("environment value logged: %q", line)
		}
		phases = append(phases, strings.Fields(strings.TrimPrefix(line, "agent-sessions: connection box: "))[0])
	}
	want := []string{ConnConnecting, ConnDisconnected, ConnReconnecting, ConnLocal}
	if strings.Join(phases, ",") != strings.Join(want, ",") {
		t.Fatalf("phases = %v, want %v\n%s", phases, want, strings.Join(got, "\n"))
	}
	if !strings.Contains(got[1], "(ssh: connect to host box port 22: Connection refused)") {
		t.Fatalf("disconnected line = %q", got[1])
	}
}

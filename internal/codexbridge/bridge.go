// Package codexbridge lets a Codex chat share Codex's background daemon.
//
// codex-acp, the Codex chat adapter, starts "$CODEX_PATH app-server" and
// talks JSON-RPC to it, one message per line. That private app-server cannot
// open a session the daemon holds: Codex lets one process write a session,
// and its daemon keeps a session for a minute after the Codex CLI quits.
//
// The app sets CODEX_PATH to its own executable and EnvCodex to the real
// codex. Run that way, Main stands in for "codex app-server": when a daemon
// runs, it carries the lines to the daemon's control socket, a WebSocket on
// a Unix socket, as the Codex CLI does. Anything else runs the real codex.
// See docs/plan/codex-daemon-chat.md.
package codexbridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/coder/websocket"
)

const (
	// EnvCodex names the real codex. When set, the executable is the bridge.
	EnvCodex = "AGENT_SESSIONS_CODEX"
	// EnvDaemon set to "off" runs the real codex for every command.
	EnvDaemon = "AGENT_SESSIONS_CODEX_DAEMON"
)

// How long to wait for the daemon to say where it is, and to answer the
// WebSocket handshake. Both take milliseconds when it runs.
var (
	versionTimeout = 5 * time.Second
	dialTimeout    = 5 * time.Second
)

// Main runs the bridge when EnvCodex is set, and returns the exit code and
// true. Without it, it returns false and the caller goes on as itself.
// args are the command line without the program name.
func Main(args []string) (int, bool) {
	real := os.Getenv(EnvCodex)
	if real == "" {
		return 0, false
	}
	// Neither the real codex nor anything it starts is a bridge.
	_ = os.Unsetenv(EnvCodex)
	if conn := daemonFor(args, real, os.Stderr); conn != nil {
		return Copy(conn, os.Stdin, os.Stdout), true
	}
	return runReal(real, args), true
}

// daemonFor connects to the daemon when args are a plain "app-server", the
// bridge is on and a daemon runs; else nil. A daemon that says it runs but
// cannot be reached is reported on stderr.
func daemonFor(args []string, real string, stderr io.Writer) *websocket.Conn {
	if len(args) != 1 || args[0] != "app-server" || os.Getenv(EnvDaemon) == "off" {
		return nil
	}
	sock, err := DaemonSocket(real)
	if err != nil {
		return nil
	}
	conn, err := Dial(sock)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "agent-sessions: cannot reach the Codex daemon, starting codex app-server: %v\n", err)
		return nil
	}
	return conn
}

// DaemonSocket asks codex where its running daemon listens. It fails when
// no daemon runs.
func DaemonSocket(codex string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, codex, "app-server", "daemon", "version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("codex app-server daemon version: %w", err)
	}
	var v struct {
		Status     string `json:"status"`
		SocketPath string `json:"socketPath"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("codex app-server daemon version: %w", err)
	}
	if v.Status != "running" || v.SocketPath == "" {
		return "", fmt.Errorf("codex daemon is %q at %q", v.Status, v.SocketPath)
	}
	return v.SocketPath, nil
}

// Dial opens a WebSocket on the daemon's Unix socket.
func Dial(sock string) (*websocket.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	// The library owns the handshake's response body.
	conn, _, err := websocket.Dial(ctx, "ws://localhost/", &websocket.DialOptions{HTTPClient: hc}) //nolint:bodyclose
	if err != nil {
		return nil, err
	}
	// A thread's history can be many megabytes in one message.
	conn.SetReadLimit(-1)
	return conn, nil
}

// Copy carries lines from in to conn as text messages, and messages from
// conn to out as lines, until in ends or conn closes. It returns 0 when
// either side closed normally.
func Copy(conn *websocket.Conn, in io.Reader, out io.Writer) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sent := make(chan error, 1)
	go func() { sent <- send(ctx, conn, in) }()
	var recvErr error
	received := make(chan struct{})
	go func() {
		recvErr = receive(ctx, conn, out)
		close(received)
	}()

	select {
	case err := <-sent:
		if err != nil {
			_ = conn.Close(websocket.StatusInternalError, "")
			return 1
		}
		// The client is done and wants no more replies: say goodbye. Close
		// waits for the daemon's reply and drops messages that cross it;
		// receive then ends, after any line it was writing.
		_ = conn.Close(websocket.StatusNormalClosure, "")
		<-received
		return 0
	case <-received:
		_ = conn.CloseNow()
		switch websocket.CloseStatus(recvErr) {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway:
			return 0
		}
		return 1
	}
}

func send(ctx context.Context, conn *websocket.Conn, in io.Reader) error {
	r := bufio.NewReaderSize(in, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 {
			if werr := conn.Write(ctx, websocket.MessageText, line); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func receive(ctx context.Context, conn *websocket.Conn, out io.Writer) error {
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		// One message per line: JSON the daemon sends is compact, but make
		// sure a pretty-printed one cannot split.
		if bytes.IndexByte(msg, '\n') >= 0 {
			var buf bytes.Buffer
			if json.Compact(&buf, msg) == nil {
				msg = buf.Bytes()
			}
		}
		if _, err := out.Write(append(msg, '\n')); err != nil {
			return err
		}
	}
}

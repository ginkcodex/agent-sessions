//go:build unix

package codexbridge

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const envTestMain = "CODEXBRIDGE_TEST_MAIN"

// The test binary stands in for the app: run with envTestMain, it is Main.
func TestMain(m *testing.M) {
	if os.Getenv(envTestMain) != "" {
		code, ok := Main(os.Args[1:])
		if !ok {
			code = 99
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// fakeDaemon serves a WebSocket on a Unix socket that echoes each message,
// and closes normally after a message "bye".
func fakeDaemon(t *testing.T) string {
	t.Helper()
	// Short: a Unix socket path has a small limit.
	dir, err := os.MkdirTemp("", "cb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.SetReadLimit(-1)
		defer func() { _ = c.CloseNow() }()
		for {
			typ, msg, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if string(msg) == "bye" {
				_ = c.Close(websocket.StatusNormalClosure, "")
				return
			}
			if string(msg) == "pretty" {
				msg = []byte("{\n  \"a\": 1\n}")
			}
			if err := c.Write(r.Context(), typ, msg); err != nil {
				return
			}
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return sock
}

// fakeCodex writes a codex whose "app-server daemon version" prints version
// (or fails when it is ""), and which otherwise says how it was run and
// exits 7.
func fakeCodex(t *testing.T, version string) string {
	t.Helper()
	answer := "echo 'failed to connect' >&2; exit 1"
	if version != "" {
		answer = "printf '%s\\n' '" + version + "'; exit 0"
	}
	path := filepath.Join(t.TempDir(), "codex")
	script := "#!/bin/sh\n" +
		"if [ \"$*\" = 'app-server daemon version' ]; then " + answer + "; fi\n" +
		"echo \"real codex: $* bridge=${" + EnvCodex + "-unset}\"\nexit 7\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func running(sock string) string {
	return `{"status":"running","backend":"pid","socketPath":"` + sock + `","cliVersion":"0.167.1"}`
}

func TestMainOffWithoutEnv(t *testing.T) {
	t.Setenv(EnvCodex, "")
	if _, ok := Main([]string{"app-server"}); ok {
		t.Fatal("ran without " + EnvCodex)
	}
}

func TestDaemonSocket(t *testing.T) {
	if got, err := DaemonSocket(fakeCodex(t, running("/tmp/d.sock"))); err != nil || got != "/tmp/d.sock" {
		t.Fatalf("running = %q, %v", got, err)
	}
	for name, version := range map[string]string{
		"none":    "",
		"stopped": `{"status":"stopped"}`,
		"no path": `{"status":"running"}`,
		"garbage": `codex 0.167.1`,
	} {
		if got, err := DaemonSocket(fakeCodex(t, version)); err == nil {
			t.Errorf("%s: socket %q", name, got)
		}
	}
	if _, err := DaemonSocket(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("no codex: no error")
	}
}

// pipes carries test lines to Copy and its lines back.
type pipes struct {
	in   *io.PipeWriter
	out  *bufio.Reader
	done chan int
}

func startCopy(t *testing.T, sock string) *pipes {
	t.Helper()
	conn, err := Dial(sock)
	if err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	p := &pipes{in: inW, out: bufio.NewReader(outR), done: make(chan int, 1)}
	go func() {
		p.done <- Copy(conn, inR, outW)
		_ = outW.Close()
	}()
	t.Cleanup(func() { _ = inW.Close() })
	return p
}

func (p *pipes) roundTrip(t *testing.T, line string) string {
	t.Helper()
	if _, err := io.WriteString(p.in, line+"\n"); err != nil {
		t.Fatal(err)
	}
	got, err := p.out.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(got, "\n")
}

func (p *pipes) exit(t *testing.T) int {
	t.Helper()
	select {
	case code := <-p.done:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("Copy did not return")
		return -1
	}
}

func TestCopy(t *testing.T) {
	p := startCopy(t, fakeDaemon(t))
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"id":2,"result":"` + strings.Repeat("x", 6<<20) + `"}`, // a long history
	} {
		if got := p.roundTrip(t, line); got != line {
			t.Fatalf("echo of %d bytes = %d bytes", len(line), len(got))
		}
	}
	if got := p.roundTrip(t, `{"id":3}`+"\r"); got != `{"id":3}` {
		t.Fatalf("CRLF line = %q", got)
	}
	if got := p.roundTrip(t, "pretty"); got != `{"a":1}` {
		t.Fatalf("multi-line message = %q", got)
	}
	// The adapter closes its end: the bridge says goodbye and exits.
	_ = p.in.Close()
	if code := p.exit(t); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if _, err := p.out.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("after exit: %v", err)
	}
}

func TestCopyDaemonCloses(t *testing.T) {
	p := startCopy(t, fakeDaemon(t))
	if _, err := io.WriteString(p.in, "bye\n"); err != nil {
		t.Fatal(err)
	}
	// stdin stays open; the daemon's close ends the bridge.
	if code := p.exit(t); code != 0 {
		t.Fatalf("exit = %d", code)
	}
}

func TestDialDeadSocket(t *testing.T) {
	if _, err := Dial(filepath.Join(t.TempDir(), "gone.sock")); err == nil {
		t.Fatal("dialled a missing socket")
	}
}

// runMain runs the test binary as the bridge with args, and returns its
// exit code, stdout and stderr.
func runMain(t *testing.T, codex, stdin string, args []string, env ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Env = append(os.Environ(), append([]string{envTestMain + "=1", EnvCodex + "=" + codex}, env...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return cmd.ProcessState.ExitCode(), out.String(), errOut.String()
}

func TestMainBridgesPlainAppServer(t *testing.T) {
	codex := fakeCodex(t, running(fakeDaemon(t)))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "app-server")
	cmd.Env = append(os.Environ(), envTestMain+"=1", EnvCodex+"="+codex)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	out := bufio.NewReader(outR)
	for _, line := range []string{`{"id":1}`, `{"id":2}`} {
		if _, err := io.WriteString(in, line+"\n"); err != nil {
			t.Fatal(err)
		}
		if got, err := out.ReadString('\n'); err != nil || got != line+"\n" {
			t.Fatalf("reply %q, %v; stderr %q", got, err, errOut.String())
		}
	}
	_ = in.Close()
	if rest, _ := io.ReadAll(out); len(rest) != 0 {
		t.Fatalf("more output %q", rest)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit: %v; stderr %q", err, errOut.String())
	}
}

func TestMainRunsRealCodex(t *testing.T) {
	sock := fakeDaemon(t)
	for _, tc := range []struct {
		name    string
		version string
		args    []string
		env     []string
		stderr  string
	}{
		{name: "overrides", version: running(sock), args: []string{"app-server", "-c", "model_provider=x"}},
		{name: "login", version: running(sock), args: []string{"login"}},
		{name: "no daemon", args: []string{"app-server"}},
		{name: "turned off", version: running(sock), args: []string{"app-server"}, env: []string{EnvDaemon + "=off"}},
		{name: "dead socket", version: running(sock + ".gone"), args: []string{"app-server"}, stderr: "cannot reach the Codex daemon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := runMain(t, fakeCodex(t, tc.version), "", tc.args, tc.env...)
			want := "real codex: " + strings.Join(tc.args, " ") + " bridge=unset\n"
			if code != 7 || out != want {
				t.Fatalf("exit %d, out %q, want %q; stderr %q", code, out, want, errOut)
			}
			if !strings.Contains(errOut, tc.stderr) {
				t.Fatalf("stderr %q, want %q", errOut, tc.stderr)
			}
		})
	}
}

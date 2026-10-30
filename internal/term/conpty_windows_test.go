//go:build windows

package term

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestConsoleRunsProcess(t *testing.T) {
	path, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	env := append(os.Environ(), "AS_TERM_TEST=hello")
	c, err := startConsole(path, []string{"cmd.exe", "/d", "/c", "echo %AS_TERM_TEST%& cd& exit 3"}, dir, env, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var out strings.Builder
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		b := make([]byte, 4096)
		for {
			n, err := c.Read(b)
			mu.Lock()
			out.Write(b[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	if err := c.Resize(120, 40); err != nil {
		t.Errorf("Resize: %v", err)
	}
	state, err := c.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if code := state.ExitCode(); code != 3 {
		t.Errorf("exit code %d, want 3", code)
	}
	// The console renders on its own schedule: closing it right away can
	// lose the last output.
	has := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(out.String(), "hello") && strings.Contains(out.String(), dir)
	}
	for deadline := time.Now().Add(5 * time.Second); !has() && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	<-readerDone
	if !has() {
		t.Errorf("output %q lacks %q or %q", out.String(), "hello", dir)
	}
	if err := c.Resize(80, 24); !errors.Is(err, errConsoleClosed) {
		t.Errorf("Resize after Close: %v, want errConsoleClosed", err)
	}
}

// TestBundledConptyPassesColorQueries needs conpty\ next to the test
// binary: copy it there from `make conpty`.
func TestBundledConptyPassesColorQueries(t *testing.T) {
	if os.Getenv("AS_TERM_COLOR_QUERY") == "1" {
		colorQueryChild()
		return
	}
	if bundledConpty() == &systemConpty {
		t.Skip("no conpty next to the test binary")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	argv := []string{exe, "-test.run=^TestBundledConptyPassesColorQueries$"}
	c, err := startConsole(exe, argv, "", append(os.Environ(), "AS_TERM_COLOR_QUERY=1"), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		b := make([]byte, 4096)
		replied := false
		for {
			n, err := c.Read(b)
			out.Write(b[:n])
			if !replied && strings.Contains(out.String(), "\x1b]11;?") {
				replied = true
				_, _ = c.Write([]byte("\x1b]11;rgb:ffff/ffff/ffff\x1b\\"))
			}
			if err != nil {
				return
			}
		}
	}()
	if _, err := c.Wait(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	_ = c.Close()
	<-readerDone
	if !strings.Contains(out.String(), "GOT rgb:ffff/ffff/ffff") {
		t.Errorf("the query or its reply did not pass through: %q", out.String())
	}
}

// colorQueryChild asks for the background color and prints the reply.
func colorQueryChild() {
	in := windows.Handle(os.Stdin.Fd())
	_ = windows.SetConsoleMode(in, windows.ENABLE_VIRTUAL_TERMINAL_INPUT)
	_, _ = os.Stdout.WriteString("\x1b]11;?\x07")
	got := make(chan string, 1)
	go func() {
		var all []byte
		b := make([]byte, 256)
		for {
			n, err := os.Stdin.Read(b)
			all = append(all, b[:n]...)
			if _, reply, ok := strings.Cut(string(all), "\x1b]11;"); ok {
				if color, _, ok := strings.Cut(reply, "\x1b\\"); ok {
					got <- color
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	select {
	case color := <-got:
		_, _ = os.Stdout.WriteString("GOT " + color + "\r\n")
	case <-time.After(3 * time.Second):
		_, _ = os.Stdout.WriteString("NO REPLY\r\n")
	}
}

func TestEnvBlock(t *testing.T) {
	block, err := envBlock([]string{"=C:=C:\\dir", "Path=a", "FOO=1", "PATH=b", "SystemRoot=C:\\Windows", "BAD"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for p := unsafe.Pointer(block); ; {
		s := windows.UTF16PtrToString((*uint16)(p))
		if s == "" {
			break
		}
		got = append(got, s)
		p = unsafe.Add(p, 2*(len(utf16.Encode([]rune(s)))+1))
	}
	want := []string{"=C:=C:\\dir", "PATH=b", "FOO=1", "SystemRoot=C:\\Windows"}
	if !slices.Equal(got, want) {
		t.Errorf("envBlock = %q, want %q", got, want)
	}
	if _, err := envBlock([]string{"A=1\x00B=2"}); err == nil {
		t.Error("envBlock accepted a NUL")
	}
}

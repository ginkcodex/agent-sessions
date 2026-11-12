//go:build !windows

package loginenv

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeShell writes an executable /bin/sh script and returns its path.
func fakeShell(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-shell")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// The shell runs as an interactive login shell in the given directory;
// what its startup files print before the marker is skipped.
func TestRead(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	shell := fakeShell(t, ""+
		"[ \"$1 $2 $3\" = \"-l -i -c\" ] || exit 1\n"+
		"[ \"$(pwd -P)\" = \""+home+"\" ] || exit 1\n"+
		"echo noise from startup files\n"+
		"MISE_BIN=/login/mise/bin; export MISE_BIN\n"+
		"exec /bin/sh -c \"$4\"\n")
	env := read(shell, home, 5*time.Second)
	if !slices.Contains(env, "MISE_BIN=/login/mise/bin") {
		t.Fatalf("env = %q", env)
	}
	if slices.ContainsFunc(env, func(kv string) bool { return strings.Contains(kv, "noise") }) {
		t.Fatal("startup noise in env")
	}
}

func TestReadFailures(t *testing.T) {
	if env := read("/bin/sh", "", 5*time.Second); len(env) == 0 {
		t.Fatal("no environment from /bin/sh")
	}
	if env := read("/no/such/shell", "", time.Second); env != nil {
		t.Fatalf("env from a missing shell: %q", env)
	}
	if env := read("", "", time.Second); env != nil {
		t.Fatalf("env without a shell: %q", env)
	}
	if env := read(fakeShell(t, "echo A=1\n"), "", time.Second); env != nil {
		t.Fatalf("env without the marker: %q", env)
	}
	if env := read(fakeShell(t, "exit 3\n"), "", time.Second); env != nil {
		t.Fatalf("env from a failed shell: %q", env)
	}
}

// A shell that hangs is killed with its whole group at the timeout.
func TestReadTimeout(t *testing.T) {
	start := time.Now()
	if env := read(fakeShell(t, "sleep 30 &\nsleep 30\n"), "", 300*time.Millisecond); env != nil {
		t.Fatalf("env from a hanging shell: %q", env)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("read took %v", d)
	}
}

// A background job left by the startup files keeps the output open after
// the shell exits; what the shell printed still counts.
func TestReadBackgroundJob(t *testing.T) {
	start := time.Now()
	env := read(fakeShell(t, "sleep 30 &\nexec /bin/sh -c \"$4\"\n"), "", 10*time.Second)
	if len(env) == 0 {
		t.Fatal("no environment")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("read took %v", d)
	}
}

func TestShell(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/fish")
	if got := Shell(); got != "/usr/bin/fish" {
		t.Fatal(got)
	}
	passwd := filepath.Join(t.TempDir(), "passwd")
	uid := strconv.Itoa(os.Getuid())
	data := "root:x:0:0:root:/root:/bin/bash\n" +
		"broken line\n" +
		"me:x:" + uid + ":" + uid + "::/home/me:/usr/bin/zsh\n"
	if err := os.WriteFile(passwd, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	old := passwdFile
	passwdFile = passwd
	t.Cleanup(func() { passwdFile = old })

	// wsl.exe --exec sets no $SHELL; a relative one is not used either.
	for _, v := range []string{"", "zsh"} {
		t.Setenv("SHELL", v)
		if got := Shell(); got != "/usr/bin/zsh" {
			t.Fatalf("SHELL=%q: %q", v, got)
		}
	}
	passwdFile = filepath.Join(t.TempDir(), "missing")
	if got := Shell(); got != "/bin/sh" && got != "/bin/zsh" {
		t.Fatal(got)
	}
}

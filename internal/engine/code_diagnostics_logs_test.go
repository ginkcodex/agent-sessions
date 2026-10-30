package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	cd "github.com/ginkcode/agent-sessions/internal/codediagnostics"
	mt "github.com/ginkcode/agent-sessions/internal/managedtools"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

func logEngine(t *testing.T) (*Engine, *checkLogs, string) {
	t.Helper()
	data := t.TempDir()
	e := NewEngine(WithCacheEnabled(false), WithRoots(paths.Roots{Claude: "/nonexistent/claude", Config: t.TempDir(), Data: data}), WithFixtureMachineIdentity(""))
	t.Cleanup(func() { _ = e.Close() })
	l, err := e.codeCheckLogs()
	if err != nil {
		t.Fatal(err)
	}
	return e, l, filepath.Join(data, "code-diagnostics", "logs")
}

func readLog(t *testing.T, dir, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func wantLines(t *testing.T, text string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("missing %q in\n%s", w, text)
		}
	}
}

func TestServerLogFormatPermissionsAndPruning(t *testing.T) {
	_, l, dir := logEngine(t)
	start := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	report := cd.SessionFailure{
		Project: cd.Project{HostID: "local:test", Root: "/repo"}, ProviderID: "svelte",
		Executable: "/bin/node", Args: []string{"/srv/bin/server.js", "--stdio"}, Directory: "frontend",
		Started: start, Failed: start.Add(1500 * time.Millisecond), Phase: "initialized",
		Cause: errors.New("toolFailed: Language server exited"), Transport: io.EOF, Exit: "exit status 1",
		Stderr: "starting\n", StderrTail: "FATAL ERROR: heap out of memory\n", StderrTruncated: true, StderrBytes: 100,
	}
	l.session(report)
	l.session(report) // same millisecond: a second file, never an overwrite

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("%v %v", entries, err)
	}
	const first = "20261028T010204.500Z-svelte-server.log"
	if names := entries[0].Name() + " " + entries[1].Name(); names != "20261028T010204.500Z-svelte-server-2.log "+first {
		t.Fatalf("names %s", names)
	}
	wantLines(t, readLog(t, dir, first),
		"Language server failure\n", "provider:  svelte\n", "project:   local:test /repo\n", "directory: frontend\n",
		`command:   "/bin/node" "/srv/bin/server.js" "--stdio"`+"\n", "phase:     initialized (up 1.5s)\n",
		"cause:     toolFailed: Language server exited\n", "transport: EOF\n", "exit:      exit status 1\n",
		"stderr:    100 bytes\n", "--- stderr ---\nstarting\n\n--- 59 bytes omitted ---\nFATAL ERROR: heap out of memory\n--- end ---\n",
	)
	if runtime.GOOS != "windows" {
		if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
			t.Errorf("dir mode %v", st.Mode())
		}
		if st, _ := os.Stat(filepath.Join(dir, first)); st.Mode().Perm() != 0o600 {
			t.Errorf("file mode %v", st.Mode())
		}
	}

	// Only the newest maxCheckLogs survive.
	for i := range maxCheckLogs + 5 {
		r := report
		r.Failed = start.Add(time.Duration(i+10) * time.Second)
		r.ProviderID = "type script"
		l.session(r)
	}
	entries, _ = os.ReadDir(dir)
	if len(entries) != maxCheckLogs || entries[len(entries)-1].Name() != "20261028T010307.000Z-type_script-server.log" {
		t.Fatalf("%d logs, newest %s", len(entries), entries[len(entries)-1].Name())
	}
}

func TestServerLogWithoutStderrOrProcess(t *testing.T) {
	text := formatSessionLog(cd.SessionFailure{ProviderID: "go", Phase: "starting", Failed: time.Now(), Cause: errors.New("toolFailed: Language server could not be started")})
	if !strings.Contains(text, "transport: (none)\n") || !strings.Contains(text, "exit:      (none)\n") ||
		!strings.Contains(text, "directory: .\n") || strings.Contains(text, "--- stderr ---") || strings.Contains(text, "(up ") {
		t.Fatal(text)
	}
}

func TestCheckAndInstallLogs(t *testing.T) {
	_, l, dir := logEngine(t)
	start := time.Date(2026, 10, 28, 1, 2, 3, 0, time.UTC)
	big := "first\n" + strings.Repeat("x", 40<<10) + "last\n"
	l.check(cd.CheckFailure{
		Project: cd.Project{HostID: "local:test", Root: "/repo"}, ProviderID: "go-vet", Executable: "/usr/bin/go",
		Args: []string{"vet", "./..."}, Started: start, Finished: start.Add(2 * time.Second), Kind: cd.OutcomeToolFailed,
		Reason: "Checker exited with status 2", ExitCode: 2,
		Stdout: cd.BoundOutput(nil, false), Stderr: cd.BoundOutput([]byte(big), true),
	})
	l.check(cd.CheckFailure{
		ProviderID: "cargo-check", Executable: "/bin/cargo", Preparation: true, Started: start, Finished: start.Add(time.Second),
		Kind: cd.OutcomeToolFailed, Reason: "Checker could not be started", ExitCode: -1, Launch: "exec: no such file",
	})
	l.install(mt.InstallReport{
		Tool: "svelte-language-server", Version: "0.17.0", Step: "installing the svelte-language-server packages with npm",
		Started: start, Failed: start.Add(3 * time.Second), Err: errors.New("node: exit status 1"),
		Command: []string{"/node/bin/node", "npm-cli.js", "ci"}, Output: []byte("npm ERR! code E404\n"),
	})

	check := readLog(t, dir, "20261028T010205.000Z-go-vet-check.log")
	wantLines(t, check, "Code check failure\n", `command:   "/usr/bin/go" "vet" "./..."`+"\n", "outcome:   toolFailed\n",
		"reason:    Checker exited with status 2\n", "exit:      status 2\n", "duration:  2s\n", "stdout:    0 bytes\n",
		"stderr:    40971 bytes (capture limit reached)\n", "--- stderr ---\nfirst\n", "\n--- 24587 bytes omitted ---\n", "xlast\n--- end ---\n")
	if strings.Contains(check, "--- stdout ---") || strings.Contains(check, "launch:") {
		t.Error(check)
	}
	wantLines(t, readLog(t, dir, "20261028T010204.000Z-cargo-check-setup.log"), "Code check setup failure\n",
		"exit:      (none)\n", "launch:    exec: no such file\n")
	wantLines(t, readLog(t, dir, "20261028T010206.000Z-svelte-language-server-install.log"), "Managed tool installation failure\n",
		"tool:      svelte-language-server 0.17.0\n", "step:      installing the svelte-language-server packages with npm\n",
		"cause:     node: exit status 1\n", "duration:  3s\n", `command:   "/node/bin/node" "npm-cli.js" "ci"`+"\n",
		"output:    19 bytes\n", "--- output ---\nnpm ERR! code E404\n--- end ---\n")
}

func TestClearCodeCheckLogs(t *testing.T) {
	e, l, dir := logEngine(t)
	info, err := e.CodeCheckLogs(context.Background())
	if err != nil || info.Dir != dir || info.Files != 0 || info.Bytes != 0 {
		t.Fatalf("%+v %v", info, err)
	}
	l.check(cd.CheckFailure{ProviderID: "go-vet", Finished: time.Now(), Reason: "Checker wrote to stderr"})
	l.install(mt.InstallReport{Tool: "ruff", Failed: time.Now()})
	// Clearing deletes logs only.
	keep := filepath.Join(dir, "notes.txt")
	if err = os.WriteFile(keep, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(dir, "dir.log"), 0o700); err != nil {
		t.Fatal(err)
	}
	if info, err = e.CodeCheckLogs(context.Background()); err != nil || info.Files != 2 || info.Bytes == 0 {
		t.Fatalf("%+v %v", info, err)
	}
	if info, err = e.ClearCodeCheckLogs(context.Background()); err != nil || info.Files != 0 || info.Bytes != 0 {
		t.Fatalf("%+v %v", info, err)
	}
	for _, p := range []string{keep, filepath.Join(dir, "dir.log")} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

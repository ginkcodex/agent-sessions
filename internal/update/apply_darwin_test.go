package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWaitExitReturnsWhenTheProcessEnds(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = cmd.Wait(); close(reaped) }()

	done := make(chan error, 1)
	go func() { done <- waitExit(context.Background(), cmd.Process.Pid, 10*time.Millisecond) }()
	select {
	case err := <-done:
		t.Fatalf("returned %v while the process ran", err)
	case <-time.After(100 * time.Millisecond):
	}
	_ = cmd.Process.Kill()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not notice the process ending")
	}
	<-reaped
}

func TestWaitExitGivesUpWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	// This test process outlives the wait.
	if err := waitExit(ctx, osPID(), 10*time.Millisecond); err == nil {
		t.Fatal("waitExit = nil for a running process")
	}
}

func TestApplyMainOnlyAnswersToItsFlag(t *testing.T) {
	if _, ok := ApplyMain(nil); ok {
		t.Fatal("answered to no arguments")
	}
	if _, ok := ApplyMain([]string{"--other"}); ok {
		t.Fatal("answered to another flag")
	}
	if code, ok := ApplyMain([]string{applyFlag, "/tmp/x/1.6.0"}); !ok || code != 2 {
		t.Fatalf("missing arguments: code %d, ok %v", code, ok)
	}
}

func osPID() int { return os.Getpid() }

func TestStartHelperWaitsForTheHelperToReportIn(t *testing.T) {
	cmd := exec.Command("sh", "-c", `printf 'ok\n' >&3; exec 3>&-; sleep 0.2`)
	if err := startHelper(cmd, 5*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestStartHelperReportsAHelperThatCannotRun(t *testing.T) {
	cases := []struct {
		name, script, want string
	}{
		// What macOS does to a Developer ID executable copied out of its bundle.
		{"killed at once", `kill -9 $$`, "signal: killed"},
		{"exits without reporting", `exit 2`, "exit status 2"},
		{"closes the pipe first", `exec 3>&-; sleep 5`, "did not start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", c.script)
			start := time.Now()
			err := startHelper(cmd, 5*time.Second)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if time.Since(start) > 3*time.Second {
				t.Fatalf("took %s to notice", time.Since(start))
			}
			if cmd.ProcessState == nil {
				t.Fatal("the helper was not reaped")
			}
		})
	}
}

func TestStartHelperGivesUpOnASilentHelper(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	err := startHelper(cmd, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "within") {
		t.Fatalf("err = %v", err)
	}
	if cmd.ProcessState == nil || cmd.ProcessState.Success() {
		t.Fatalf("the silent helper was left running: %v", cmd.ProcessState)
	}
}

func TestCopyBundleKeepsFilesModesAndLinks(t *testing.T) {
	for _, name := range []string{"clone", "copy"} {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(t.TempDir(), "A.app")
			writeTree(t, src)
			dst := filepath.Join(t.TempDir(), "A.app")
			copyFn := copyBundle
			if name == "copy" {
				copyFn = copyTree
			}
			if err := copyFn(src, dst); err != nil {
				t.Fatal(err)
			}
			exe := filepath.Join(dst, "Contents", "MacOS", "a")
			if info, err := os.Stat(exe); err != nil || info.Mode().Perm() != 0o755 {
				t.Fatalf("executable: %v %v", info, err)
			}
			if b, err := os.ReadFile(filepath.Join(dst, "Contents", "Info.plist")); err != nil || string(b) != "plist" {
				t.Fatalf("Info.plist: %q %v", b, err)
			}
			if link, err := os.Readlink(filepath.Join(dst, "Contents", "Current")); err != nil || link != "MacOS" {
				t.Fatalf("link: %q %v", link, err)
			}
		})
	}
}

func writeTree(t *testing.T, root string) {
	t.Helper()
	for path, body := range map[string]string{
		"Contents/Info.plist":                   "plist",
		"Contents/_CodeSignature/CodeResources": "seal",
		"Contents/Resources/icon.icns":          "icon",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Contents", "MacOS", "a"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("MacOS", filepath.Join(root, "Contents", "Current")); err != nil {
		t.Fatal(err)
	}
}

// TestSignedHelperCopyRuns checks the helper's copy of a real Developer ID
// app can run. Set AGENT_SESSIONS_SIGNED_APP to such an app, for example
// "/Applications/Agent Sessions.app"; CI has none, so it is skipped there.
func TestSignedHelperCopyRuns(t *testing.T) {
	app := os.Getenv("AGENT_SESSIONS_SIGNED_APP")
	if app == "" {
		t.Skip("AGENT_SESSIONS_SIGNED_APP is not set")
	}
	dst := filepath.Join(t.TempDir(), filepath.Base(app))
	if err := copyBundle(app, dst); err != nil {
		t.Fatal(err)
	}
	// Missing arguments: the helper prints its usage and exits 2. A copy
	// macOS refuses to run is killed before it gets that far.
	cmd := exec.Command(filepath.Join(dst, "Contents", "MacOS", "agent-sessions"), applyFlag, "/nonexistent")
	out, err := cmd.CombinedOutput()
	if code := cmd.ProcessState.ExitCode(); code != 2 {
		t.Fatalf("helper copy: exit %d (%v): %s", code, err, out)
	}
}

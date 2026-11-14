package update

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperModeEnv = "AGENT_SESSIONS_TEST_UPDATE_HELPER"

// TestHelperProcess is the helper that the tests below start, as a child
// process of the test binary. It does nothing in a normal test run.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperModeEnv)
	if mode == "" {
		return
	}
	switch mode {
	case "report":
		// As ApplyMain does: report on the handle named in the arguments.
		args := os.Args
		for i, a := range args {
			if a == "--notify-handle" && i+1 < len(args) {
				h, err := strconv.ParseUint(args[i+1], 10, 64)
				if err != nil {
					os.Exit(4)
				}
				reportStarted(uintptr(h))
				os.Exit(0)
			}
		}
		os.Exit(5) // no handle given
	case "exit":
		os.Exit(3)
	case "silent":
		time.Sleep(time.Minute)
	}
	os.Exit(6)
}

func startTestHelper(t *testing.T, mode string, timeout time.Duration) error {
	t.Helper()
	t.Setenv(helperModeEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return startDetached(exe, []string{"-test.run=^TestHelperProcess$", "--"}, timeout)
}

func TestStartDetachedWaitsForTheHelperToReportIn(t *testing.T) {
	if err := startTestHelper(t, "report", 20*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestStartDetachedReportsAHelperThatCannotRun(t *testing.T) {
	err := startTestHelper(t, "exit", 20*time.Second)
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("err = %v, want the helper's exit status", err)
	}
}

func TestStartDetachedGivesUpOnASilentHelper(t *testing.T) {
	start := time.Now()
	err := startTestHelper(t, "silent", 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "within") {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 15*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
}

func TestStartDetachedReportsAMissingProgram(t *testing.T) {
	err := startDetached(`C:\does-not-exist\agent-sessions-apply.exe`, nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "start the update helper") {
		t.Fatalf("err = %v", err)
	}
}

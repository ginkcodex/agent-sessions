//go:build !windows

package rpc

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func assertProcessStopped(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		if runtime.GOOS == "linux" {
			// An orphan may stay a zombie until init reaps it; it cannot run.
			data, _ := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
			if strings.Contains(string(data), ") Z ") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still running", pid)
}

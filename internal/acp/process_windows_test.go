package acp_test

import (
	"testing"

	"golang.org/x/sys/windows"
)

func assertStopped(t *testing.T, pid int) {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // the process is already gone
	}
	defer func() { _ = windows.CloseHandle(h) }()
	status, err := windows.WaitForSingleObject(h, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant %d is still running: %v %v", pid, status, err)
	}
}

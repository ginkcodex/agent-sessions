package rpc

import (
	"testing"

	"golang.org/x/sys/windows"
)

func assertProcessStopped(t *testing.T, pid int) {
	t.Helper()
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // the process is already gone
	}
	defer func() { _ = windows.CloseHandle(h) }()
	status, err := windows.WaitForSingleObject(h, 10000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("process %d is still running: %v %v", pid, status, err)
	}
}

package update

import (
	"context"
	"os"
	"os/exec"
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

package app

import (
	"slices"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/update"
)

func TestLinuxReportsHowItGetsUpdates(t *testing.T) {
	if installMethod == nil {
		t.Fatal("no install method on Linux")
	}
	got := installMethod()
	if !slices.Contains([]string{update.InstallAptRepo, update.InstallDnfRepo, update.InstallAUR, update.InstallManual}, got) {
		t.Fatalf("installMethod() = %q", got)
	}
}

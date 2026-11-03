package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The desktop entry has to be named after the application id, and claim its
// windows' WM_CLASS (the program name), or the dock shows a generic icon and
// the raw id.
func TestDesktopEntryMatchesLinuxAppID(t *testing.T) {
	path := filepath.Join("..", "..", "packaging", "linux", linuxAppID+".desktop")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no desktop entry named after the application id: %v", err)
	}
	want := "StartupWMClass=" + linuxProgramName
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == want {
			return
		}
	}
	t.Errorf("%s lacks %q", path, want)
}

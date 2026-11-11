//go:build darwin && desktop

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpaqueWindowExperimentSwitch(t *testing.T) {
	for _, value := range []string{"", "0", "1"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("AGENT_SESSIONS_MACOS_OPAQUE", value)
			if got, want := opaqueWindowExperimentEnabled(), value != "0"; got != want {
				t.Fatalf("enabled = %v, want %v", got, want)
			}
		})
	}
}

// Exercise the actual Objective-C helper against an unshown AppKit window on
// the process main thread. Go test goroutines are not the AppKit main thread.
// This checks native properties, not WKWebView rendering or GPU performance.
func TestNativeWindowOpaqueBacking(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate native test fixture")
	}
	dir := filepath.Dir(file)
	binary := filepath.Join(t.TempDir(), "window-opacity-test")
	compile := exec.Command("xcrun", "clang", "-fobjc-arc", "-framework", "Cocoa",
		filepath.Join(dir, "windowopacity_darwin.m"),
		filepath.Join(dir, "testdata", "windowopacity.m"), "-o", binary)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile AppKit fixture: %v\n%s", err, output)
	}
	if output, err := exec.Command(binary).CombinedOutput(); err != nil {
		t.Fatalf("AppKit fixture: %v\n%s", err, output)
	}
}

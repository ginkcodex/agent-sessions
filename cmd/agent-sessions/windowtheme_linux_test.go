//go:build linux && desktop

package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Run the GTK helper in a subprocess so GTK owns its initial main thread.
func TestNativeLinuxWindowTheme(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate native test fixture")
	}
	dir := filepath.Dir(file)
	flags, err := exec.Command("pkg-config", "--cflags", "--libs", "gtk4").Output()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "window-theme-test")
	args := []string{
		"-Wall", "-Wextra", "-Werror", filepath.Join(dir, "windowtheme_linux.c"),
		filepath.Join(dir, "testdata", "windowtheme_linux.c"), "-o", binary,
	}
	args = append(args, strings.Fields(string(flags))...)
	if output, err := exec.Command("cc", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile GTK fixture: %v\n%s", err, output)
	}
	output, err := exec.Command(binary).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 77 {
		t.Skipf("GTK display unavailable: %s", output)
	}
	if err != nil {
		t.Fatalf("GTK fixture: %v\n%s", err, output)
	}
	t.Logf("%s", output)
}

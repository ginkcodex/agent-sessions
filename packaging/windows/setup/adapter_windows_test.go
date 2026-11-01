//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// These tests never invoke msiexec or modify an installed product.
func TestSystemInstallerPath(t *testing.T) {
	path, err := systemMSIExec()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(path, filepath.Join(dir, "msiexec.exe")) {
		t.Fatalf("untrusted installer path %q", path)
	}
}

func TestSetupMutexExcludesSecondInstance(t *testing.T) {
	// Use a test-only identity so this test cannot block the actual setup.
	name := "test-" + filepath.Base(t.TempDir())
	first, err := acquireSetupMutex(name)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(first)
	second, err := acquireSetupMutex(name)
	if err == nil {
		windows.CloseHandle(second)
		t.Fatal("second setup acquired the same lock")
	}
}

func TestReadOnlyProductDiscovery(t *testing.T) {
	products, err := installedProducts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range products {
		if !validGUID(p.Code) {
			t.Fatalf("invalid product identity %q", p.Code)
		}
		if _, err := parseVersion(p.Version); err != nil {
			t.Fatal(err)
		}
		t.Logf("discovered product %s version %s context %d", p.Code, p.Version, p.Context)
	}
}

func TestValidateMSIReadOnly(t *testing.T) {
	// Optional real-payload check. Only opens the database for reading; never
	// executes installation actions, even when testing a production MSI.
	path := os.Getenv("AGENT_SESSIONS_SETUP_TEST_MSI")
	version := os.Getenv("AGENT_SESSIONS_SETUP_TEST_VERSION")
	if path == "" || version == "" {
		t.Skip("set AGENT_SESSIONS_SETUP_TEST_MSI and AGENT_SESSIONS_SETUP_TEST_VERSION")
	}
	if err := validateMSI(path, version); err != nil {
		t.Fatal(err)
	}
	if err := validateMSI(path, "255.255.65535"); err == nil {
		t.Fatal("accepted mismatched bundle version")
	}
}

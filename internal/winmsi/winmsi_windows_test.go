//go:build windows

package winmsi

import (
	"os"
	"testing"
)

func TestReadOnlyProductDiscovery(t *testing.T) {
	products, err := InstalledProducts()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range products {
		if !ValidGUID(p.Code) {
			t.Fatalf("invalid product identity %q", p.Code)
		}
		if _, err := ParseVersion(p.Version); err != nil {
			t.Fatal(err)
		}
		t.Logf("discovered product %s version %s context %d", p.Code, p.Version, p.Context)
	}
}

func TestReadInfoOfARealMSI(t *testing.T) {
	// Optional real-package check. It only opens the database for reading.
	path, version := os.Getenv("AGENT_SESSIONS_SETUP_TEST_MSI"), os.Getenv("AGENT_SESSIONS_SETUP_TEST_VERSION")
	if path == "" || version == "" {
		t.Skip("set AGENT_SESSIONS_SETUP_TEST_MSI and AGENT_SESSIONS_SETUP_TEST_VERSION")
	}
	info, err := ReadInfo(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := info.Check(version); err != nil {
		t.Fatalf("%+v: %v", info, err)
	}
	if err := info.Check("255.255.65535"); err == nil {
		t.Fatal("accepted a mismatched version")
	}
}

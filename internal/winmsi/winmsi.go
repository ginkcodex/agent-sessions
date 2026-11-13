// Package winmsi reads Windows Installer packages and registrations, for the
// setup wrapper and the in-app updater. The checks that need no Windows (the
// version and identity rules) live here; reading an MSI and the installed
// products is in winmsi_windows.go.
package winmsi

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// UpgradeCode is how a new Agent Sessions MSI finds the installed one.
	// It never changes (packaging/windows/agent-sessions.wxs).
	UpgradeCode = "{D7786BE1-BF16-4887-9C3B-345C5D87CD95}"
	// ProductName is the MSI's ProductName.
	ProductName = "Agent Sessions"
	// UserUnmanaged is the Windows Installer context of a per-user install
	// that no administrator manages (MSIINSTALLCONTEXT_USERUNMANAGED).
	UserUnmanaged = 2
)

// Version is an MSI ProductVersion. MSI compares the first three numeric
// fields, not semantic versions.
type Version [3]uint64

// ParseVersion reads "major.minor.build": the first two fields at most 255
// and the last at most 65535.
func ParseVersion(s string) (Version, error) {
	var v Version
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("invalid MSI version %q (expected major.minor.build)", s)
	}
	for i, p := range parts {
		if p == "" || strings.IndexFunc(p, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return v, fmt.Errorf("invalid MSI version %q", s)
		}
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || (i < 2 && n > 255) {
			return v, fmt.Errorf("MSI version out of range: %q", s)
		}
		v[i] = n
	}
	return v, nil
}

// NewerThan reports whether v is a later version than other.
func (v Version) NewerThan(other Version) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] > other[i]
		}
	}
	return false
}

// ForRelease is the MSI ProductVersion of a release version: the numeric
// major.minor.patch, without a leading "v" or a pre-release suffix.
func ForRelease(release string) string {
	release = strings.TrimPrefix(strings.TrimSpace(release), "v")
	if i := strings.IndexAny(release, "-+"); i >= 0 {
		release = release[:i]
	}
	return release
}

// Product is an installed product registration.
type Product struct {
	Code, Version, SID string
	Context            uint32
}

// Info is what an MSI file says about itself.
type Info struct {
	ProductCode, UpgradeCode, ProductName, ProductVersion string
	// AllUsers and PerUser are the ALLUSERS and MSIINSTALLPERUSER properties.
	AllUsers, PerUser string
}

// Check says whether the MSI is an unmanaged per-user Agent Sessions package
// of the given MSI version.
func (i Info) Check(expectedVersion string) error {
	if !ValidGUID(i.ProductCode) || !strings.EqualFold(i.UpgradeCode, UpgradeCode) {
		return fmt.Errorf("the MSI has an invalid ProductCode or does not belong to Agent Sessions")
	}
	if i.ProductName != ProductName {
		return fmt.Errorf("the MSI is for %q, not %s", i.ProductName, ProductName)
	}
	if i.AllUsers != "" || (i.PerUser != "" && i.PerUser != "1") {
		return fmt.Errorf("the MSI is not an unmanaged per-user package")
	}
	version, err := ParseVersion(i.ProductVersion)
	if err != nil {
		return err
	}
	expected, err := ParseVersion(expectedVersion)
	if err != nil {
		return err
	}
	if version != expected {
		return fmt.Errorf("the MSI's version %s differs from the expected %s", i.ProductVersion, expectedVersion)
	}
	return nil
}

// ValidGUID reports whether s is a braced GUID such as
// {D7786BE1-BF16-4887-9C3B-345C5D87CD95}.
func ValidGUID(s string) bool {
	if len(s) != 38 || s[0] != '{' || s[37] != '}' {
		return false
	}
	for i := 1; i < 37; i++ {
		if i == 9 || i == 14 || i == 19 || i == 24 {
			if s[i] != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", rune(s[i])) {
			return false
		}
	}
	return true
}

// InstallerArgs are the msiexec arguments for a quiet install ("/i") or
// removal ("/x") of target with a verbose log: no restart, no dialogs.
func InstallerArgs(operation, target, logPath string) []string {
	return []string{operation, target, "/quiet", "/norestart", "/L*v", logPath,
		"REBOOT=ReallySuppress", "MSIRESTARTMANAGERCONTROL=DisableShutdown", "MSIDISABLERMRESTART=1"}
}

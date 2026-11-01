package main

import (
	"fmt"
	"strconv"
	"strings"
)

const upgradeCode = "{D7786BE1-BF16-4887-9C3B-345C5D87CD95}"

// MSI compares the first three numeric version fields, not semantic versions.
type msiVersion [3]uint64

func parseVersion(s string) (msiVersion, error) {
	var v msiVersion
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

func (v msiVersion) newerThan(other msiVersion) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] > other[i]
		}
	}
	return false
}

type product struct {
	Code, Version, SID string
	Context            uint32
}

const userUnmanaged = 2

type payload struct {
	Path, Directory, Version, Code string
}

// platform keeps the destructive sequencing independently testable. Discover
// must return every related registration visible to the current user, including
// unsupported contexts; filtering must never silently turn a conflict into a
// fresh installation.
type platform interface {
	Prepare() (payload, error)
	Prerequisite() error
	Discover() ([]product, error)
	CurrentSID() string
	Confirm(string) bool
	Running() (bool, error)
	RetryClose() bool
	Execute(operation, target, logPath string) (uint32, error)
	LogPath(string) string
	Trace(string)
}

type outcome struct {
	Operation string
	Status    uint32
	Message   string
	Payload   payload
	Success   bool
}

func runSetup(p platform) outcome {
	out := outcome{Operation: "preflight"}
	fail := func(message string) outcome { out.Message = message; p.Trace(message); return out }
	var err error
	out.Payload, err = p.Prepare()
	if err != nil {
		return fail(err.Error())
	}
	version, err := parseVersion(out.Payload.Version)
	if err != nil {
		return fail(err.Error())
	}
	if err := p.Prerequisite(); err != nil {
		return fail(err.Error())
	}
	products, err := p.Discover()
	if err != nil {
		return fail("Cannot discover installed products: " + err.Error())
	}
	seen := make(map[string]bool)
	for _, product := range products {
		if product.Context != userUnmanaged || product.SID != p.CurrentSID() {
			return fail("A conflicting machine-wide, managed, or other-user installation was found. Remove it using its original account/installer before running setup.")
		}
		if !validGUID(product.Code) || seen[strings.ToUpper(product.Code)] {
			return fail("Invalid or duplicate installed product registration.")
		}
		seen[strings.ToUpper(product.Code)] = true
		installed, err := parseVersion(product.Version)
		if err != nil {
			return fail(err.Error())
		}
		if installed.newerThan(version) {
			return fail("A newer version (" + product.Version + ") is installed. Setup will not downgrade or remove it.")
		}
	}
	if !p.Confirm("Install Agent Sessions " + out.Payload.Version + "?\n\nSetup first uninstalls any existing Agent Sessions installation, then installs a fresh copy, even for the same version. Settings and session data are kept.\n\nIf the new installation fails, the old app cannot be restored automatically and the app may be absent. The recovery MSI and logs will be kept at:\n" + out.Payload.Directory + "\n\nClick Install to continue.") {
		out.Status = 1602
		return fail("Setup cancelled before removing anything.")
	}
	closeApp := func() error {
		for {
			running, err := p.Running()
			if err != nil {
				return fmt.Errorf("cannot safely check running Agent Sessions processes: %w", err)
			}
			if !running {
				return nil
			}
			if !p.RetryClose() {
				out.Status = 1602
				return fmt.Errorf("setup cancelled; close Agent Sessions before retrying")
			}
		}
	}
	if err := closeApp(); err != nil {
		return fail(err.Error())
	}
	for i, product := range products {
		if i > 0 {
			if err := closeApp(); err != nil {
				return fail(err.Error())
			}
		}
		current, err := p.Discover()
		if err != nil {
			return fail("Cannot revalidate installed products: " + err.Error())
		}
		if !sameProducts(current, products[i:]) {
			return fail("Installed products changed during setup. Nothing further was removed; rerun setup.")
		}
		out.Operation = "uninstall " + product.Code
		out.Status, err = p.Execute("/x", product.Code, p.LogPath(fmt.Sprintf("uninstall-%02d.log", i+1)))
		if err != nil {
			return fail("Could not start/wait for uninstall: " + err.Error())
		}
		if out.Status == 3010 || out.Status == 1641 {
			return fail(statusMessage("Uninstall", out.Status) + " Installation was not started. Rerun setup after restarting Windows.")
		}
		if out.Status != 0 && out.Status != 1605 && out.Status != 1614 {
			return fail(statusMessage("Uninstall", out.Status))
		}
		// Even an already-removed result is safe only after absence is verified.
		remaining, err := p.Discover()
		if err != nil {
			return fail("Cannot verify removal: " + err.Error())
		}
		for _, remaining := range remaining {
			if strings.EqualFold(remaining.Code, product.Code) {
				return fail("Uninstall returned, but the product is still registered. Installation was not started.")
			}
		}
	}
	out.Operation = "verify removal"
	remaining, err := p.Discover()
	if err != nil {
		return fail("Cannot verify removal: " + err.Error())
	}
	if len(remaining) != 0 {
		return fail("An Agent Sessions registration remains or appeared during setup. Installation was not started.")
	}
	if err := closeApp(); err != nil {
		return fail(err.Error())
	}
	remaining, err = p.Discover()
	if err != nil {
		return fail("Cannot recheck removal: " + err.Error())
	}
	if len(remaining) != 0 {
		return fail("A product appeared while waiting for the app to close. Installation was not started.")
	}
	out.Operation = "install"
	out.Status, err = p.Execute("/i", out.Payload.Path, p.LogPath("install.log"))
	if err != nil {
		return fail("Could not start/wait for installation: " + err.Error())
	}
	if out.Status != 0 && out.Status != 3010 && out.Status != 1641 {
		return fail(statusMessage("Installation", out.Status))
	}
	installed, err := p.Discover()
	if err != nil {
		return fail("Cannot verify installation: " + err.Error())
	}
	if len(installed) != 1 || !strings.EqualFold(installed[0].Code, out.Payload.Code) || installed[0].Context != userUnmanaged || installed[0].SID != p.CurrentSID() {
		return fail("Installation returned, but the expected current-user registration could not be verified.")
	}
	installedVersion, err := parseVersion(installed[0].Version)
	if err != nil || installedVersion != version {
		return fail("Installation returned, but the expected version could not be verified.")
	}
	out.Success = true
	out.Message = "Agent Sessions was installed successfully. You can now launch it normally."
	if out.Status == 3010 || out.Status == 1641 {
		out.Message = statusMessage("Installation completed", out.Status)
	}
	p.Trace(out.Message)
	return out
}

func statusMessage(operation string, status uint32) string {
	switch status {
	case 1602:
		return operation + " was cancelled."
	case 1618:
		return operation + " could not proceed because another Windows Installer transaction is running. Wait for it to finish and rerun setup."
	case 1641:
		return operation + " reported that a restart was initiated. Do not start the app; let Windows restart before retrying."
	case 3010:
		return operation + " requires a restart. Restart Windows before running setup or the app again."
	default:
		return fmt.Sprintf("%s failed (Windows Installer status %d).", operation, status)
	}
}

func validGUID(s string) bool {
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

func sameProducts(a, b []product) bool {
	if len(a) != len(b) {
		return false
	}
	for _, item := range a {
		found := false
		for _, expected := range b {
			if item == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

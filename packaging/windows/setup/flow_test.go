package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testCode = "{00000000-0000-0000-0000-000000000001}"
const testCode2 = "{00000000-0000-0000-0000-000000000002}"

type fakePlatform struct {
	mutateAt                                                         int
	wrongInstall                                                     bool
	products                                                         []product
	events                                                           []string
	confirm, retry                                                   bool
	running                                                          []bool
	statuses                                                         []uint32
	prepareErr, prerequisiteErr, discoverErr, runningErr, executeErr error
	keepProduct                                                      bool
	discoverCount                                                    int
	appearAt                                                         int
}

func newFake() *fakePlatform { return &fakePlatform{confirm: true} }
func oldProduct() product {
	return product{Code: testCode, Version: "0.9.35", SID: "user", Context: userUnmanaged}
}
func (p *fakePlatform) Prepare() (payload, error) {
	p.events = append(p.events, "prepare")
	return payload{Path: "recovery/payload.msi", Directory: "recovery", Version: "0.9.36", Code: testCode}, p.prepareErr
}
func (p *fakePlatform) Prerequisite() error {
	p.events = append(p.events, "prerequisite")
	return p.prerequisiteErr
}
func (p *fakePlatform) Discover() ([]product, error) {
	p.events = append(p.events, "discover")
	p.discoverCount++
	if p.mutateAt == p.discoverCount && len(p.products) > 0 {
		p.products[0].Version = "1.0.0"
	}
	if p.appearAt == p.discoverCount {
		p.products = append(p.products, oldProduct())
	}
	return append([]product(nil), p.products...), p.discoverErr
}
func (p *fakePlatform) CurrentSID() string { return "user" }
func (p *fakePlatform) Confirm(s string) bool {
	p.events = append(p.events, "confirm")
	return p.confirm
}
func (p *fakePlatform) Running() (bool, error) {
	p.events = append(p.events, "running")
	if len(p.running) == 0 {
		return false, p.runningErr
	}
	r := p.running[0]
	p.running = p.running[1:]
	return r, p.runningErr
}
func (p *fakePlatform) RetryClose() bool { p.events = append(p.events, "retry"); return p.retry }
func (p *fakePlatform) Execute(op, target, logPath string) (uint32, error) {
	p.events = append(p.events, op+" "+target)
	status := uint32(0)
	if len(p.statuses) > 0 {
		status = p.statuses[0]
		p.statuses = p.statuses[1:]
	}
	if op == "/x" && !p.keepProduct && (status == 0 || status == 1605 || status == 1614) {
		var remaining []product
		for _, product := range p.products {
			if product.Code != target {
				remaining = append(remaining, product)
			}
		}
		p.products = remaining
	}
	if op == "/i" && (status == 0 || status == 3010 || status == 1641) {
		p.products = []product{{Code: testCode, Version: "0.9.36", SID: "user", Context: 2}}
		if p.wrongInstall {
			p.products[0].Code = testCode2
		}
	}
	return status, p.executeErr
}
func (p *fakePlatform) LogPath(name string) string { return "recovery/" + name }
func (p *fakePlatform) Trace(string)               {}
func transactions(events []string) []string {
	var result []string
	for _, event := range events {
		if strings.HasPrefix(event, "/") {
			result = append(result, event)
		}
	}
	return result
}

func TestSetupInstallSequences(t *testing.T) {
	for _, test := range []struct {
		name     string
		products []product
		want     []string
	}{
		{"fresh", nil, []string{"/i recovery/payload.msi"}},
		{"upgrade", []product{oldProduct()}, []string{"/x " + testCode, "/i recovery/payload.msi"}},
		{"identical", []product{{Code: testCode, Version: "0.9.36", SID: "user", Context: 2}}, []string{"/x " + testCode, "/i recovery/payload.msi"}},
		{"multiple", []product{oldProduct(), {Code: testCode2, Version: "0.9.34", SID: "user", Context: 2}}, []string{"/x " + testCode, "/x " + testCode2, "/i recovery/payload.msi"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := newFake()
			p.products = test.products
			out := runSetup(p)
			if !out.Success {
				t.Fatalf("setup failed: %+v", out)
			}
			if got := transactions(p.events); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("transactions %v; want %v", got, test.want)
			}
			// Fresh install also verifies absence and rechecks processes.
			if len(test.products) == 0 && !reflect.DeepEqual(p.events, []string{"prepare", "prerequisite", "discover", "confirm", "running", "discover", "running", "discover", "/i recovery/payload.msi", "discover"}) {
				t.Fatalf("unexpected ordering %v", p.events)
			}
		})
	}
}

func TestPreflightNeverRemovesOnFailure(t *testing.T) {
	for _, name := range []string{"downgrade", "invalid-version", "machine", "managed", "other-user", "duplicate", "invalid-code", "decline", "prerequisite", "discovery", "payload", "process-error", "running-cancel"} {
		t.Run(name, func(t *testing.T) {
			p := newFake()
			item := oldProduct()
			switch name {
			case "downgrade":
				item.Version = "0.10.0"
			case "invalid-version":
				item.Version = "garbage"
			case "machine":
				item.Context = 4
			case "managed":
				item.Context = 1
			case "other-user":
				item.SID = "someone-else"
			case "invalid-code":
				item.Code = "not-a-guid"
			case "decline":
				p.confirm = false
			case "prerequisite":
				p.prerequisiteErr = errors.New("missing prerequisite")
			case "discovery":
				p.discoverErr = errors.New("discovery failed")
			case "payload":
				p.prepareErr = errors.New("invalid payload")
			case "process-error":
				p.runningErr = errors.New("cannot inspect")
			case "running-cancel":
				p.running = []bool{true}
			}
			p.products = []product{item}
			if name == "duplicate" {
				p.products = append(p.products, item)
			}
			out := runSetup(p)
			if out.Success || out.Message == "" {
				t.Fatalf("unexpected outcome %+v", out)
			}
			if got := transactions(p.events); len(got) != 0 {
				t.Fatalf("destructive calls %v", got)
			}
		})
	}
}

func TestUninstallStopsBeforeInstall(t *testing.T) {
	for _, status := range []uint32{1602, 1603, 1618, 3010, 1641} {
		p := newFake()
		p.products = []product{oldProduct()}
		p.statuses = []uint32{status}
		out := runSetup(p)
		if out.Success || out.Status != status || len(transactions(p.events)) != 1 {
			t.Fatalf("status %d outcome %+v events %v", status, out, p.events)
		}
	}
	p := newFake()
	p.products = []product{oldProduct()}
	p.executeErr = errors.New("launch failure")
	if out := runSetup(p); out.Success || len(transactions(p.events)) != 1 {
		t.Fatalf("unexpected outcome %+v", out)
	}
}

func TestAlreadyRemovedRequiresVerification(t *testing.T) {
	for _, status := range []uint32{0, 1605, 1614} {
		for _, remains := range []bool{true, false} {
			p := newFake()
			p.products = []product{oldProduct()}
			p.statuses = []uint32{status}
			p.keepProduct = remains
			out := runSetup(p)
			if out.Success == remains {
				t.Fatalf("status %d remains %v outcome %+v", status, remains, out)
			}
		}
	}
}

func TestAppCloseRetryAndFinalRecheck(t *testing.T) {
	p := newFake()
	p.products = []product{oldProduct()}
	p.running = []bool{true, false, true, false}
	p.retry = true
	if out := runSetup(p); !out.Success {
		t.Fatalf("retry failed %+v", out)
	}
	p = newFake()
	p.products = []product{oldProduct()}
	p.running = []bool{false, true}
	if out := runSetup(p); out.Success || out.Status != 1602 || len(transactions(p.events)) != 1 {
		t.Fatalf("final recheck failed %+v %v", out, p.events)
	}
}

func TestNewRegistrationAbortsInstall(t *testing.T) {
	p := newFake()
	p.appearAt = 2
	if out := runSetup(p); out.Success || len(transactions(p.events)) != 0 {
		t.Fatalf("unexpected outcome %+v", out)
	}
}

func TestInstallFailurePreservesRecovery(t *testing.T) {
	for _, status := range []uint32{1602, 1603, 1618} {
		p := newFake()
		p.products = []product{oldProduct()}
		p.statuses = []uint32{0, status}
		out := runSetup(p)
		if out.Success || out.Status != status || out.Operation != "install" || out.Payload.Path != "recovery/payload.msi" {
			t.Fatalf("unexpected outcome %+v", out)
		}
	}
}

func TestVersionsAndGUIDs(t *testing.T) {
	for _, bad := range []string{"", "1.2", "1.2.3.4", "256.0.0", "0.256.0", "0.0.65536", "-1.0.0", "1.2.x", "1.2.3-beta", "1. 2.3"} {
		if _, err := parseVersion(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	v, err := parseVersion("255.255.65535")
	if err != nil || v != (msiVersion{255, 255, 65535}) {
		t.Fatal(v, err)
	}
	if !validGUID(upgradeCode) || validGUID("{wrong}") {
		t.Fatal("GUID validation failed")
	}
}

func TestInstallerArguments(t *testing.T) {
	want := []string{"/i", `C:\recovery dir\payload.msi`, "/passive", "/norestart", "/L*v", `C:\recovery dir\install.log`, "REBOOT=ReallySuppress", "MSIRESTARTMANAGERCONTROL=DisableShutdown", "MSIDISABLERMRESTART=1"}
	if got := installerArgs("/i", want[1], want[5]); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}

func TestChangesAfterConfirmationAbortRemoval(t *testing.T) {
	p := newFake()
	p.products = []product{oldProduct()}
	p.mutateAt = 2
	out := runSetup(p)
	if out.Success || len(transactions(p.events)) != 0 {
		t.Fatalf("removed changed product: %+v %v", out, p.events)
	}
}
func TestRegistrationAppearingAfterFinalCloseAbortsInstall(t *testing.T) {
	p := newFake()
	p.appearAt = 3
	out := runSetup(p)
	if out.Success || len(transactions(p.events)) != 0 {
		t.Fatalf("installed over changed product: %+v %v", out, p.events)
	}
}
func TestInstallationRestartAndIdentity(t *testing.T) {
	for _, status := range []uint32{3010, 1641} {
		p := newFake()
		p.statuses = []uint32{status}
		out := runSetup(p)
		if !out.Success || out.Status != status {
			t.Fatalf("restart outcome %+v", out)
		}
	}
	p := newFake()
	p.wrongInstall = true
	if out := runSetup(p); out.Success {
		t.Fatalf("accepted wrong installed product: %+v", out)
	}
}

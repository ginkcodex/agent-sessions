package codediagnostics

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRustLiveUnpinnedWithoutAffectingCargo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rust live discovery requires macOS/Linux")
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='hello'\nversion='0.1.0'\n")
	f.put(t, "Cargo.lock", "version=3\n")
	f.put(t, "src/main.rs", "fn main() {}\n")
	p := f.discover(t)
	r := rustLiveRegistration(p)
	if r.Availability != "unsupported" || len(r.Live.Workspaces) != 0 || r.Live.Server.Executable != "" || !strings.Contains(r.Live.State.Notice, "pinned managed") {
		t.Fatalf("Rust must not propose or launch an unverified server: %+v", r)
	}
	if p.Availability("cargo-check") != "available" {
		t.Fatal("live policy block disabled Cargo")
	}
	checker, err := NewCargoCheckPlan(f.root, p)
	if err != nil || len(checker.Tools()) == 0 {
		t.Fatalf("saved checker unavailable: %v", err)
	}
}

func TestRustLiveBlockedStateInBothDiscoveryRoutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local live discovery requires macOS/Linux")
	}
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "nested"}[nested], func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='hello'\nversion='0.1.0'\n")
			f.put(t, "src/main.rs", "fn main() {}\n")
			if nested {
				f.put(t, "web/package.json", "{}\n")
				f.put(t, "web/src/a.ts", "export const a = 1;\n")
			}
			regs, err := DiscoverLocalLive(f.root, LocalLiveEnvironment{Path: []string{f.bin}, Rust: f.env, Go: GoplsEnvironment{Home: f.env.Home, Cache: filepath.Join(f.env.Home, "cache"), ModuleCache: filepath.Join(f.env.Home, "modules")}, TS: TSEnvironment{Home: f.env.Home}, Ruff: RuffEnvironment{Home: f.env.Home}, Pyright: PyrightEnvironment{Home: f.env.Home}})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range regs {
				if r.ProviderID == rustAnalyzerProviderID {
					found = true
					if r.Availability != "unsupported" || r.Live.State.Notice != rustLiveUnpinnedNotice {
						t.Fatal(r.Live.State)
					}
				}
			}
			if !found {
				t.Fatal("missing Rust status")
			}
		})
	}
}

func TestNonRustCoverageRootsSerializeAsArray(t *testing.T) {
	f := newRustFixture(t) // No Cargo.toml: the Rust status is still registered.
	plan := f.discover(t)
	registration := rustLiveRegistration(plan)
	if registration.Availability != "unsupported" {
		t.Fatal(registration.Availability)
	}
	assertArray := func(name string, roots []string) {
		t.Helper()
		wire, err := json.Marshal(roots)
		if err != nil || string(wire) != "[]" {
			t.Fatalf("%s roots must encode as [], got %s: %v", name, wire, err)
		}
	}
	assertArray("discovery", plan.Roots())
	assertArray("registration", registration.Live.State.Coverage.Roots)
	// The manager also protects clients from another registration with nil roots.
	registration.Live.State.Coverage.Roots = nil
	trust := NewTrust(nil)
	m, err := NewManager("empty-rust-roots", trust, NewSupervisor(trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	snapshot, err := m.Attach(f.root, 1, []Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	assertArray("snapshot", snapshot.Providers[0].Coverage.Roots)
}

func TestSavedProviderNoticePropagation(t *testing.T) {
	f := newRustFixture(t)
	trust := NewTrust(nil)
	m, err := NewManager("rust-notice", trust, NewSupervisor(trust, SupervisorOptions{}), ManagerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	snapshot, err := m.Attach(f.root, 1, []Registration{{ProviderID: "cargo-check", Availability: "unsupported", Notice: "Prepare Cargo.lock"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Providers) != 1 || snapshot.Providers[0].Notice != "Prepare Cargo.lock" {
		t.Fatal(snapshot.Providers)
	}
}

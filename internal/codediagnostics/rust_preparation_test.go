package codediagnostics

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func rustPreparationFixture(t *testing.T) rustFixture {
	t.Helper()
	f := newRustFixture(t)
	f.env.PrepareDir = filepath.Join(filepath.Dir(f.env.Home), "app-data", "rust-preparation")
	if err := os.MkdirAll(f.env.PrepareDir, 0700); err != nil {
		t.Fatal(err)
	}
	f.put(t, "Cargo.toml", "[package]\nname='app'\nversion='0.1.0'\n[workspace]\n")
	f.put(t, "Cargo.lock", "version=3\n")
	f.put(t, "src/lib.rs", "pub fn app() {}\n")
	rustTestWrite(t, filepath.Join(f.bin, rustExeName("rustup")), "fake rustup metadata only; never execute", 0755)
	return f
}

func TestRustPreparationMissingSelectedToolchain(t *testing.T) {
	f := rustPreparationFixture(t)
	f.put(t, "rust-toolchain.toml", "[toolchain]\nchannel='stable'\ncomponents=['clippy','rust-src']\ntargets=['wasm32-unknown-unknown']\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "missingTool" {
		t.Fatal(p.Notice())
	}
	prep := p.Preparation()
	if prep == nil || len(prep.Steps) != 4 {
		t.Fatalf("preparation %+v: %s", prep, p.Notice())
	}
	want := [][]string{
		{"toolchain", "install", "stable", "--profile", "minimal", "--no-self-update"},
		{"component", "add", "--toolchain", "stable", "clippy", "rust-src"},
		{"target", "add", "--toolchain", "stable", "wasm32-unknown-unknown"},
		{"run", "stable", "cargo", "fetch", "--locked", "--manifest-path", filepath.Join(f.root.Project().Root, "Cargo.toml")},
	}
	for i, step := range prep.Steps {
		if !slices.Equal(step.Command.Args, want[i]) || step.Command.Executable != filepath.Join(f.bin, rustExeName("rustup")) || step.Command.PreparationDirectory != f.env.PrepareDir {
			t.Fatalf("step %d: %+v", i, step)
		}
		env := strings.Join(step.Command.Env, "\n")
		for _, name := range []string{"RUSTUP_AUTO_INSTALL=0", "CARGO_NET_OFFLINE=false", "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER="} {
			if !strings.Contains(env, name) {
				t.Fatal("missing preparation environment", name)
			}
		}
		if step.Command.ValidateStart == nil {
			t.Fatal("step does not revalidate inputs")
		}
	}
	if err := validatePreparation(f.root, prep); err != nil {
		t.Fatal(err)
	}
	if err := prep.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := Fingerprint(f.root, prep.Tools); err != nil {
		t.Fatal(err)
	}
	// Simulate only expected rustup outputs: no Rust tool is ever executed.
	toolDir := filepath.Join(f.env.Home, ".rustup", "toolchains", "stable-"+rustHostTriple())
	for _, name := range []string{"cargo", "rustc", "rustdoc"} {
		rustTestWrite(t, filepath.Join(toolDir, "bin", rustExeName(name)), "new concrete "+name, 0755)
	}
	rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "default_toolchain='stable'\nversion='12'\n", 0644)
	if err := prep.Prepared(nil); err != nil {
		t.Fatal(err)
	}
	// A newly installed unrelated executable cannot borrow approval.
	rustTestWrite(t, filepath.Join(toolDir, "bin", rustExeName("rust-analyzer")), "unplanned analyzer", 0755)
	if err := os.Remove(filepath.Join(f.bin, rustExeName("rust-analyzer"))); err != nil {
		t.Fatal(err)
	}
	if err := prep.Prepared(nil); err == nil {
		t.Fatal("unplanned analyzer accepted")
	}
}

func TestRustPreparationRejectsChangedInputs(t *testing.T) {
	for _, change := range []string{"source", "new source", "config", "new host config", "tool", "settings"} {
		t.Run(change, func(t *testing.T) {
			f := rustPreparationFixture(t)
			if change == "settings" {
				rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "version='12'\n", 0644)
			}
			p := f.discover(t)
			prep := p.Preparation()
			if prep == nil {
				t.Fatal(p.Notice())
			}
			switch change {
			case "source":
				f.put(t, "src/lib.rs", "pub fn changed() {}\n")
			case "new source":
				f.put(t, "src/new.rs", "pub fn new_file() {}\n")
			case "config":
				f.put(t, "Cargo.lock", "version=4\n")
			case "new host config":
				rustTestWrite(t, filepath.Join(f.env.Home, ".cargo", "config.toml"), "[net]\noffline=true\n", 0644)
			case "tool":
				rustTestWrite(t, filepath.Join(f.bin, rustExeName("cargo")), "changed cargo", 0755)
			case "settings":
				rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "version='13'\n", 0644)
			}
			if err := prep.Validate(); err == nil {
				t.Fatal("changed input accepted")
			}
		})
	}
}

func TestRustPreparationWorkingDirectoryCargoConfiguration(t *testing.T) {
	for _, location := range []string{"cwd", "ancestor"} {
		for _, name := range []string{"config", "config.toml"} {
			t.Run(location+"/"+name, func(t *testing.T) {
				f := rustPreparationFixture(t)
				dir := f.env.PrepareDir
				if location == "ancestor" {
					dir = filepath.Dir(dir)
				}
				path := filepath.Join(dir, ".cargo", name)
				p := f.discover(t)
				prep := p.Preparation()
				if prep == nil {
					t.Fatal(p.Notice())
				}
				rustTestWrite(t, path, "[net]\noffline=true\n", 0644)
				if p.Unchanged() == nil || prep.Validate() == nil {
					t.Fatal("new preparation cwd Cargo configuration not detected")
				}
				p = f.discover(t)
				prep = p.Preparation()
				if prep == nil || !slices.Contains(p.hostInputs, path) {
					t.Fatal("safe preparation cwd Cargo configuration not bound", p.Notice())
				}
				if err := prep.Validate(); err != nil {
					t.Fatal("unchanged preparation cwd config rejected", err)
				}
				if err := p.Unchanged(); err != nil {
					t.Fatal(err)
				}
				rustTestWrite(t, path, "[net]\noffline=true\n# changed bytes\n", 0644)
				if p.Unchanged() == nil || prep.Validate() == nil {
					t.Fatal("changed preparation cwd config bytes not detected")
				}
				p = f.discover(t)
				prep = p.Preparation()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if p.Unchanged() == nil || prep.Validate() == nil {
					t.Fatal("removed preparation cwd config not detected")
				}
				rustTestWrite(t, path, "[build]\nrustc-wrapper='evil'\n", 0644)
				p = f.discover(t)
				if p.Availability("cargo-check") != "unsupported" || p.Preparation() != nil || p.Notice() == "" {
					t.Fatal("unsafe preparation cwd Cargo configuration accepted")
				}
			})
		}
	}
	t.Run("shared ancestors deduplicated", func(t *testing.T) {
		f := rustPreparationFixture(t)
		path := filepath.Join(filepath.Dir(f.env.Home), ".cargo", "config.toml")
		rustTestWrite(t, path, "[net]\noffline=true\n", 0644)
		p := f.discover(t)
		prep := p.Preparation()
		if prep == nil {
			t.Fatal(p.Notice())
		}
		if err := prep.Validate(); err != nil {
			t.Fatal("overlapping ancestor walks changed enumeration", err)
		}
	})
}

func TestRustPreparationHostAnalyzerConfiguration(t *testing.T) {
	for _, change := range []string{"created", "changed", "removed"} {
		t.Run(change, func(t *testing.T) {
			f := rustPreparationFixture(t)
			path := rustTestAnalyzerConfig(t, f.env.Home)
			if change != "created" {
				rustTestWrite(t, path, "# reviewed empty\n", 0644)
			}
			p := f.discover(t)
			prep := p.Preparation()
			if prep == nil {
				t.Fatal(p.Notice())
			}
			if err := prep.Validate(); err != nil {
				t.Fatal(err)
			}
			if change == "removed" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				rustTestWrite(t, path, "# new empty\n", 0644)
			}
			if prep.Validate() == nil {
				t.Fatal("changed host analyzer config accepted during preparation")
			}
		})
	}
}

func TestRustPreparationLockAndHostBarriers(t *testing.T) {
	for _, which := range []string{"missing lock", "no selection", "cargo overlap", "rustup overlap", "prepare overlap", "cargo symlink", "missing rustup"} {
		t.Run(which, func(t *testing.T) {
			f := rustPreparationFixture(t)
			switch which {
			case "missing lock":
				if err := os.Remove(filepath.Join(f.root.Project().Root, "Cargo.lock")); err != nil {
					t.Fatal(err)
				}
			case "no selection":
				for _, name := range []string{"cargo", "rustc"} {
					if err := os.Remove(filepath.Join(f.bin, rustExeName(name))); err != nil {
						t.Fatal(err)
					}
				}
			case "cargo overlap":
				f.env.CargoHome = filepath.Join(f.root.Project().Root, ".cargo")
			case "rustup overlap":
				f.env.RustupHome = filepath.Join(f.root.Project().Root, ".rustup")
			case "prepare overlap":
				f.env.PrepareDir = f.root.Project().Root
			case "cargo symlink":
				link := filepath.Join(filepath.Dir(f.env.Home), "cargo-link")
				if err := os.Symlink(f.env.Home, link); err != nil {
					t.Skip(err)
				}
				f.env.CargoHome = link
			case "missing rustup":
				f.put(t, "rust-toolchain.toml", "[toolchain]\nchannel='stable'\n")
				if err := os.Remove(filepath.Join(f.bin, rustExeName("rustup"))); err != nil {
					t.Fatal(err)
				}
			}
			p := f.discover(t)
			if p.Preparation() != nil {
				t.Fatal("unsafe preparation proposed", which)
			}
			if p.Notice() == "" {
				t.Fatal("missing actionable setup guidance")
			}
		})
	}
}

func TestRustPreparationMissingWorkingDirectory(t *testing.T) {
	f := rustPreparationFixture(t)
	if err := os.RemoveAll(f.env.PrepareDir); err != nil {
		t.Fatal(err)
	}
	p := f.discover(t)
	if p.Preparation() == nil {
		t.Fatal("absent app-owned cwd should not block metadata-only preparation")
	}
	// A regular file at the cwd is not a reviewed working directory.
	f = rustPreparationFixture(t)
	if err := os.RemoveAll(f.env.PrepareDir); err != nil {
		t.Fatal(err)
	}
	rustTestWrite(t, f.env.PrepareDir, "not a directory", 0644)
	p = f.discover(t)
	if p.Preparation() != nil {
		t.Fatal("non-directory cwd accepted")
	}
}

func TestRustPreparationBindsRediscoveredRegistrations(t *testing.T) {
	f := rustPreparationFixture(t)
	p := f.discover(t)
	prep := p.Preparation()
	next := f.discover(t)
	cargo, err := NewCargoCheckPlan(next.root, next)
	if err != nil {
		t.Fatal(err)
	}
	registrations := []Registration{{ProviderID: "cargo-check", Adapter: cargo, Prepare: next.Preparation()}}
	if err := prep.Prepared(registrations); err != nil {
		t.Fatal("valid rediscovered registration rejected", err)
	}
	cargo.tools[0].Policy += ":unplanned"
	if err := prep.Prepared(registrations); err == nil {
		t.Fatal("unplanned rediscovered command policy accepted")
	}
}

func TestRustPreparationIndependentRootsAndDeclarations(t *testing.T) {
	f := rustPreparationFixture(t)
	if err := os.Remove(filepath.Join(f.root.Project().Root, "Cargo.toml")); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"a", "b"} {
		f.put(t, root+"/Cargo.toml", "[package]\nname='"+root+"'\n[workspace]\n")
		f.put(t, root+"/Cargo.lock", "version=3\n")
	}
	f.put(t, "a/rust-toolchain.toml", "[toolchain]\nchannel='stable'\ncomponents=['clippy']\n")
	f.put(t, "b/rust-toolchain.toml", "[toolchain]\nchannel='stable'\ncomponents=['rustfmt']\ntargets=['wasm32-unknown-unknown']\n")
	p := f.discover(t)
	if p.Availability("cargo-check") == "unsupported" || !slices.Equal(p.components, []string{"clippy", "rustfmt"}) {
		t.Fatal(p.Notice(), p.components)
	}
	prep := p.Preparation()
	if prep == nil || len(prep.Steps) != 5 {
		t.Fatal(p.Notice())
	}
	for i, root := range []string{"a", "b"} {
		args := prep.Steps[3+i].Command.Args
		if args[len(args)-1] != filepath.Join(f.root.Project().Root, root, "Cargo.toml") {
			t.Fatal(args)
		}
	}
}

package codediagnostics

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type rustFixture struct {
	root AuthorizedRoot
	env  RustEnvironment
	bin  string
}

func rustTestWrite(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}
func newRustFixture(t *testing.T) rustFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(base, "project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := AuthorizeRoot("local:rust-tests", project, func(p string) bool { return p == project })
	if err != nil {
		t.Fatal(err)
	}
	f := rustFixture{root: root, env: RustEnvironment{Home: filepath.Join(base, "home")}, bin: filepath.Join(base, "tools")}
	if err := os.MkdirAll(f.env.Home, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cargo", "rustc", "rust-analyzer"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		rustTestWrite(t, filepath.Join(f.bin, name), "fake executable metadata only: "+name, 0755)
	}
	return f
}
func (f rustFixture) put(t *testing.T, path, text string) {
	t.Helper()
	rustTestWrite(t, filepath.Join(f.root.Project().Root, filepath.FromSlash(path)), text, 0644)
}
func (f rustFixture) discover(t *testing.T) *RustPlan {
	t.Helper()
	p, err := DiscoverRust(f.root, []string{f.bin}, f.env)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRustDiscoveryConcreteIndependentTools(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Native Windows Rust execution is not supported")
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='hello'\nversion='0.1.0'\n")
	f.put(t, "Cargo.lock", "version=3\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "available" || p.Availability("rust-analyzer") != "available" {
		t.Fatalf("unexpected availability: %s", p.Notice())
	}
	if !slices.Equal(p.Roots(), []string{""}) {
		t.Fatalf("roots %q", p.Roots())
	}
	if err := p.Unchanged(); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(p.Environment(), "\n")
	for _, s := range []string{"CARGO_NET_OFFLINE=true", "RUSTUP_AUTO_INSTALL=0", "RUSTC_WRAPPER=", "RUSTC=" + filepath.Join(f.bin, "rustc")} {
		if !strings.Contains(env, s) {
			t.Fatalf("missing %s", s)
		}
	}
	if err := os.Remove(filepath.Join(f.bin, "rust-analyzer")); err != nil {
		t.Fatal(err)
	}
	q := f.discover(t)
	if q.Availability("cargo-check") != "available" || q.Availability("rust-analyzer") != "missingTool" {
		t.Fatal("server availability is not independent")
	}
	if p.Unchanged() == nil {
		t.Fatal("missing server did not invalidate identity")
	}
}
func TestRustDiscoveryWorkspaceGlobsExclusionsAndNested(t *testing.T) {
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[workspace]\nmembers=['crates/*']\nexclude=['crates/excluded','independent']\n")
	for _, name := range []string{"a", "b", "excluded"} {
		f.put(t, "crates/"+name+"/Cargo.toml", "[package]\nname='"+name+"'\nversion='0.1.0'\n")
	}
	f.put(t, "independent/Cargo.toml", "[package]\nname='nested'\nversion='0.1.0'\n[workspace]\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "available" {
		t.Fatal(p.Notice())
	}
	if !slices.Equal(p.Roots(), []string{"", "crates/excluded", "independent"}) {
		t.Fatalf("roots %q", p.Roots())
	}
}
func TestRustDiscoveryRejectsExternalAndUnreviewedConfigurations(t *testing.T) {
	cases := map[string]map[string]string{
		"external dependency": {"Cargo.toml": "[package]\nname='p'\nversion='0.1.0'\n[dependencies]\nx={path='../outside'}\n"},
		"workspace parent":    {"Cargo.toml": "[package]\nname='p'\nworkspace='..'\n"},
		"external member":     {"Cargo.toml": "[workspace]\nmembers=['../outside']\n"},
		"unsupported glob":    {"Cargo.toml": "[workspace]\nmembers=['**/x']\n"},
		"invalid TOML":        {"Cargo.toml": "[package\n"},
		"wrapper":             {"Cargo.toml": "[package]\nname='p'\n", ".cargo/config.toml": "[build]\nrustc-wrapper='evil'\n"},
		"source replacement":  {"Cargo.toml": "[package]\nname='p'\n", ".cargo/config": "[source.crates-io]\nreplace-with='evil'\n"},
		"unowned nested":      {"Cargo.toml": "[workspace]\n", "nested/Cargo.toml": "[package]\nname='nested'\n"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRustFixture(t)
			for path, text := range files {
				f.put(t, path, text)
			}
			p := f.discover(t)
			if p.Availability("cargo-check") != "unsupported" {
				t.Fatalf("unsafe configuration accepted: %s", p.Notice())
			}
			if _, ok := p.Cargo(); ok {
				t.Fatal("blocked plan exposes available Cargo")
			}
		})
	}
}
func TestRustDiscoveryContainedPathDependencyAndWorkspacePointer(t *testing.T) {
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[workspace]\nmembers=['app']\n")
	f.put(t, "app/Cargo.toml", "[package]\nname='app'\nworkspace='..'\n[dependencies]\nlib={path='../lib'}\n")
	f.put(t, "lib/Cargo.toml", "[package]\nname='lib'\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "available" {
		t.Fatal(p.Notice())
	}
	if !slices.Equal(p.Roots(), []string{""}) {
		t.Fatal(p.Roots())
	}
}
func TestRustDiscoveryInputsCreatedRemovedAndChanged(t *testing.T) {
	for _, path := range []string{"Cargo.lock", ".cargo/config.toml", "rust-toolchain.toml", "new/Cargo.toml"} {
		t.Run(path, func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='app'\n[workspace]\n")
			p := f.discover(t)
			text := ""
			switch path {
			case "Cargo.lock":
				text = "version=3\n"
			case ".cargo/config.toml":
				text = "[net]\noffline=true\n"
			case "rust-toolchain.toml":
				text = "[toolchain]\nchannel='stable'\n"
			case "new/Cargo.toml":
				text = "[package]\nname='new'\n[workspace]\n"
			}
			f.put(t, path, text)
			if p.Unchanged() == nil {
				t.Fatal("created input not detected")
			}
			q := f.discover(t)
			if err := os.Remove(filepath.Join(f.root.Project().Root, filepath.FromSlash(path))); err != nil {
				t.Fatal(err)
			}
			if q.Unchanged() == nil {
				t.Fatal("removed input not detected")
			}
		})
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='app'\n")
	p := f.discover(t)
	rustTestWrite(t, filepath.Join(f.env.Home, ".cargo", "config.toml"), "[net]\noffline=true\n", 0644)
	if p.Unchanged() == nil {
		t.Fatal("host config creation not detected")
	}
}
func TestRustDiscoveryRustupProxyAndConcreteSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses Unix executable semantics")
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='app'\n")
	rustTestWrite(t, filepath.Join(f.bin, "rustup"), "fake rustup must never execute", 0755)
	for _, name := range []string{"cargo", "rustc", "rust-analyzer"} {
		if err := os.Remove(filepath.Join(f.bin, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(filepath.Join(f.bin, "rustup"), filepath.Join(f.bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	p := f.discover(t)
	if p.Availability("cargo-check") != "missingTool" {
		t.Fatal("proxy accepted")
	}
	f.put(t, "rust-toolchain.toml", "[toolchain]\nchannel='stable'\n")
	selected := "stable-" + rustHostTriple()
	dir := filepath.Join(f.env.Home, ".rustup", "toolchains", selected, "bin")
	for _, name := range []string{"cargo", "rustc"} {
		rustTestWrite(t, filepath.Join(dir, name), "concrete "+name, 0755)
	}
	q := f.discover(t)
	if q.Availability("cargo-check") != "available" || q.Availability("rust-analyzer") != "missingTool" {
		t.Fatalf("selection unavailable: %s", q.Notice())
	}
	cargo, _ := q.Cargo()
	if cargo != filepath.Join(dir, "cargo") {
		t.Fatal(cargo)
	}
	rustTestWrite(t, filepath.Join(dir, "rust-analyzer"), "concrete server", 0755)
	if q.Unchanged() == nil {
		t.Fatal("new component did not invalidate discovery")
	}
	q = f.discover(t)
	if q.Availability("rust-analyzer") != "available" {
		t.Fatal(q.Notice())
	}
	rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "default_toolchain='stable'\n", 0644)
	if q.Unchanged() == nil {
		t.Fatal("settings creation did not invalidate")
	}
}
func TestRustDiscoveryAncestorWorkspaceDoesNotExpandAuthority(t *testing.T) {
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='app'\n")
	rustTestWrite(t, filepath.Join(filepath.Dir(f.root.Project().Root), "Cargo.toml"), "[workspace]\nmembers=['project']\n", 0644)
	p := f.discover(t)
	if p.Availability("cargo-check") != "unsupported" {
		t.Fatal("ancestor workspace accepted")
	}
	f.put(t, "Cargo.toml", "[package]\nname='app'\n[workspace]\n")
	p = f.discover(t)
	if p.Availability("cargo-check") != "available" {
		t.Fatal(p.Notice())
	}
}
func TestRustDiscoveryLimitsAndSymlinks(t *testing.T) {
	t.Run("manifests", func(t *testing.T) {
		f := newRustFixture(t)
		for i := 0; i <= maxRustManifests; i++ {
			name := strings.Repeat("a", i+1)
			f.put(t, name+"/Cargo.toml", "[package]\nname='p'\n[workspace]\n")
		}
		p := f.discover(t)
		if p.Availability("cargo-check") != "unsupported" {
			t.Fatal("manifest bound not enforced")
		}
	})
	t.Run("symlink", func(t *testing.T) {
		f := newRustFixture(t)
		f.put(t, "Cargo.toml", "[package]\nname='app'\n")
		if err := os.Symlink(t.TempDir(), filepath.Join(f.root.Project().Root, "external")); err != nil {
			t.Skip(err)
		}
		p := f.discover(t)
		if p.Availability("cargo-check") != "unsupported" {
			t.Fatal("symlink accepted")
		}
	})
}

func TestRustDiscoveryToolchainHelperIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable fixture")
	}
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='app'\n")
	p := f.discover(t)
	helper := filepath.Join(filepath.Dir(f.bin), "libexec", "rust-analyzer-proc-macro-srv")
	rustTestWrite(t, helper, "concrete proc macro helper", 0755)
	if p.Unchanged() == nil {
		t.Fatal("new helper did not invalidate approval")
	}
	p = f.discover(t)
	if p.ProcMacroServer() != helper {
		t.Fatalf("helper %q", p.ProcMacroServer())
	}
	rustTestWrite(t, helper, "changed proc macro helper", 0755)
	if p.Unchanged() == nil {
		t.Fatal("changed helper did not invalidate approval")
	}
	rustTestWrite(t, filepath.Join(f.bin, "rustdoc"), "concrete rustdoc", 0755)
	p = f.discover(t)
	if p.Rustdoc() == "" {
		t.Fatal("missing rustdoc")
	}
	if !slices.Contains(p.Environment(), "RUSTDOC="+p.Rustdoc()) {
		t.Fatal("rustdoc is not pinned")
	}
}
func TestRustDiscoverySkippedDependencyCannotEscapeReview(t *testing.T) {
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='app'\n[dependencies]\nx={path='vendor/x'}\n")
	f.put(t, "vendor/x/Cargo.toml", "[package]\nname='x'\n[dependencies]\nescape={path='../../../../escape'}\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "unsupported" {
		t.Fatal("skipped path dependency accepted")
	}
}
func TestRustDiscoveryIndependentRootToolchainSelections(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		available  bool
	}{
		{"both-absent", "", "", true},
		{"both-identical", "stable", "stable", true},
		{"both-different", "stable", "nightly", false},
		{"first-absent", "", "stable", false},
		{"second-absent", "stable", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRustFixture(t)
			for _, dir := range []string{"a", "b"} {
				f.put(t, dir+"/Cargo.toml", "[package]\nname='"+dir+"'\n[workspace]\n")
			}
			for dir, channel := range map[string]string{"a": tc.a, "b": tc.b} {
				if channel != "" {
					f.put(t, dir+"/rust-toolchain.toml", "[toolchain]\nchannel='"+channel+"'\n")
				}
			}
			// The absent root's host default differs from an explicit stable
			// selector. Never let the explicit root silently override it.
			rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "default_toolchain='nightly'\n", 0644)
			for _, channel := range []string{"stable", "nightly"} {
				for _, name := range []string{"cargo", "rustc"} {
					if runtime.GOOS == "windows" {
						name += ".exe"
					}
					rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "toolchains", channel+"-"+rustHostTriple(), "bin", name), "concrete tool metadata only", 0755)
				}
			}
			p := f.discover(t)
			want := "unsupported"
			if tc.available {
				want = "available"
			}
			if got := p.Availability("cargo-check"); got != want {
				t.Fatalf("availability %s, want %s: %s", got, want, p.Notice())
			}
		})
	}
}

func TestRustDiscoveryBuildScriptInSkippedDirectory(t *testing.T) {
	for _, mode := range []string{"regular", "external-symlink", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='app'\nbuild='vendor/build.rs'\n")
			switch mode {
			case "regular":
				f.put(t, "vendor/build.rs", "fn main() {}\n")
			case "external-symlink":
				outside := filepath.Join(t.TempDir(), "build.rs")
				rustTestWrite(t, outside, "fn main() {}\n", 0644)
				if err := os.MkdirAll(filepath.Join(f.root.Project().Root, "vendor"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(f.root.Project().Root, "vendor", "build.rs")); err != nil {
					t.Skip(err)
				}
			}
			p := f.discover(t)
			want := "unsupported"
			if mode == "regular" {
				want = "available"
			}
			if p.Availability("cargo-check") != want {
				t.Fatalf("availability %s, want %s: %s", p.Availability("cargo-check"), want, p.Notice())
			}
		})
	}
}

func TestRustDiscoveryHostPolicies(t *testing.T) {
	for _, which := range []string{"config", "override", "unbounded-component", "rust-analyzer-config"} {
		t.Run(which, func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='p'\n")
			switch which {
			case "config":
				rustTestWrite(t, filepath.Join(f.env.Home, ".cargo", "config.toml"), "[env]\nUNREVIEWED='1'\n", 0644)
			case "override":
				root := strings.ReplaceAll(f.root.Project().Root, `\`, `\\`)
				rustTestWrite(t, filepath.Join(f.env.Home, ".rustup", "settings.toml"), "[overrides]\n'"+root+"'='stable'\n", 0644)
			case "unbounded-component":
				f.put(t, "rust-toolchain.toml", "[toolchain]\nchannel='stable'\ncomponents=['../evil']\n")
			case "rust-analyzer-config":
				f.put(t, "rust-analyzer.toml", "[cargo]\nextraEnv={X='1'}\n")
			}
			if p := f.discover(t); p.Availability("cargo-check") != "unsupported" {
				t.Fatalf("host policy accepted: %s", p.Notice())
			}
		})
	}
}

func rustTestAnalyzerConfig(t *testing.T, home string) string {
	t.Helper()
	switch runtime.GOOS {
	case "linux":
		return filepath.Join(home, ".config", "rust-analyzer", "rust-analyzer.toml")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "rust-analyzer", "rust-analyzer.toml")
	default:
		t.Skip("native Windows Rust live analysis is unsupported")
		return ""
	}
}

func TestRustDiscoveryHostAnalyzerConfiguration(t *testing.T) {
	for _, content := range []string{"[cargo]\nextraEnv={X='1'}\n", "[invalid", strings.Repeat("#", (1<<20)+1)} {
		t.Run("reject", func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='app'\n")
			rustTestWrite(t, rustTestAnalyzerConfig(t, f.env.Home), content, 0644)
			p := f.discover(t)
			if p.Availability("rust-analyzer") != "unsupported" || p.Notice() == "" {
				t.Fatal("unreviewed user analyzer configuration accepted")
			}
		})
	}
	for _, change := range []string{"created", "changed", "removed"} {
		t.Run(change, func(t *testing.T) {
			f := newRustFixture(t)
			f.put(t, "Cargo.toml", "[package]\nname='app'\n")
			path := rustTestAnalyzerConfig(t, f.env.Home)
			if change != "created" {
				rustTestWrite(t, path, "# reviewed empty configuration\n", 0644)
			}
			p := f.discover(t)
			if p.Availability("rust-analyzer") != "available" || p.Unchanged() != nil {
				t.Fatal("empty user analyzer configuration rejected", p.Notice())
			}
			if change != "created" && !slices.Contains(p.hostInputs, path) {
				t.Fatal("user analyzer configuration bytes not bound")
			}
			if change == "removed" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				rustTestWrite(t, path, "# new empty configuration\n", 0644)
			}
			if p.Unchanged() == nil {
				t.Fatal("user analyzer configuration enumeration or bytes changed without invalidation")
			}
		})
	}
	t.Run("symlink", func(t *testing.T) {
		f := newRustFixture(t)
		f.put(t, "Cargo.toml", "[package]\nname='app'\n")
		path := rustTestAnalyzerConfig(t, f.env.Home)
		outside := filepath.Join(filepath.Dir(f.env.Home), "outside-analyzer.toml")
		rustTestWrite(t, outside, "# empty\n", 0644)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Skip(err)
		}
		if p := f.discover(t); p.Availability("rust-analyzer") != "unsupported" {
			t.Fatal("symlinked user analyzer configuration accepted")
		}
	})
}

func TestRustDiscoveryOpaqueMetadataPaths(t *testing.T) {
	f := newRustFixture(t)
	f.put(t, "Cargo.toml", "[package]\nname='opaque'\nversion='0.1.0'\n[package.metadata.docs]\npath='/not-a-cargo-path'\n[workspace]\n[workspace.metadata.notes]\nbuild='../not-a-script'\n")
	p := f.discover(t)
	if p.Availability("cargo-check") != "available" {
		t.Fatal(p.Notice())
	}
}

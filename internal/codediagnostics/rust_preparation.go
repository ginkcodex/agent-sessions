package codediagnostics

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const rustPreparationPolicy = "rust-prepare-v1:explicit-toolchain:minimal:locked-fetch:no-inherited-wrappers:app-owned-cwd"

// rustNoLinks checks every existing path component, including ancestors of an
// absent installation destination. Resolving only the final file is not enough:
// a symlinked RUSTUP_HOME would grant an install outside the reviewed home.
func rustNoLinks(path string) error {
	if !filepath.IsAbs(path) || !bounded(path, 4096) || hasParent(path) || filepath.Clean(path) != path {
		return failure("unsupported", "Rust host paths must be clean absolute paths")
	}
	for cur := path; ; cur = filepath.Dir(cur) {
		st, err := os.Lstat(cur)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return failure("unsupported", "Rust host path cannot be reviewed")
		}
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return failure("unsupported", "Rust host paths must not contain symlinks")
		}
		if filepath.Dir(cur) == cur {
			return nil
		}
	}
}

func rustRegularFile(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&os.ModeSymlink == 0
}

func rustPathsOverlap(a, b string) bool {
	within := func(parent, child string) bool {
		rel, err := filepath.Rel(parent, child)
		return err == nil && (rel == "." || filepath.IsLocal(rel))
	}
	return within(a, b) || within(b, a)
}

func rustExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Preparation is metadata only. It never bootstraps rustup, probes tools, creates
// directories, or fetches dependencies. Every executable exists and is hashed at
// proposal time; a missing Cargo is invoked through the existing bound rustup.
// Missing lockfiles are deliberately not generated, and no unlocked retry exists.
func (p *RustPlan) Preparation() *PreparationPlan {
	if p == nil || p.blocked != "" || len(p.roots) == 0 || p.env.PrepareDir == "" {
		return nil
	}
	if p.preparation != nil {
		return p.preparation
	}
	stop := func(message string) *PreparationPlan {
		p.notice = message
		return nil
	}
	for _, path := range []string{p.env.CargoHome, p.env.RustupHome, p.env.PrepareDir} {
		if rustNoLinks(path) != nil || rustPathsOverlap(path, p.root.Project().Root) {
			return stop("Rust preparation needs symlink-free Cargo, rustup and app-data directories outside the project; no installation was proposed.")
		}
	}
	if st, err := os.Lstat(p.env.PrepareDir); err == nil {
		if !st.IsDir() {
			return stop("Rust preparation needs an app-owned working directory outside the project, not a file.")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return stop("Rust preparation working directory cannot be reviewed.")
	}
	for _, root := range p.roots {
		lock := rustJoin(root, "Cargo.lock")
		if !slices.Contains(p.configs, lock) {
			return stop("Rust preparation requires a reviewed Cargo.lock for every selected workspace. Generate and review missing lockfiles separately, then retry; preparation never fetches without --locked.")
		}
	}
	if p.selection == "" && (p.cargo == "" || p.rustc == "") {
		return stop("No concrete installed Cargo/rustc pair or explicit Rust toolchain selection was found. Select a channel in rust-toolchain.toml or configure a host default, then review preparation again; rustup is never bootstrapped automatically.")
	}
	dirs := append(slices.Clone(p.path), filepath.Join(p.env.CargoHome, "bin"))
	rustup := firstExecutable(dirs, rustExeName("rustup"))
	if rustup != "" && (rustNoLinks(rustup) != nil || rustPathsOverlap(rustup, p.root.Project().Root)) {
		return stop("Rust preparation requires a concrete existing rustup executable outside the project.")
	}
	missing := p.selection != "" && p.toolDir == ""
	if p.selection != "" && p.toolDir != "" && (p.cargo == "" || p.rustc == "") {
		return stop("The selected installed Rust toolchain is incomplete. Repair it separately and review again; preparation does not update or replace an existing toolchain.")
	}
	toolDir := p.toolDir
	if missing {
		name := p.selection
		if triple := rustHostTriple(); triple != "" && !strings.HasSuffix(name, "-"+triple) {
			name += "-" + triple
		}
		toolDir = filepath.Join(p.env.RustupHome, "toolchains", name)
	}
	// Component commands are idempotent and don't update the toolchain. Always
	// request the reviewed names rather than trusting mutable rustup inventories.
	// No inventory or whole-directory exemption enters the approval boundary.
	components := append(slices.Clone(p.components), "rust-src")
	slices.Sort(components)
	components = slices.Compact(components)
	var addComponents, addTargets []string
	if p.selection != "" {
		addComponents = components
		addTargets = slices.Clone(p.targets)
	}
	if (missing || len(addComponents) != 0 || len(addTargets) != 0) && rustup == "" {
		return stop("The selected Rust toolchain or components need preparation, but no concrete rustup executable is installed. Install rustup separately or prepare the selected toolchain manually; this app never bootstraps rustup.")
	}
	if p.cargo == "" && !missing {
		return nil
	}
	prep := &PreparationPlan{Covers: []string{"cargo-check", "rust-analyzer", "rust-prepare"}}
	prepEnv := func(executable string) []string {
		// This is a complete allowlist, not os.Environ: inherited wrappers, flags,
		// custom servers, Cargo aliases and credential providers never enter it.
		env := []string{"HOME=" + p.env.Home, "CARGO_HOME=" + p.env.CargoHome, "RUSTUP_HOME=" + p.env.RustupHome, "RUSTUP_AUTO_INSTALL=0", "CARGO_NET_OFFLINE=false", "PATH=" + strings.Join([]string{filepath.Dir(executable), "/usr/bin", "/bin"}, string(os.PathListSeparator)), "RUSTFLAGS=", "RUSTDOCFLAGS=", "CARGO_ENCODED_RUSTFLAGS=", "CARGO_ENCODED_RUSTDOCFLAGS=", "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER="}
		if p.rustc != "" {
			env = append(env, "RUSTC="+p.rustc)
		}
		return env
	}
	add := func(name, executable string, args ...string) {
		c := Command{ProviderID: fmt.Sprintf("rust-prepare-%d", len(prep.Steps)), Executable: executable, Args: args, Env: prepEnv(executable), PreparationDirectory: p.env.PrepareDir, AdapterPolicy: rustPreparationPolicy, AcceptedExits: []int{0}}
		prep.Steps = append(prep.Steps, PreparationStep{Name: name, Command: c})
		prep.Tools = append(prep.Tools, ToolInput{ProviderID: c.ProviderID, Executable: executable, ConfigPaths: p.ConfigPaths(), Policy: c.Policy()})
	}
	if missing {
		add("Install selected Rust toolchain", rustup, "toolchain", "install", p.selection, "--profile", "minimal", "--no-self-update")
		for _, name := range []string{"cargo", "rustc", "rustdoc"} {
			prep.Outputs = append(prep.Outputs, filepath.Join(toolDir, "bin", rustExeName(name)))
		}
		prep.Outputs = append(prep.Outputs, filepath.Join(toolDir, "libexec", rustExeName("rust-analyzer-proc-macro-srv")))
	}
	if len(addComponents) != 0 {
		add("Prepare selected Rust components", rustup, append([]string{"component", "add", "--toolchain", p.selection}, addComponents...)...)
		for _, component := range addComponents {
			prep.Outputs = append(prep.Outputs, filepath.Join(toolDir, "lib", "rustlib", "manifest-"+component))
			if component == "rust-src" {
				prep.Outputs = append(prep.Outputs, filepath.Join(toolDir, "lib", "rustlib", "src", "rust", "library", "Cargo.toml"))
			}
			for _, name := range map[string][]string{"clippy": {"cargo-clippy", "clippy-driver"}, "rustfmt": {"cargo-fmt", "rustfmt"}, "rust-analyzer": {"rust-analyzer"}}[component] {
				prep.Outputs = append(prep.Outputs, filepath.Join(toolDir, "bin", rustExeName(name)))
			}
		}
	}
	if len(addTargets) != 0 {
		add("Prepare selected Rust targets", rustup, append([]string{"target", "add", "--toolchain", p.selection}, addTargets...)...)
	}
	for i, root := range p.roots {
		args := []string{"fetch", "--locked", "--manifest-path", filepath.Join(p.root.Project().Root, filepath.FromSlash(root), "Cargo.toml")}
		if missing {
			add(fmt.Sprintf("Fetch locked Cargo dependencies %d", i+1), rustup, append([]string{"run", p.selection, "cargo"}, args...)...)
		} else {
			add(fmt.Sprintf("Fetch locked Cargo dependencies %d", i+1), p.cargo, args...)
		}
	}
	// All preexisting host files, including rustup settings, remain byte-exact.
	// A rustup version that rewrites existing settings conservatively needs fresh
	// review; no mutable-file exemption weakens the original composite approval.
	prep.Tools = append(prep.Tools, p.Tools("rust-prepare-input")...)
	if missing && p.settingsPath == "" {
		prep.Outputs = append(prep.Outputs, filepath.Join(p.env.RustupHome, "settings.toml"))
	}
	snapshot, err := p.preparationSnapshot(prep.Tools)
	if err != nil {
		return stop("Rust preparation inputs cannot be verified. Review the selected files, directories and installed tools before retrying.")
	}
	prep.Validate = func() error {
		current, err := p.preparationSnapshot(prep.Tools)
		if err != nil || !equalRustSnapshot(snapshot, current) {
			return failure("approvalChanged", "Rust preparation source, configuration or preexisting executable inputs changed; review again")
		}
		return nil
	}
	for i := range prep.Steps {
		prep.Steps[i].Command.ValidateStart = prep.Validate
	}
	p.preparation = prep
	prep.Prepared = func(registrations []Registration) error {
		if err := prep.Validate(); err != nil {
			return err
		}
		next, err := DiscoverRust(p.root, p.path, p.env)
		if err != nil {
			return err
		}
		return p.validatePrepared(next, prep.Outputs, registrations)
	}
	return prep
}

type rustPreparationSnapshot map[string]fileIdentity

func equalRustSnapshot(a, b rustPreparationSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for path, identity := range a {
		if identity != b[path] {
			return false
		}
	}
	return true
}

// preparationSnapshot deliberately excludes generated target/cache trees, but no
// directory containing execution inputs is exempt: project sources/configs and
// each preexisting executable/host configuration file retain exact identities.
func (p *RustPlan) preparationSnapshot(tools []ToolInput) (rustPreparationSnapshot, error) {
	if err := p.root.Revalidate(); err != nil {
		return nil, err
	}
	for _, path := range []string{p.env.CargoHome, p.env.RustupHome, p.env.PrepareDir} {
		if rustNoLinks(path) != nil || rustPathsOverlap(path, p.root.Project().Root) {
			return nil, failure("approvalChanged", "Rust preparation host directory changed")
		}
	}
	next := &RustPlan{root: p.root, env: p.env}
	if err := next.discoverProject(); err != nil {
		return nil, err
	}
	if err := next.discoverHost(); err != nil {
		return nil, err
	}
	next.hostConfigs = sortedRustPaths(next.hostInputs)
	if !slices.Equal(p.configs, sortedRustPaths(next.configs)) || !slices.Equal(p.roots, next.roots) || !slices.Equal(p.hostConfigs, next.hostConfigs) {
		return nil, failure("approvalChanged", "Rust project configuration enumeration changed")
	}
	paths := []string{}
	for _, path := range p.configs {
		paths = append(paths, filepath.Join(p.root.Project().Root, filepath.FromSlash(path)))
		if filepath.Base(path) == "Cargo.toml" {
			data, err := rootedRead(p.root, path, 1<<20)
			if err != nil {
				return nil, err
			}
			manifest, err := rustDecode(data)
			if err != nil {
				return nil, err
			}
			// Explicit build scripts may live under a skipped dependency/cache
			// directory. Bind that reviewed regular file individually, not its dir.
			if script, ok := rustTable(manifest["package"])["build"].(string); ok {
				paths = append(paths, filepath.Join(p.root.Project().Root, filepath.FromSlash(rustJoin(filepath.ToSlash(filepath.Dir(path)), script))))
			}
		}
	}
	for _, tool := range tools {
		paths = append(paths, tool.Executable)
	}
	entries := 0
	err := filepath.WalkDir(p.root.Project().Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != p.root.Project().Root && slices.Contains([]string{"target", "vendor", "node_modules", ".git", ".hg", ".svn", ".cache"}, d.Name()) {
			return filepath.SkipDir
		}
		entries++
		if entries > maxRustEntries+1 || d.Type()&os.ModeSymlink != 0 {
			return failure("approvalChanged", "Rust source enumeration cannot be verified")
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".rs" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := rustPreparationSnapshot{}
	var total int64
	for _, path := range sortedRustPaths(paths) {
		if err := rustNoLinks(path); err != nil {
			return nil, err
		}
		identity, err := identifyFile(path, &total, nil, "")
		if err != nil {
			return nil, err
		}
		out[path] = identity
	}
	return out, nil
}

func sortedRustPaths(paths []string) []string {
	paths = slices.Clone(paths)
	slices.Sort(paths)
	return slices.Compact(paths)
}

// ValidatePrepared is conservative: preparation may add only the explicitly
// selected toolchain's expected executable identities. Replacing an existing
// Cargo/rustc or adding unrelated analyzers always requires a fresh approval.
func (p *RustPlan) ValidatePrepared(next *RustPlan) error {
	prep := p.Preparation()
	if prep == nil {
		return failure("approvalChanged", "No reviewed Rust preparation plan exists")
	}
	if err := prep.Validate(); err != nil {
		return err
	}
	return p.validatePrepared(next, prep.Outputs, nil)
}

func (p *RustPlan) validatePrepared(next *RustPlan, outputs []string, registrations []Registration) error {
	if next == nil || next.blocked != "" || p.selection != next.selection || !slices.Equal(p.roots, next.roots) || !slices.Equal(p.configs, next.configs) || !slices.Equal(p.components, next.components) || !slices.Equal(p.targets, next.targets) {
		return failure("approvalChanged", "Rust preparation changed unplanned selection or project inputs")
	}
	old := append([]string{p.cargo, p.rustc, p.rustdoc, p.procMacro, p.analyzer}, p.hostInputs...)
	for _, path := range append([]string{next.cargo, next.rustc, next.rustdoc, next.procMacro, next.analyzer}, next.hostInputs...) {
		if path == "" || slices.Contains(old, path) {
			continue
		}
		if !slices.Contains(outputs, path) || rustNoLinks(path) != nil || !rustRegularFile(path) {
			return failure("approvalChanged", "Rust preparation discovered an unplanned tool or host input; review again")
		}
	}
	// Existing settings are already byte-exact in the snapshot. A newly created
	// settings file may set only rustup's bookkeeping/default keys, never overrides.
	if p.settings == nil {
		for key := range next.settings {
			if key != "default_toolchain" && key != "profile" && key != "version" {
				return failure("approvalChanged", "Rust preparation created unplanned rustup settings; review again")
			}
		}
	}
	if next.cargo == "" || next.rustc == "" {
		return failure("approvalChanged", "Rust preparation did not produce the expected tools")
	}
	if registrations == nil {
		return nil
	}
	// The callback binds the exact registrations the manager will carry forward.
	// Their covered Rust tools must match the validated rediscovery, including
	// preparation tools; a second discovery is never approval by itself.
	expected := map[string]ToolInput{}
	cargo, err := NewCargoCheckPlan(next.root, next)
	if err != nil {
		return err
	}
	expectedRegistrations := []Registration{{ProviderID: "cargo-check", Adapter: cargo, Prepare: next.Preparation()}, rustLiveRegistration(next)}
	for _, registration := range expectedRegistrations {
		for _, tool := range registrationTools(registration) {
			expected[tool.ProviderID+"\x00"+tool.Executable] = tool
		}
	}
	actual := map[string]ToolInput{}
	for _, registration := range registrations {
		covered := false
		for _, prefix := range []string{"cargo-check", "rust-analyzer", "rust-prepare"} {
			covered = covered || strings.HasPrefix(registration.ProviderID, prefix)
		}
		if !covered {
			continue
		}
		for _, tool := range registrationTools(registration) {
			actual[tool.ProviderID+"\x00"+tool.Executable] = tool
		}
	}
	// Bind both directions: the actual covered registrations must equal the
	// validated rediscovery exactly. A dropped covered tool (for example a
	// vanished analyzer) is as suspicious as an added or altered one, and a
	// second discovery is never approval by itself.
	for key, tool := range actual {
		want, ok := expected[key]
		if !ok || tool.Policy != want.Policy || !slices.Equal(tool.ConfigPaths, want.ConfigPaths) {
			return failure("approvalChanged", "Prepared Rust registration does not match validated discovery")
		}
	}
	for key, tool := range expected {
		got, ok := actual[key]
		if !ok || tool.Policy != got.Policy || !slices.Equal(tool.ConfigPaths, got.ConfigPaths) {
			return failure("approvalChanged", "Prepared Rust registration is missing a reviewed tool")
		}
	}
	return nil
}

func registrationTools(r Registration) []ToolInput {
	var tools []ToolInput
	if r.Adapter != nil {
		tools = append(tools, r.Adapter.Tools()...)
	}
	if r.Live != nil && r.Availability == "available" {
		tools = append(tools, liveTools(r.Live)...)
	}
	if r.Prepare != nil {
		tools = append(tools, r.Prepare.Tools...)
	}
	return tools
}

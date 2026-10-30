package codediagnostics

// Rust discovery is deliberately metadata-only. In particular a rustup proxy is
// not evidence of an installed toolchain, and discovery must never ask it to
// resolve a channel (which can install software).
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ginkcode/agent-sessions/internal/managedtools"
)

const rustDiscoveryPolicy = "rust-discovery-v1:contained:offline:locked:build-scripts-proc-macros-approved:no-inherited-flags:no-auto-install"
const maxRustEntries = 8192
const maxRustManifests = 64

// RustEnvironment contains reviewed host paths, never inherited Rust flags or
// wrappers. Empty CargoHome/RustupHome use the conventional directories in Home.
// PrepareDir is an app-owned absolute directory, outside the project, used as
// the working directory of reviewed preparation commands.
type RustEnvironment struct{ Home, CargoHome, RustupHome, PrepareDir string }

type RustPlan struct {
	root                              AuthorizedRoot
	path                              []string
	env                               RustEnvironment
	roots, configs, hostInputs        []string
	cargo, rustc, analyzer, selection string
	rustdoc, procMacro                string
	components, targets               []string
	toolDir                           string // installed concrete selected toolchain directory, if any
	settings                          map[string]any
	settingsPath                      string
	hostConfigs                       []string
	blocked, notice                   string
	fingerprint                       string
	preparation                       *PreparationPlan
}

func (p *RustPlan) Roots() []string       { return append([]string{}, p.roots...) }
func (p *RustPlan) ConfigPaths() []string { return slices.Clone(p.configs) }
func (p *RustPlan) Rustc() string         { return p.rustc }
func (p *RustPlan) Cargo() (string, bool) {
	return p.cargo, p.Availability("cargo-check") == "available"
}
func (p *RustPlan) RustAnalyzer() (string, bool) {
	return p.analyzer, p.Availability("rust-analyzer") == "available"
}
func (p *RustPlan) Notice() string { return p.notice }

// Rustdoc and ProcMacroServer are optional concrete toolchain components. They
// are bound in Tools when present; empty means the component is not installed.
func (p *RustPlan) Rustdoc() string         { return p.rustdoc }
func (p *RustPlan) ProcMacroServer() string { return p.procMacro }
func (p *RustPlan) Availability(provider string) string {
	if p.blocked != "" {
		return p.blocked
	}
	if len(p.roots) == 0 {
		return "unsupported"
	}
	if p.cargo == "" || p.rustc == "" || (provider == "rust-analyzer" && p.analyzer == "") {
		return "missingTool"
	}
	return "available"
}

// Environment is the entire adapter environment. PATH contains only the concrete
// toolchain plus system utilities, never ~/.cargo/bin proxies. This is not a
// sandbox: approved build scripts and procedural macros can write/run/network.
func (p *RustPlan) Environment() []string {
	dirs := []string{}
	for _, tool := range []string{p.cargo, p.rustc} {
		if tool != "" && !slices.Contains(dirs, filepath.Dir(tool)) {
			dirs = append(dirs, filepath.Dir(tool))
		}
	}
	dirs = append(dirs, "/usr/bin", "/bin")
	env := []string{"HOME=" + p.env.Home, "CARGO_HOME=" + p.env.CargoHome, "RUSTUP_HOME=" + p.env.RustupHome, "RUSTUP_AUTO_INSTALL=0", "CARGO_NET_OFFLINE=true", "RUSTC=" + p.rustc, "PATH=" + strings.Join(dirs, string(os.PathListSeparator)), "RUSTFLAGS=", "RUSTDOCFLAGS=", "CARGO_ENCODED_RUSTFLAGS=", "CARGO_ENCODED_RUSTDOCFLAGS=", "RUSTC_WRAPPER=", "RUSTC_WORKSPACE_WRAPPER="}
	if p.rustdoc != "" {
		env = append(env, "RUSTDOC="+p.rustdoc)
	}
	return env
}

// Tools supplies additional approval inputs, not the adapter's command identity.
// Callers must also bind their exact Command/ServerConfig (argv/settings/dir).
// Host metadata is a tool identity, never a bogus project-relative ConfigPath.
func (p *RustPlan) Tools(provider string) []ToolInput {
	data, _ := json.Marshal(struct {
		Environment []string
		Selection   string
		Roots       []string
	}{p.Environment(), p.selection, p.roots})
	policy := rustDiscoveryPolicy + ":" + string(data)
	var out []ToolInput
	for _, path := range append([]string{p.cargo, p.rustc}, p.hostInputs...) {
		if path == "" {
			continue
		}
		sum := sha256.Sum256([]byte(path))
		t := ToolInput{ProviderID: provider + "-rust-input-" + hex.EncodeToString(sum[:]), Executable: path, Policy: policy}
		if len(out) == 0 {
			t.ConfigPaths = slices.Clone(p.configs)
		}
		out = append(out, t)
	}
	// A missing Cargo can coexist with a concrete server, useful for independent
	// setup status. Do not duplicate a component if it is itself a host input.
	if provider == "rust-analyzer" && p.analyzer != "" {
		out = append(out, ToolInput{ProviderID: provider + "-rust-server", Executable: p.analyzer, ConfigPaths: slices.Clone(p.configs), Policy: policy})
	}
	return out
}

// Unchanged must be called before execution AND before publication. Re-enumerate
// first: hashing the old list alone misses newly created configs and manifests.
func (p *RustPlan) Unchanged() error {
	next, err := DiscoverRust(p.root, p.path, p.env)
	if err != nil {
		return err
	}
	if p.fingerprint == "" || p.fingerprint != next.fingerprint {
		return failure("approvalChanged", "Rust workspace, configuration or installed toolchain changed; review again")
	}
	return nil
}

// DiscoverRust never runs cargo metadata, rustc, rustup, or rust-analyzer.
// Unsupported project/tool selection is an actionable unavailable plan, not a
// partial plan that would still grant Cargo permission to load external files.
func DiscoverRust(root AuthorizedRoot, reviewedPath []string, env RustEnvironment) (*RustPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	if env.CargoHome == "" {
		env.CargoHome = filepath.Join(env.Home, ".cargo")
	}
	if env.RustupHome == "" {
		env.RustupHome = filepath.Join(env.Home, ".rustup")
	}
	for _, path := range []string{env.Home, env.CargoHome, env.RustupHome} {
		if !filepath.IsAbs(path) || !bounded(path, 4096) || hasParent(path) {
			return nil, failure("invalidArgument", "Reviewed absolute Rust home paths are required")
		}
	}
	p := &RustPlan{root: root, path: slices.Clone(reviewedPath), env: env}
	block := func(err error) {
		if p.blocked == "" {
			p.blocked = "unsupported"
			var e *Error
			if errors.As(err, &e) {
				p.notice = shorten(e.Message, 1800)
			} else {
				p.notice = "Rust project configuration cannot be verified."
			}
		}
	}
	if err := p.discoverProject(); err != nil {
		block(err)
	}
	if err := p.discoverHost(); err != nil {
		block(err)
	}
	p.hostConfigs = slices.Clone(p.hostInputs)
	slices.Sort(p.hostConfigs)
	p.hostConfigs = slices.Compact(p.hostConfigs)
	if err := p.resolveTools(); err != nil {
		block(err)
	}
	if p.notice == "" {
		switch {
		case len(p.roots) == 0:
			p.notice = "No contained Cargo package or workspace was found."
		case p.cargo == "" || p.rustc == "":
			p.notice = "Install a complete Rust toolchain on this host; rustup proxies alone are not installed Cargo/rustc components. No tool is installed automatically."
		case p.analyzer == "":
			p.notice = "rust-analyzer is not installed on this host; saved Cargo checking remains available."
		}
	}
	slices.Sort(p.configs)
	p.configs = slices.Compact(p.configs)
	slices.Sort(p.hostInputs)
	p.hostInputs = slices.Compact(p.hostInputs)
	// The snapshot binds state/enumeration even when no executable is available.
	tools := p.Tools("rust-analyzer")
	snapshot := struct {
		Roots, Configs, Hosts                      []string
		Cargo, Rustc, Analyzer, Selection, Blocked string
		Fingerprint                                string
	}{p.roots, p.configs, p.hostInputs, p.cargo, p.rustc, p.analyzer, p.selection, p.blocked, ""}
	if len(tools) > 0 {
		fp, err := Fingerprint(root, tools)
		if err != nil {
			return nil, err
		}
		snapshot.Fingerprint = fp
	}
	b, _ := json.Marshal(snapshot)
	h := sha256.Sum256(b)
	p.fingerprint = hex.EncodeToString(h[:])
	return p, nil
}

type rustManifest struct {
	dir              string
	raw              map[string]any
	pkg, workspace   bool
	members, exclude []string
	pointer          string
}

func rustTable(v any) map[string]any { m, _ := v.(map[string]any); return m }
func rustStrings(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	a, ok := v.([]any)
	if !ok {
		if ss, yes := v.([]string); yes {
			return ss, true
		}
		return nil, false
	}
	var out []string
	for _, v := range a {
		s, ok := v.(string)
		if !ok || !bounded(s, 4096) {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
func rustDecode(data []byte) (map[string]any, error) {
	var m map[string]any
	_, err := toml.Decode(string(data), &m)
	if err != nil {
		return nil, failure("unsupported", "Cargo TOML cannot be parsed; correct it before enabling Rust tools")
	}
	return m, nil
}

func (p *RustPlan) discoverProject() error {
	r, err := os.OpenRoot(p.root.Project().Root)
	if err != nil {
		return err
	}
	defer r.Close()
	manifests := map[string]*rustManifest{}
	entries := 0
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		f, err := r.Open(dir)
		if err != nil {
			return failure("unsupported", "Rust directory cannot be reviewed")
		}
		ds, err := f.ReadDir(maxRustEntries + 1 - entries)
		f.Close()
		if err != nil && !errors.Is(err, io.EOF) {
			return failure("unsupported", "Rust directory cannot be enumerated")
		}
		for _, d := range ds {
			entries++
			if entries > maxRustEntries {
				return failure("limitExceeded", "Rust discovery entry limit exceeded")
			}
			rel := filepath.ToSlash(filepath.Join(dir, d.Name()))
			if ValidatePath(rel) != nil {
				return failure("unsupported", "Rust discovery encountered an unsupported path")
			}
			if d.Type()&os.ModeSymlink != 0 {
				return failure("unsupported", "Rust discovery does not follow project symlinks; review a symlink-free Cargo scope")
			}
			if d.IsDir() {
				if slices.Contains([]string{"target", "vendor", "node_modules", ".git", ".hg", ".svn", ".cache"}, d.Name()) {
					continue
				}
				if depth >= 32 {
					return failure("limitExceeded", "Rust directory depth limit exceeded")
				}
				if err := walk(rel, depth+1); err != nil {
					return err
				}
				continue
			}
			name := d.Name()
			relevant := name == "Cargo.toml" || name == "Cargo.lock" || name == "rust-toolchain" || name == "rust-toolchain.toml" || name == "rust-analyzer.toml" || name == ".rust-analyzer.toml" || (filepath.Base(filepath.Dir(rel)) == ".cargo" && (name == "config" || name == "config.toml"))
			if !relevant {
				continue
			}
			if len(p.configs) >= 128 {
				return failure("limitExceeded", "Rust configuration limit exceeded")
			}
			p.configs = append(p.configs, rel)
			data, err := rootedRead(p.root, rel, 1<<20)
			if err != nil {
				return failure("unsupported", "Rust project configuration cannot be reviewed")
			}
			if filepath.Base(filepath.Dir(rel)) == ".cargo" && (name == "config" || name == "config.toml") {
				if err := rustSafeConfig(data); err != nil {
					return err
				}
			}
			// Workspace rust-analyzer configuration overrides client settings, including
			// executable Cargo/wrapper settings. Until harmless options are explicitly
			// allowlisted, only an empty configuration is a reviewed no-op.
			if name == "rust-analyzer.toml" || name == ".rust-analyzer.toml" {
				m, err := rustDecode(data)
				if err != nil {
					return err
				}
				if len(m) != 0 {
					return failure("unsupported", "Nonempty rust-analyzer.toml configuration can override the reviewed execution policy; remove it before enabling Rust tools")
				}
			}
			if name != "Cargo.toml" {
				continue
			}
			if len(manifests) >= maxRustManifests {
				return failure("limitExceeded", "Rust manifest limit exceeded")
			}
			raw, err := rustDecode(data)
			if err != nil {
				return err
			}
			base := filepath.ToSlash(filepath.Dir(rel))
			if base == "." {
				base = ""
			}
			m := &rustManifest{dir: base, raw: raw, pkg: raw["package"] != nil, workspace: raw["workspace"] != nil}
			if !m.pkg && !m.workspace {
				return failure("unsupported", "Cargo manifest has neither package nor workspace")
			}
			if w := rustTable(raw["workspace"]); w != nil {
				var ok bool
				m.members, ok = rustStrings(w["members"])
				if !ok {
					return failure("unsupported", "Cargo workspace members must be paths")
				}
				m.exclude, ok = rustStrings(w["exclude"])
				if !ok {
					return failure("unsupported", "Cargo workspace exclusions must be paths")
				}
				for _, pat := range append(slices.Clone(m.members), m.exclude...) {
					if !localInside(base, pat) || strings.Contains(pat, "**") || strings.Contains(pat, `\`) {
						return failure("unsupported", "Cargo workspace members/exclusions must be contained, supported path globs")
					}
				}
			}
			if pkg := rustTable(raw["package"]); pkg != nil {
				if v, ok := pkg["workspace"]; ok {
					var valid bool
					m.pointer, valid = v.(string)
					if !valid || !localInside(base, m.pointer) {
						return failure("unsupported", "Cargo package workspace points outside the authorized root")
					}
				}
			}
			if err := p.validateManifestPaths(base, raw); err != nil {
				return err
			}
			manifests[base] = m
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return err
	}
	// A directory-valued path dependency must have been parsed by this walk,
	// not hidden under a skipped dependency/cache directory.
	for base, manifest := range manifests {
		var check func(any) error
		check = func(v any) error {
			if m := rustTable(v); m != nil {
				if path, ok := m["path"].(string); ok {
					rel := rustJoin(base, path)
					st, err := os.Stat(filepath.Join(p.root.Project().Root, filepath.FromSlash(rel)))
					if err == nil && st.IsDir() && manifests[rel] == nil {
						return failure("unsupported", "Cargo path dependency is in an unreviewed or skipped directory")
					}
				}
				for _, child := range m {
					if err := check(child); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := check(manifest.raw); err != nil {
			return err
		}
	}
	// Resolve each package to one contained workspace. A nested [workspace] is
	// independent, and is not accidentally swallowed by an ancestor member glob.
	owners := map[string]string{}
	for dir, m := range manifests {
		if m.workspace {
			owners[dir] = dir
			continue
		}
		if !m.pkg {
			continue
		}
		candidate := ""
		found := false
		for wdir, w := range manifests {
			if !w.workspace || !rustWithin(wdir, dir) || wdir == dir {
				continue
			}
			if !found || len(wdir) > len(candidate) {
				candidate = wdir
				found = true
			}
		}
		if m.pointer != "" {
			target := rustJoin(dir, m.pointer)
			w := manifests[target]
			if w == nil || !w.workspace {
				return failure("unsupported", "Explicit Cargo workspace is not a reviewed contained workspace")
			}
			candidate = target
			found = true
		}
		if !found {
			owners[dir] = dir
			continue
		}
		w := manifests[candidate]
		rel, _ := filepath.Rel(filepath.FromSlash(candidate), filepath.FromSlash(dir))
		rel = filepath.ToSlash(rel)
		excluded, err := rustMatchAny(w.exclude, rel)
		if err != nil {
			return err
		}
		member, err := rustMatchAny(w.members, rel)
		if err != nil {
			return err
		}
		if excluded {
			if m.pointer != "" {
				return failure("unsupported", "Cargo package has a conflicting workspace exclusion")
			}
			owners[dir] = dir
			continue
		}
		if !member { // Cargo implicitly includes contained local path dependencies.
			member = rustManifestReferences(w.raw, candidate, dir)
			if !member {
				for pd, pm := range manifests {
					if rustWithin(candidate, pd) && rustManifestReferences(pm.raw, pd, dir) {
						member = true
						break
					}
				}
			}
		}
		if !member {
			return failure("unsupported", "A nested Cargo package is not a reviewed workspace member; add it to members or exclude it explicitly")
		}
		owners[dir] = candidate
	}
	for dir, m := range manifests {
		if !m.workspace {
			continue
		}
		for _, pattern := range m.members {
			matched := false
			for pkg, owner := range owners {
				if owner != dir || !manifests[pkg].pkg {
					continue
				}
				rel, _ := filepath.Rel(filepath.FromSlash(dir), filepath.FromSlash(pkg))
				ok, err := rustMatchAny([]string{pattern}, filepath.ToSlash(rel))
				if err != nil {
					return err
				}
				matched = matched || ok
			}
			if !matched {
				return failure("unsupported", "Cargo workspace member has no reviewed contained package")
			}
		}
	}
	for _, owner := range owners {
		if !slices.Contains(p.roots, owner) {
			p.roots = append(p.roots, owner)
		}
	}
	slices.Sort(p.roots)
	return nil
}

func rustWithin(parent, child string) bool {
	return parent == "" || parent == child || strings.HasPrefix(child, parent+"/")
}
func rustJoin(base, target string) string {
	s := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.FromSlash(base), filepath.FromSlash(target))))
	if s == "." {
		return ""
	}
	return s
}
func rustMatchAny(patterns []string, path string) (bool, error) {
	for _, pat := range patterns {
		ok, err := filepath.Match(filepath.FromSlash(pat), filepath.FromSlash(path))
		if err != nil {
			return false, failure("unsupported", "Cargo workspace glob cannot be verified")
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}
func rustManifestReferences(raw map[string]any, base, target string) bool {
	found := false
	var scan func(any)
	scan = func(v any) {
		if m := rustTable(v); m != nil {
			if path, ok := m["path"].(string); ok && rustJoin(base, path) == target {
				found = true
			}
			for _, v := range m {
				scan(v)
			}
		}
	}
	scan(raw)
	return found
}
func (p *RustPlan) validateManifestPaths(base string, raw map[string]any) error {
	var scan func(any) error
	scan = func(v any) error {
		if m := rustTable(v); m != nil {
			for k, v := range m {
				// All Cargo local dependency/patch/replace paths and target/build paths
				// must stay inside the catalog root, including symlink resolution.
				if k == "path" {
					s, ok := v.(string)
					if !ok || !localInside(base, s) {
						return failure("unsupported", "Cargo local path leaves the authorized root")
					}
					rel := rustJoin(base, s)
					if rel != "" {
						r, err := os.OpenRoot(p.root.Project().Root)
						if err != nil {
							return err
						}
						st, err := r.Stat(filepath.FromSlash(rel))
						r.Close()
						if err != nil || (!st.IsDir() && !st.Mode().IsRegular()) {
							return failure("unsupported", "Cargo local path cannot be verified inside the authorized root")
						}
					}
				}
				if k == "build" || k == "readme" || k == "license-file" {
					if s, ok := v.(string); ok {
						if !localInside(base, s) {
							return failure("unsupported", "Cargo package file leaves the authorized root")
						}
						// Skipped cache/dependency directories are not walked. Resolve
						// explicit scripts as regular contained files even under them.
						if k == "build" {
							if _, err := p.root.ResolvePath(p.root.Project().HostID, rustJoin(base, s)); err != nil {
								return failure("unsupported", "Cargo build script cannot be verified inside the authorized root")
							}
						}
					}
				}
				if err := scan(v); err != nil {
					return err
				}
			}
		}
		if a, ok := v.([]any); ok {
			for _, v := range a {
				if err := scan(v); err != nil {
					return err
				}
			}
		}
		if a, ok := v.([]map[string]any); ok {
			for _, v := range a {
				if err := scan(v); err != nil {
					return err
				}
			}
		}
		return nil
	}
	// Cargo treats package.metadata and workspace.metadata as opaque data for
	// other tools; their keys are not Cargo dependency or target paths.
	reviewed := make(map[string]any, len(raw))
	for key, value := range raw {
		if key == "package" || key == "workspace" {
			if table := rustTable(value); table != nil {
				copy := make(map[string]any, len(table))
				for k, v := range table {
					if k != "metadata" {
						copy[k] = v
					}
				}
				value = copy
			}
		}
		reviewed[key] = value
	}
	return scan(reviewed)
}

// Cargo configuration is executable authority (wrappers, aliases, credential
// providers, rustflags, source replacements, env, runner, target-dir). Initially
// accept only the offline=true no-op; unknown options fail closed, not ignored.
func rustSafeConfig(data []byte) error {
	m, err := rustDecode(data)
	if err != nil {
		return err
	}
	for k, v := range m {
		if k != "net" {
			return failure("unsupported", "Cargo configuration options are not supported by the reviewed Rust policy; only net.offline=true is allowed")
		}
		net := rustTable(v)
		if net == nil {
			return failure("unsupported", "Cargo net configuration cannot be verified")
		}
		for key, val := range net {
			if key != "offline" || val != true {
				return failure("unsupported", "Cargo configuration must not weaken offline policy or add unreviewed behavior")
			}
		}
	}
	return nil
}

// readHost is bounded and excludes symlinks. Host files are identity contributions
// rather than project ConfigPaths; optional absence is re-enumerated every time.
func (p *RustPlan) readHost(path string) ([]byte, bool, error) {
	if !filepath.IsAbs(path) || !bounded(path, 4096) || hasParent(path) || filepath.Clean(path) != path {
		return nil, false, failure("unsupported", "Rust host paths must be clean absolute paths")
	}
	cur := path
	for {
		st, err := os.Lstat(cur)
		if err == nil && st.Mode()&os.ModeSymlink != 0 {
			return nil, false, failure("unsupported", "Rust host paths must not contain symlinks")
		}
		if filepath.Dir(cur) == cur {
			break
		}
		cur = filepath.Dir(cur)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false, failure("unsupported", "Rust host path cannot be reviewed")
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
	}
	st, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !st.Mode().IsRegular() || st.Size() > 1<<20 {
		return nil, false, failure("unsupported", "Rust host configuration is not a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, failure("unsupported", "Rust host configuration cannot be read")
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, false, failure("unsupported", "Rust host configuration exceeds read limit")
	}
	after, err := f.Stat()
	current, e := os.Lstat(path)
	if err != nil || e != nil || !os.SameFile(st, current) || st.Size() != after.Size() || st.ModTime() != after.ModTime() {
		return nil, false, failure("approvalChanged", "Rust host configuration changed during discovery")
	}
	p.hostInputs = append(p.hostInputs, path)
	return data, true, nil
}
func (p *RustPlan) discoverHost() error {
	// Cargo reads config in all ancestors, independently of workspace boundaries.
	// Read ancestors deliberately, but never add their packages to execution roots.
	for dir := filepath.Dir(p.root.Project().Root); ; dir = filepath.Dir(dir) {
		for _, name := range []string{"config", "config.toml"} {
			data, ok, err := p.readHost(filepath.Join(dir, ".cargo", name))
			if err != nil {
				return err
			}
			if ok {
				if err := rustSafeConfig(data); err != nil {
					return err
				}
			}
		}
		data, ok, err := p.readHost(filepath.Join(dir, "Cargo.toml"))
		if err != nil {
			return err
		}
		if ok {
			m, err := rustDecode(data)
			if err != nil {
				return err
			}
			if m["workspace"] != nil { // root-owned workspaces explicitly stop Cargo's parent search.
				for _, root := range p.roots {
					data, err := rootedRead(p.root, rustJoin(root, "Cargo.toml"), 1<<20)
					if err != nil {
						return err
					}
					own, err := rustDecode(data)
					if err != nil {
						return err
					}
					if own["workspace"] == nil {
						return failure("unsupported", "Cargo would select an ancestor workspace outside the authorized root; authorize that workspace explicitly")
					}
				}
			}
		}
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	for _, name := range []string{"config", "config.toml"} {
		data, ok, err := p.readHost(filepath.Join(p.env.CargoHome, name))
		if err != nil {
			return err
		}
		if ok {
			if err := rustSafeConfig(data); err != nil {
				return err
			}
		}
	}
	// Cargo also searches the preparation cwd and its ancestors. Those paths
	// need not overlap the project ancestors, and a manifest-path does not stop
	// Cargo loading their configuration. Optional absence is re-enumerated.
	if p.env.PrepareDir != "" {
		for dir := p.env.PrepareDir; ; dir = filepath.Dir(dir) {
			for _, name := range []string{"config", "config.toml"} {
				data, ok, err := p.readHost(filepath.Join(dir, ".cargo", name))
				if err != nil {
					return err
				}
				if ok {
					if err := rustSafeConfig(data); err != nil {
						return err
					}
				}
			}
			if parent := filepath.Dir(dir); parent == dir {
				break
			}
		}
	}
	// rust-analyzer's user configuration overrides client settings too. The
	// complete environment omits XDG_CONFIG_HOME, so Linux resolves under HOME.
	// Native Windows live analysis is unsupported and has no reviewed path here.
	var analyzerConfig string
	switch runtime.GOOS {
	case "linux":
		analyzerConfig = filepath.Join(p.env.Home, ".config", "rust-analyzer", "rust-analyzer.toml")
	case "darwin":
		analyzerConfig = filepath.Join(p.env.Home, "Library", "Application Support", "rust-analyzer", "rust-analyzer.toml")
	}
	if analyzerConfig != "" {
		data, ok, err := p.readHost(analyzerConfig)
		if err != nil {
			return err
		}
		if ok {
			m, err := rustDecode(data)
			if err != nil {
				return err
			}
			if len(m) != 0 {
				return failure("unsupported", "Nonempty host rust-analyzer.toml configuration can override the reviewed execution policy; remove it before enabling Rust tools")
			}
		}
	}
	return nil
}

func rustHostTriple() string {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64", "386": "i686"}[runtime.GOARCH]
	suffix := map[string]string{"darwin": "apple-darwin", "linux": "unknown-linux-gnu", "windows": "pc-windows-msvc"}[runtime.GOOS]
	if arch == "" || suffix == "" {
		return ""
	}
	return arch + "-" + suffix
}

// rustSelection is one reviewed toolchain selector: a concrete channel plus the
// bounded component/target declarations rustup would otherwise install on load.
type rustSelection struct {
	name                string
	components, targets []string
}

func (s rustSelection) key() string {
	return s.name + "\x00" + strings.Join(s.components, ",") + "\x00" + strings.Join(s.targets, ",")
}

var (
	rustComponentName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
	rustTargetName    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	rustChannelName   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// rustNameList validates a bounded, duplicate-free declaration list.
func rustNameList(v any, re *regexp.Regexp) ([]string, error) {
	ss, ok := rustStrings(v)
	if !ok || len(ss) > 32 {
		return nil, failure("unsupported", "Rust toolchain component/target declarations must be a short list of names")
	}
	out := []string{}
	for _, s := range ss {
		if !re.MatchString(s) {
			return nil, failure("unsupported", "Rust toolchain component/target name is unsupported")
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

func rustToolchainSelection(data []byte, tomlFile bool) (rustSelection, error) {
	var sel rustSelection
	var name string
	if !tomlFile {
		name = strings.TrimSpace(string(data))
		if strings.ContainsAny(name, "\r\n") {
			return sel, failure("unsupported", "Legacy rust-toolchain must contain only an installed channel name")
		}
	} else {
		m, err := rustDecode(data)
		if err != nil {
			return sel, err
		}
		t := rustTable(m["toolchain"])
		if len(m) != 1 || t == nil {
			return sel, failure("unsupported", "Rust toolchain selection cannot be verified")
		}
		for k := range t {
			if k != "channel" && k != "components" && k != "targets" && k != "profile" {
				return sel, failure("unsupported", "Custom/path Rust toolchain selectors are not supported")
			}
		}
		var ok bool
		name, ok = t["channel"].(string)
		if !ok {
			return sel, failure("unsupported", "Rust toolchain needs a concrete installed channel")
		}
		if v, present := t["profile"]; present {
			if profile, _ := v.(string); profile != "minimal" && profile != "default" && profile != "complete" {
				return sel, failure("unsupported", "Rust toolchain profile is unsupported")
			}
		}
		// Declarations are only ever satisfied by an explicit, approved preparation
		// step; discovery itself never asks rustup to install them.
		if sel.components, err = rustNameList(t["components"], rustComponentName); err != nil {
			return sel, err
		}
		if sel.targets, err = rustNameList(t["targets"], rustTargetName); err != nil {
			return sel, err
		}
	}
	if !bounded(name, 256) || !rustChannelName.MatchString(name) || name == "." || name == ".." {
		return sel, failure("unsupported", "Rust toolchain channel name is unsupported")
	}
	sel.name = name
	return sel, nil
}
func (p *RustPlan) resolveTools() error {
	// Apply the nearest toolchain file for every execution root, requiring one
	// concrete selection for a plan. Never silently choose one nested selection.
	selected := rustSelection{}
	selectionSet := false
	for _, root := range p.roots {
		dir := filepath.Join(p.root.Project().Root, filepath.FromSlash(root))
		choice := rustSelection{}
		for {
			count := 0
			for _, name := range []string{"rust-toolchain", "rust-toolchain.toml"} {
				path := filepath.Join(dir, name)
				var data []byte
				var ok bool
				var err error
				rel, e := filepath.Rel(p.root.Project().Root, path)
				if e == nil && filepath.IsLocal(rel) {
					_, e := os.Lstat(path)
					if e == nil {
						data, err = rootedRead(p.root, filepath.ToSlash(rel), 1<<20)
						ok = err == nil
					} else if !errors.Is(e, os.ErrNotExist) {
						err = e
					}
				} else {
					data, ok, err = p.readHost(path)
				}
				if err != nil {
					return failure("unsupported", "Rust toolchain selection cannot be read")
				}
				if ok {
					count++
					var err error
					choice, err = rustToolchainSelection(data, name == "rust-toolchain.toml")
					if err != nil {
						return err
					}
				}
			}
			if count > 1 {
				return failure("unsupported", "Both rust-toolchain and rust-toolchain.toml exist; choose one unambiguous selector")
			}
			if choice.name != "" || filepath.Dir(dir) == dir {
				break
			}
			dir = filepath.Dir(dir)
		}
		// Absence is a selection too: it must not silently inherit another
		// independent root's explicit selector instead of the host default/PATH.
		if selectionSet && selected.name != choice.name {
			return failure("unsupported", "Multiple Rust toolchain selections require separate authorized scopes")
		}
		selected.name = choice.name
		selected.components = append(selected.components, choice.components...)
		selected.targets = append(selected.targets, choice.targets...)
		selectionSet = true
	}
	data, ok, err := p.readHost(filepath.Join(p.env.RustupHome, "settings.toml"))
	if err != nil {
		return err
	}
	if ok {
		m, err := rustDecode(data)
		if err != nil {
			return err
		}
		if overrides := rustTable(m["overrides"]); len(overrides) > 0 {
			for path, value := range overrides {
				if !filepath.IsAbs(path) {
					return failure("unsupported", "Rustup override paths cannot be verified")
				}
				for _, root := range p.roots {
					dir := filepath.Join(p.root.Project().Root, filepath.FromSlash(root))
					rel, err := filepath.Rel(path, dir)
					if err == nil && (rel == "." || filepath.IsLocal(rel)) {
						_ = value
						return failure("unsupported", "Applicable rustup directory overrides are not supported; use a reviewed rust-toolchain.toml")
					}
				}
			}
		}
		p.settings = m
		p.settingsPath = filepath.Join(p.env.RustupHome, "settings.toml")
		if selected.name == "" {
			selected.name, _ = m["default_toolchain"].(string)
		}
	}
	p.selection = selected.name
	p.components, p.targets = selected.components, selected.targets
	slices.Sort(p.components)
	p.components = slices.Compact(p.components)
	slices.Sort(p.targets)
	p.targets = slices.Compact(p.targets)
	dirs := append(slices.Clone(p.path), filepath.Join(p.env.CargoHome, "bin"))
	exe := func(name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return firstExecutable(dirs, name)
	}
	cargo, rustc, analyzer := exe("cargo"), exe("rustc"), exe("rust-analyzer")
	// A pinned managed rust-analyzer is the only server live analysis may run, and
	// it must win even when an earlier reviewed directory holds an unpinned copy.
	// The unpinned result stays as a fallback so discovery identity and tests still
	// describe it; the live registration applies the pin itself.
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) || hasParent(dir) {
			continue
		}
		candidate := filepath.Join(dir, "rust-analyzer")
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if managedtools.IsPinnedRustAnalyzer(candidate) {
			analyzer = candidate
			break
		}
	}
	proxy := func(path string) bool {
		if path == "" {
			return false
		}
		// rustup owns Cargo/rustc names in CARGO_HOME/bin; treat them as proxies
		// even if hardlink identity cannot be compared.
		if filepath.Dir(path) == filepath.Join(p.env.CargoHome, "bin") && strings.TrimSuffix(filepath.Base(path), ".exe") != "rust-analyzer" {
			return true
		}
		if strings.TrimSuffix(filepath.Base(path), ".exe") == "rustup" {
			return true
		}
		st, err := os.Stat(path)
		if err != nil {
			return true
		}
		for _, d := range dirs {
			candidate := filepath.Join(d, "rustup")
			if runtime.GOOS == "windows" {
				candidate += ".exe"
			}
			s, err := os.Stat(candidate)
			if err == nil && os.SameFile(st, s) {
				return true
			}
		}
		return false
	}
	if selected.name != "" {
		if _, err := rustToolchainSelection([]byte(selected.name), false); err != nil {
			return err
		}
		triple := rustHostTriple()
		if triple == "" {
			return failure("unsupported", "Rust host platform is unsupported")
		}
		// Exact installed names are accepted; a channel shorthand only maps to the
		// executing host's known rustup directory, never to an arbitrary glob match.
		names := []string{selected.name}
		if !strings.HasSuffix(selected.name, "-"+triple) {
			names = append(names, selected.name+"-"+triple)
		}
		toolDir := ""
		for _, name := range names {
			candidate := filepath.Join(p.env.RustupHome, "toolchains", name)
			st, err := os.Lstat(candidate)
			if err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
				if toolDir != "" {
					return failure("unsupported", "Rust toolchain selection matches multiple installed directories")
				}
				toolDir, err = filepath.EvalSymlinks(candidate)
				if err != nil {
					return failure("unsupported", "Rust toolchain directory cannot be resolved")
				}
			}
		}
		p.toolDir = toolDir
		if toolDir == "" {
			cargo = ""
			rustc = ""
			if proxy(analyzer) {
				analyzer = ""
			}
		} else {
			component := func(name string) string {
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				path := filepath.Join(toolDir, "bin", name)
				if !isExecutable(path) {
					return ""
				}
				real, err := filepath.EvalSymlinks(path)
				if err != nil {
					return ""
				}
				rel, err := filepath.Rel(toolDir, real)
				if err != nil || !filepath.IsLocal(rel) {
					return ""
				}
				return real
			}
			cargo, rustc = component("cargo"), component("rustc")
			if analyzer == "" || proxy(analyzer) {
				analyzer = component("rust-analyzer")
			}
		}
	} else {
		if proxy(cargo) {
			cargo = ""
		}
		if proxy(rustc) {
			rustc = ""
		}
		if proxy(analyzer) {
			analyzer = ""
		}
	}
	// Explicit installations must have a paired compiler: mixing unrelated PATH
	// toolchains is not a verifiable concrete selection.
	if cargo != "" && rustc != "" && filepath.Dir(cargo) != filepath.Dir(rustc) {
		return failure("unsupported", "Cargo and rustc must belong to the same concrete installed toolchain")
	}
	p.cargo, p.rustc, p.analyzer = cargo, rustc, analyzer
	// rustdoc and the sysroot proc-macro server are loaded by the approved
	// toolchain/server, so their presence and bytes are part of its identity.
	if rustc != "" {
		suffix := ""
		if runtime.GOOS == "windows" {
			suffix = ".exe"
		}
		sysroot := filepath.Dir(filepath.Dir(rustc))
		for _, path := range []string{filepath.Join(filepath.Dir(rustc), "rustdoc"+suffix), filepath.Join(sysroot, "libexec", "rust-analyzer-proc-macro-srv"+suffix)} {
			if !isExecutable(path) {
				continue
			}
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				return failure("unsupported", "Rust toolchain component cannot be resolved")
			}
			if strings.HasSuffix(real, "rustdoc"+suffix) {
				p.rustdoc = real
			} else {
				p.procMacro = real
			}
			p.hostInputs = append(p.hostInputs, real)
		}
	}
	return nil
}

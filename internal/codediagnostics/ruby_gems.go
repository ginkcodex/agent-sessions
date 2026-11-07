package codediagnostics

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxGemfileLock   = 2 << 20
	maxBundleConfig  = 64 << 10
	maxGemDirEntries = 8192
)

// gemLockfile is what discovery reads from Gemfile.lock: the top-level specs
// of each source section, RUBY VERSION and BUNDLED WITH. Dependency lines
// (six-space indent) and every other section are ignored.
type gemLockfile struct {
	gems        map[string]lockedGem // first entry per name
	rubyVersion string               // "ruby 3.3.5p100", unnormalized
	bundledWith string
}

// lockedGem is one top-level spec. source is "GEM" (a rubygems source,
// installed under a gem directory), "GIT" or "PATH".
type lockedGem struct {
	version, platform, source string
}

var (
	lockSpecLine    = regexp.MustCompile(`^    ([A-Za-z0-9][A-Za-z0-9._-]*) \(([^()\s]+)\)$`)
	gemVersionShape = regexp.MustCompile(`^[0-9]+(\.[0-9A-Za-z]+){0,7}$`)
)

// readGemfileLock parses a bounded Gemfile.lock. present is false when the
// file does not exist; ok is false when it exists but is not a lockfile
// (oversized, a link leaving boundary, conflict markers, a malformed spec).
func readGemfileLock(file, boundary string) (lock gemLockfile, present, ok bool) {
	data, present, ok := readBoundedFile(file, boundary, maxGemfileLock)
	if !present || !ok {
		return gemLockfile{}, present, false
	}
	lock, ok = parseGemfileLock(string(data))
	return lock, true, ok
}

func parseGemfileLock(data string) (gemLockfile, bool) {
	lock := gemLockfile{gems: map[string]lockedGem{}}
	section, inSpecs, sections := "", false, 0
	for line := range strings.SplitSeq(strings.ReplaceAll(data, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "<<<<<<<") || strings.HasPrefix(line, ">>>>>>>") || line == "=======" {
			return gemLockfile{}, false
		}
		if line == "" {
			continue
		}
		if line[0] != ' ' {
			section, inSpecs = line, false
			sections++
			continue
		}
		switch section {
		case "GEM", "GIT", "PATH":
			if line == "  specs:" {
				inSpecs = true
				continue
			}
			if !inSpecs || strings.HasPrefix(line, "      ") {
				continue
			}
			m := lockSpecLine.FindStringSubmatch(line)
			if m == nil {
				return gemLockfile{}, false
			}
			version, platform, _ := strings.Cut(m[2], "-")
			if !gemVersionShape.MatchString(version) {
				return gemLockfile{}, false
			}
			if _, seen := lock.gems[m[1]]; !seen {
				lock.gems[m[1]] = lockedGem{version, platform, section}
			}
		case "RUBY VERSION":
			if lock.rubyVersion != "" {
				return gemLockfile{}, false
			}
			lock.rubyVersion = strings.TrimSpace(line)
		case "BUNDLED WITH":
			v := strings.TrimSpace(line)
			if lock.bundledWith != "" || !gemVersionShape.MatchString(v) {
				return gemLockfile{}, false
			}
			lock.bundledWith = v
		}
	}
	return lock, sections > 0
}

// readBoundedFile reads a regular file whose real path stays inside boundary.
func readBoundedFile(file, boundary string, limit int64) (data []byte, present, ok bool) {
	if _, err := os.Lstat(file); err != nil {
		return nil, false, false
	}
	realBoundary, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		return nil, true, false
	}
	real, err := filepath.EvalSymlinks(file)
	if err != nil || !within(realBoundary, real) {
		return nil, true, false
	}
	st, err := os.Stat(real)
	if err != nil || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, true, false
	}
	data, err = readLimited(real, limit)
	return data, true, err == nil
}

// bundleSetting reads one top-level key of a Bundler config file: the
// "KEY: value" lines Bundler writes, with an optionally quoted scalar. No
// general YAML is parsed; any other shape for that key makes ok false.
func bundleSetting(data []byte, key string) (value string, found, ok bool) {
	for line := range strings.SplitSeq(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		k, v, cut := strings.Cut(line, ":")
		if !cut || strings.TrimSpace(k) != key || strings.HasPrefix(line, " ") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		} else if strings.ContainsAny(v, "\"'#{}[]&*!|>%@`") {
			return "", true, false
		}
		if v == "" || !bounded(v, 4096) || strings.ContainsAny(v, "\x00\n") {
			return "", true, false
		}
		return v, true, true
	}
	return "", false, true
}

// rubyGem is an installed gem identified by files only.
type rubyGem struct {
	Name, Version string
	Home          string // the gem directory (GEM_HOME layout) holding it
	Path          string // Home/gems/<name>-<version>[-<platform>]
	Entry         string // the file the launch loads, inside Path
	Spec          string // Home/specifications/<dir>.gemspec
}

// gemEntry is the file each gem is launched or required through.
func gemEntry(name string) string {
	switch name {
	case "bundler":
		return filepath.Join("lib", "bundler", "setup.rb")
	case rubyLspGem:
		// The app's wrapper requires ruby_lsp/internal; exe/ruby-lsp never runs.
		return filepath.Join("lib", "ruby_lsp", "internal.rb")
	}
	return filepath.Join("exe", name)
}

// findGem returns the gem at exactly version from the first directory that
// has it; newestGem the highest version across dirs (the earlier directory
// on a tie). Both require the entry file and the gemspec as regular files
// whose real paths stay inside the gem directory.
func findGem(dirs []string, name, version string) (rubyGem, bool) {
	for _, dir := range dirs {
		for _, g := range installedGems(dir, name) {
			if g.Version == version {
				return g, true
			}
		}
	}
	return rubyGem{}, false
}

func newestGem(dirs []string, name string) (rubyGem, bool) {
	var best rubyGem
	found := false
	for _, dir := range dirs {
		for _, g := range installedGems(dir, name) {
			// A release beats any prerelease, as RubyGems activates them.
			newer := compareGemVersions(g.Version, best.Version) > 0
			if pre, bestPre := prereleaseGem(g.Version), prereleaseGem(best.Version); pre != bestPre {
				newer = bestPre
			}
			if !found || newer {
				best, found = g, true
			}
		}
	}
	return best, found
}

func prereleaseGem(v string) bool { return strings.ContainsFunc(v, unicode.IsLetter) }

func installedGems(dir, name string) []rubyGem {
	home, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil
	}
	f, err := os.Open(filepath.Join(home, "gems"))
	if err != nil {
		return nil
	}
	entries, _ := f.ReadDir(maxGemDirEntries)
	_ = f.Close()
	var out []rubyGem
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), name+"-")
		if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' {
			continue
		}
		version, _, _ := strings.Cut(rest, "-")
		if !gemVersionShape.MatchString(version) {
			continue
		}
		g := rubyGem{Name: name, Version: version, Home: home, Path: filepath.Join(home, "gems", e.Name()),
			Spec: filepath.Join(home, "specifications", e.Name()+".gemspec")}
		g.Entry = filepath.Join(g.Path, gemEntry(name))
		if regularWithin(home, g.Entry) && regularWithin(home, g.Spec) {
			out = append(out, g)
		}
	}
	return out
}

// regularWithin reports that file is a regular file whose real path is file
// itself or another path inside base.
func regularWithin(base, file string) bool {
	real, err := filepath.EvalSymlinks(file)
	if err != nil || !within(base, real) {
		return false
	}
	st, err := os.Stat(real)
	return err == nil && st.Mode().IsRegular()
}

// compareGemVersions orders RubyGems versions: numeric segments numerically,
// a prerelease (letter) segment before any numeric one.
func compareGemVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		x, y := "0", "0"
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		switch {
		case xerr == nil && yerr == nil:
			if xn != yn {
				return xn - yn
			}
		case xerr == nil:
			return 1
		case yerr == nil:
			return -1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return 0
}

// defaultBundler finds the Bundler that ships with the interpreter: its
// gemspec in specifications/default and lib/ruby/<abi>/bundler/setup.rb.
func defaultBundler(rt rubyRuntime) (rubyGem, bool) {
	home := filepath.Join(rt.Prefix, "lib", "ruby", "gems", rt.ABI)
	real, err := filepath.EvalSymlinks(home)
	if err != nil {
		return rubyGem{}, false
	}
	f, err := os.Open(filepath.Join(real, "specifications", "default"))
	if err != nil {
		return rubyGem{}, false
	}
	entries, _ := f.ReadDir(maxGemDirEntries)
	_ = f.Close()
	entry := filepath.Join(rt.Prefix, "lib", "ruby", rt.ABI, "bundler", "setup.rb")
	if !regularWithin(rt.Prefix, entry) {
		return rubyGem{}, false
	}
	var best rubyGem
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), "bundler-")
		v, ok2 := strings.CutSuffix(rest, ".gemspec")
		if !ok || !ok2 || !gemVersionShape.MatchString(v) {
			continue
		}
		spec := filepath.Join(real, "specifications", "default", e.Name())
		if regularWithin(real, spec) && (best.Version == "" || compareGemVersions(v, best.Version) > 0) {
			best = rubyGem{Name: "bundler", Version: v, Home: real, Path: filepath.Dir(filepath.Dir(entry)), Entry: entry, Spec: spec}
		}
	}
	return best, best.Version != ""
}

// rubocopInstall is how RuboCop runs for one workspace. State is "" or one of
// missing (no RuboCop gem), notInstalled (the locked version is absent),
// unsupportedSource (locked from a git or path source), invalidLockfile,
// invalidBundleConfig, bundlePathOutside (a relative BUNDLE_PATH leaving the
// workspace) and missingBundler.
type rubocopInstall struct {
	Mode          string // "bundler" or "standalone"
	Rubocop       rubyGem
	Bundler       rubyGem // Bundler mode only
	BundlerEnv    string  // BUNDLER_VERSION to set, "" for none
	GemDirs       []string
	LockedVersion string
	BundlePath    string // canonical BUNDLE_PATH/ruby/<abi>, "" when unset
	State         string
}

// findRubocop picks Bundler mode when the workspace's Gemfile.lock locks
// rubocop, else standalone mode. workspace is canonical; lock is the parsed
// workspace lockfile (lockPresent false when there is none, lockOK false
// when it was unreadable).
func findRubocop(workspace string, lock gemLockfile, lockPresent, lockOK bool, rt rubyRuntime, env RubyEnvironment) rubocopInstall {
	if lockPresent && !lockOK {
		return rubocopInstall{State: "invalidLockfile"}
	}
	interpreterGems := filepath.Join(rt.Prefix, "lib", "ruby", "gems", rt.ABI)
	user := userGemDirs(rt, env)
	locked, isLocked := lock.gems["rubocop"]
	if !lockPresent || !isLocked {
		dirs := existingDirs(append(user, interpreterGems))
		g, ok := newestGem(dirs, "rubocop")
		if !ok {
			return rubocopInstall{Mode: "standalone", State: "missing"}
		}
		return rubocopInstall{Mode: "standalone", Rubocop: g, GemDirs: dirs}
	}
	inst := rubocopInstall{Mode: "bundler", LockedVersion: locked.version}
	if locked.source != "GEM" {
		inst.State = "unsupportedSource"
		return inst
	}
	bundlePath, state := configuredBundlePath(workspace, rt, env)
	if state != "" {
		inst.State = state
		return inst
	}
	// With a configured path Bundler installs every gem there.
	dirs := existingDirs(append(user, interpreterGems))
	if bundlePath != "" {
		inst.BundlePath = bundlePath
		dirs = existingDirs([]string{bundlePath})
	}
	inst.GemDirs = dirs
	g, ok := findGem(dirs, "rubocop", locked.version)
	if !ok {
		inst.State = "notInstalled"
		return inst
	}
	inst.Rubocop = g
	// Bundler itself comes from the interpreter's gem directories.
	bundlerDirs := existingDirs(append(user, interpreterGems))
	newest, hasNewest := newestGem(bundlerDirs, "bundler")
	if def, ok := defaultBundler(rt); ok && (!hasNewest || compareGemVersions(def.Version, newest.Version) > 0) {
		newest, hasNewest = def, true
	}
	if !hasNewest {
		inst.State = "missingBundler"
		return inst
	}
	inst.Bundler = newest
	if want := lock.bundledWith; want != "" && want != newest.Version {
		if b, ok := findGem(bundlerDirs, "bundler", want); ok {
			inst.Bundler, inst.BundlerEnv = b, want
		} else if def, ok := defaultBundler(rt); ok && def.Version == want {
			inst.Bundler, inst.BundlerEnv = def, want
		}
	}
	return inst
}

// userGemDirs are RubyGems' user install directories for this interpreter:
// ~/.gem/ruby/<abi> (or its XDG form), and the per-version GEM_HOME that
// chruby (~/.gem/ruby/<version>) and rvm (~/.rvm/gems/ruby-<version>) set.
func userGemDirs(rt rubyRuntime, env RubyEnvironment) []string {
	if !filepath.IsAbs(env.Home) || hasParent(env.Home) {
		return nil
	}
	dirs := []string{
		filepath.Join(env.Home, ".gem", "ruby", rt.ABI),
		filepath.Join(env.Home, ".local", "share", "gem", "ruby", rt.ABI),
	}
	if rt.Version != rt.ABI {
		dirs = append(dirs, filepath.Join(env.Home, ".gem", "ruby", rt.Version))
	}
	return append(dirs, filepath.Join(env.Home, ".rvm", "gems", "ruby-"+rt.Version))
}

func existingDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		real, err := filepath.EvalSymlinks(d)
		if err != nil || slices.Contains(out, real) {
			continue
		}
		if st, err := os.Stat(real); err == nil && st.IsDir() {
			out = append(out, real)
		}
	}
	return out
}

// configuredBundlePath reads BUNDLE_PATH (or BUNDLE_DEPLOYMENT's default
// vendor/bundle) from the workspace's .bundle/config, then the user's
// ~/.bundle/config, and returns <path>/ruby/<abi> canonically. A relative
// project path must stay inside the workspace; a relative user path is
// ignored, as Bundler resolves it against the current directory.
func configuredBundlePath(workspace string, rt rubyRuntime, env RubyEnvironment) (string, string) {
	resolve := func(value, base string) (string, string) {
		if rest, ok := strings.CutPrefix(value, "~/"); ok && filepath.IsAbs(env.Home) {
			value = filepath.Join(env.Home, rest)
		}
		p := value
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
			if hasParent(value) || !within(workspace, filepath.Clean(p)) {
				return "", "bundlePathOutside"
			}
			if real, err := filepath.EvalSymlinks(p); err == nil && !within(workspace, real) {
				return "", "bundlePathOutside"
			}
		}
		return filepath.Join(p, "ruby", rt.ABI), ""
	}
	data, present, ok := readBoundedFile(filepath.Join(workspace, ".bundle", "config"), workspace, maxBundleConfig)
	if present && !ok {
		return "", "invalidBundleConfig"
	}
	if present {
		value, found, ok := bundleSetting(data, "BUNDLE_PATH")
		if !ok {
			return "", "invalidBundleConfig"
		}
		if found {
			return resolve(value, workspace)
		}
		if deploy, found, _ := bundleSetting(data, "BUNDLE_DEPLOYMENT"); found && deploy == "true" {
			return resolve("vendor/bundle", workspace)
		}
	}
	if filepath.IsAbs(env.Home) && !hasParent(env.Home) {
		userConfig := filepath.Join(env.Home, ".bundle", "config")
		if data, present, ok := readBoundedFile(userConfig, filepath.Dir(userConfig), maxBundleConfig); present && ok {
			if value, found, ok := bundleSetting(data, "BUNDLE_PATH"); found && ok && (filepath.IsAbs(value) || strings.HasPrefix(value, "~/")) {
				return resolve(value, workspace)
			}
		}
	}
	return "", ""
}

const (
	rubyLspGem = "ruby-lsp"
	// rubyLspMinVersion is the oldest ruby-lsp observed with the app's wrapper
	// (RL-00, 0.26.11); rubyLspMinRuby its gemspec's required_ruby_version.
	rubyLspMinVersion = "0.26"
	rubyLspMinRuby    = "3.0"
	maxGemspec        = 256 << 10
	// maxRubyLspServer bounds lib/ruby_lsp/server.rb (57 KB in 0.26.11).
	maxRubyLspServer = 1 << 20
)

// gemspecDependency matches the runtime dependency lines RubyGems writes into
// an installed gemspec; add_development_dependency lines do not match.
var (
	gemspecDependency = regexp.MustCompile(`\badd_(?:runtime_)?dependency\(\s*(?:%q<([A-Za-z0-9][A-Za-z0-9._-]*)>|"([A-Za-z0-9][A-Za-z0-9._-]*)")`)
	// The app's wrapper (ruby_lsp.go) replaces RubyLsp::Server#load_addons; a
	// ruby-lsp whose server.rb no longer defines it is incompatible.
	rubyLspServerClass = regexp.MustCompile(`(?m)^\s*class Server\b`)
	rubyLspLoadAddons  = regexp.MustCompile(`(?m)^\s*def load_addons\b`)
)

// rubyLspInstall is the ruby-lsp found for one workspace. State is "" or one
// of missing, notInstalled (the locked version is absent), unsupportedSource,
// invalidLockfile, invalidBundleConfig, bundlePathOutside, tooOld (older
// than rubyLspMinVersion), rubyTooOld, incompatible (lib/ruby_lsp/server.rb
// does not define RubyLsp::Server#load_addons), invalidGemspec and
// missingDependency (Dependency names it).
type rubyLspInstall struct {
	RubyLsp       rubyGem
	Server        string   // lib/ruby_lsp/server.rb, which the wrapper patches
	GemDirs       []string // GEM_PATH, in lookup order
	LockedVersion string   // "" unless Gemfile.lock lists ruby-lsp
	BundlePath    string
	Dependency    string
	State         string
}

// findRubyLsp takes the version Gemfile.lock locks, from BUNDLE_PATH or the
// user and interpreter gem directories, else the newest ruby-lsp installed in
// those directories. The server loads from GEM_PATH, never through the
// project bundle, so GemDirs always ends with the user and interpreter
// directories (bundled gems such as rbs live there). Each runtime dependency
// named in the gemspec must be installed by name in GemDirs or be a default
// gem; versions are not checked.
func findRubyLsp(workspace string, lock gemLockfile, lockPresent, lockOK bool, rt rubyRuntime, env RubyEnvironment) rubyLspInstall {
	if compareGemVersions(rt.Version, rubyLspMinRuby) < 0 {
		return rubyLspInstall{State: "rubyTooOld"}
	}
	if lockPresent && !lockOK {
		return rubyLspInstall{State: "invalidLockfile"}
	}
	interpreterGems := filepath.Join(rt.Prefix, "lib", "ruby", "gems", rt.ABI)
	shared := append(userGemDirs(rt, env), interpreterGems)
	var inst rubyLspInstall
	locked, isLocked := lock.gems[rubyLspGem]
	if lockPresent && isLocked {
		inst.LockedVersion = locked.version
		if locked.source != "GEM" {
			inst.State = "unsupportedSource"
			return inst
		}
		bundlePath, state := configuredBundlePath(workspace, rt, env)
		if state != "" {
			inst.State = state
			return inst
		}
		inst.BundlePath = bundlePath
		dirs := shared
		if bundlePath != "" {
			dirs = append([]string{bundlePath}, shared...)
		}
		inst.GemDirs = existingDirs(dirs)
		g, ok := findGem(inst.GemDirs, rubyLspGem, locked.version)
		if !ok {
			inst.State = "notInstalled"
			return inst
		}
		inst.RubyLsp = g
	} else {
		inst.GemDirs = existingDirs(shared)
		g, ok := newestGem(inst.GemDirs, rubyLspGem)
		if !ok {
			inst.State = "missing"
			return inst
		}
		inst.RubyLsp = g
	}
	if compareGemVersions(inst.RubyLsp.Version, rubyLspMinVersion) < 0 {
		inst.State = "tooOld"
		return inst
	}
	server := filepath.Join(inst.RubyLsp.Path, "lib", "ruby_lsp", "server.rb")
	if data, _, ok := readBoundedFile(server, inst.RubyLsp.Path, maxRubyLspServer); !ok || !rubyLspServerClass.Match(data) || !rubyLspLoadAddons.Match(data) {
		inst.State = "incompatible"
		return inst
	}
	inst.Server = server
	deps, ok := gemspecDependencies(inst.RubyLsp)
	if !ok {
		inst.State = "invalidGemspec"
		return inst
	}
	defaults := filepath.Join(interpreterGems, "specifications", "default")
	for _, name := range deps {
		if !gemPresent(inst.GemDirs, defaults, name) {
			inst.State, inst.Dependency = "missingDependency", name
			return inst
		}
	}
	return inst
}

// gemspecDependencies reads the runtime dependency names from an installed
// gemspec (bounded, inside the gem directory), sorted and unique.
func gemspecDependencies(g rubyGem) ([]string, bool) {
	data, present, ok := readBoundedFile(g.Spec, g.Home, maxGemspec)
	if !present || !ok {
		return nil, false
	}
	var names []string
	for _, m := range gemspecDependency.FindAllStringSubmatch(string(data), -1) {
		name := m[1] + m[2]
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, true
}

// gemPresent reports a gemspec for name, any version and platform, in one of
// dirs (specifications/<name>-<v>.gemspec) or among the interpreter's default
// gems.
func gemPresent(dirs []string, defaults, name string) bool {
	for _, specs := range append(gemSpecDirs(dirs), defaults) {
		real, err := filepath.EvalSymlinks(specs)
		if err != nil {
			continue
		}
		f, err := os.Open(real)
		if err != nil {
			continue
		}
		entries, _ := f.ReadDir(maxGemDirEntries)
		_ = f.Close()
		for _, e := range entries {
			rest, ok := strings.CutPrefix(e.Name(), name+"-")
			if !ok || rest == "" || rest[0] < '0' || rest[0] > '9' || !strings.HasSuffix(rest, ".gemspec") {
				continue
			}
			version, _, _ := strings.Cut(strings.TrimSuffix(rest, ".gemspec"), "-")
			if gemVersionShape.MatchString(version) && regularWithin(real, filepath.Join(real, e.Name())) {
				return true
			}
		}
	}
	return false
}

func gemSpecDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, filepath.Join(d, "specifications"))
	}
	return out
}

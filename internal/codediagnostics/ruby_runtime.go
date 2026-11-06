package codediagnostics

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// RubyEnvironment is the reviewed host environment for Ruby discovery. Home is
// required; the manager roots are the RBENV_ROOT, MISE_DATA_DIR and
// ASDF_DATA_DIR values of the executing host, honored only when absolute.
type RubyEnvironment struct {
	Home        string
	RbenvRoot   string
	MiseDataDir string
	AsdfDataDir string
}

// minRubyMajor/minRubyMinor: RuboCop's own floor; macOS's /usr/bin/ruby (2.6)
// is below it.
const (
	minRubyMajor, minRubyMinor = 2, 7
	maxRubyVersionFile         = 64 << 10
	maxRbConfig                = 1 << 20
	maxRubyInstallEntries      = 512
)

// rubySystemRubies is chruby's system-wide install directory (a variable so
// tests can point it into a temp tree).
var rubySystemRubies = "/opt/rubies"

// rubyRuntime is the interpreter discovery picked, identified from metadata
// only. State is "" (usable), "missing" (no ruby anywhere), "notInstalled"
// (the requested version is not installed; Request and RequestFile name it),
// "tooOld", "foreign" (only shims, scripts or unidentifiable installs) or
// "invalidRequest" (a version file exists but cannot be used).
type rubyRuntime struct {
	Interpreter string // real path of the native interpreter
	Prefix      string // its installation prefix (the parent of bin)
	Version     string // RUBY_PROGRAM_VERSION parts from rbconfig.rb
	ABI         string // ruby_version, the gem directory name (3.3.0)
	Request     string // normalized requested version, "" with no request
	RequestFile string // absolute path of the file that made the request
	ShimSkipped bool   // a version-manager shim or script preceded the PATH ruby
	State       string
}

// rubyRequest is a normalized version request. locked requests come from
// Gemfile.lock and also accept any installed patch release of the same minor
// version, whose gems share the ABI directory.
type rubyRequest struct {
	version, file string
	locked        bool
}

// resolveRuby picks the interpreter a developer's shell would, without running
// anything. dirs are canonical absolute directories, the workspace first and
// then its ancestors, the last being the authorized boundary that version-file
// links must stay inside. lockedRuby is Gemfile.lock's RUBY VERSION ("" when
// absent) for the workspace's dirs[0]/Gemfile.lock.
func resolveRuby(dirs []string, lockedRuby string, reviewedPath []string, env RubyEnvironment) rubyRuntime {
	req, ok := findRubyRequest(dirs, lockedRuby)
	if !ok {
		return rubyRuntime{Request: req.version, RequestFile: req.file, State: "invalidRequest"}
	}
	if req.version == "" {
		return rubyFromPath(reviewedPath)
	}
	rt := rubyRuntime{Request: req.version, RequestFile: req.file, State: "notInstalled"}
	roots := rubyInstallRoots(env)
	pick := func(prefix string) bool {
		got, state := inspectRubyPrefix(prefix)
		if state == "" && !rubyVersionMatches(got.Version, req) {
			state = "foreign" // the directory name does not describe its contents
		}
		if state == "foreign" {
			return false
		}
		got.Request, got.RequestFile, got.State = rt.Request, rt.RequestFile, state
		rt = got
		return true
	}
	for _, r := range roots {
		if prefix := r.exact(req.version); prefix != "" && pick(prefix) {
			return rt
		}
	}
	partial := req.version
	if req.locked {
		partial = majorMinor(req.version)
	}
	for _, r := range roots {
		for _, prefix := range r.matching(partial) {
			if pick(prefix) {
				return rt
			}
		}
	}
	return rt
}

// findRubyRequest walks dirs nearest first. In each directory .ruby-version
// wins over the ruby entry of mise.toml, .mise.toml and .tool-versions; a
// lockfile request applies only when no directory has one. ok is false when a
// version file exists but cannot be read as a plain version.
func findRubyRequest(dirs []string, lockedRuby string) (rubyRequest, bool) {
	if len(dirs) == 0 {
		return rubyRequest{}, true
	}
	boundary := dirs[len(dirs)-1]
	for _, dir := range dirs {
		for _, name := range []string{".ruby-version", "mise.toml", ".mise.toml", ".tool-versions"} {
			file := filepath.Join(dir, name)
			data, present, ok := readBoundedFile(file, boundary, maxRubyVersionFile)
			if !present {
				continue
			}
			if !ok {
				return rubyRequest{file: file}, false
			}
			var raw string
			var found bool
			switch name {
			case ".ruby-version":
				raw, found = firstField(data), true
			case ".tool-versions":
				raw, found = toolVersionsRuby(data)
			default:
				if raw, found, ok = miseTOMLRuby(data); !ok {
					return rubyRequest{file: file}, false
				}
			}
			if !found {
				continue
			}
			v, ok := normalizeRubyVersion(raw)
			if !ok {
				return rubyRequest{file: file}, false
			}
			if v == "system" {
				return rubyRequest{}, true // rbenv/asdf: use the system ruby on PATH
			}
			return rubyRequest{version: v, file: file}, true
		}
	}
	if lockedRuby != "" {
		file := filepath.Join(dirs[0], "Gemfile.lock")
		v, ok := normalizeRubyVersion(lockedRuby)
		if !ok || !numericVersion(v) {
			return rubyRequest{file: file}, false
		}
		return rubyRequest{version: v, file: file, locked: true}, true
	}
	return rubyRequest{}, true
}

func readLimited(file string, limit int64) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, failure("invalidArgument", "file too large")
	}
	return data, nil
}

// within reports whether real is base or below it.
func within(base, real string) bool {
	rel, err := filepath.Rel(base, real)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// firstField is the first word of the first line that is not blank or a
// comment, as rbenv and chruby read .ruby-version.
func firstField(data []byte) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if f := strings.Fields(line); len(f) > 0 {
			return f[0]
		}
	}
	return ""
}

// toolVersionsRuby reads asdf's "ruby <version> [fallback...]" line.
func toolVersionsRuby(data []byte) (string, bool) {
	for line := range strings.SplitSeq(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		if f := strings.Fields(line); len(f) >= 1 && f[0] == "ruby" {
			if len(f) < 2 {
				return "", true
			}
			return f[1], true
		}
	}
	return "", false
}

var miseRubyLine = regexp.MustCompile(`^(?:ruby|"ruby"|'ruby')\s*=\s*(.*)$`)

// miseTOMLRuby reads only the ruby key of the [tools] table: a quoted string
// value. Other value shapes (arrays, inline tables) make ok false; no general
// TOML is parsed.
func miseTOMLRuby(data []byte) (value string, found, ok bool) {
	inTools := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			if i := strings.IndexByte(line, '#'); i >= 0 {
				line = strings.TrimSpace(line[:i])
			}
			inTools = line == "[tools]"
			continue
		}
		if !inTools {
			continue
		}
		m := miseRubyLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := m[1]
		if len(rest) < 2 || (rest[0] != '"' && rest[0] != '\'') {
			return "", true, false
		}
		end := strings.IndexByte(rest[1:], rest[0])
		if end < 0 {
			return "", true, false
		}
		if tail := strings.TrimSpace(rest[end+2:]); tail != "" && !strings.HasPrefix(tail, "#") {
			return "", true, false
		}
		return rest[1 : end+1], true, true
	}
	return "", false, true
}

var (
	rubyVersionPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+){0,3}(-(preview|rc)[0-9]+)?$`)
	rubyPatchLevel     = regexp.MustCompile(`-?p[0-9]+$`)
)

// normalizeRubyVersion strips a "ruby-" or "ruby " prefix and a patch-level
// suffix (3.3.5p100, 2.7.8-p225). "system" and "latest" pass through; other
// implementations (jruby-, truffleruby-) and anything that is not a plain
// version are rejected.
func normalizeRubyVersion(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	v = strings.TrimPrefix(v, "ruby-")
	v = strings.TrimSpace(strings.TrimPrefix(v, "ruby "))
	if v == "system" || v == "latest" {
		return v, true
	}
	v = rubyPatchLevel.ReplaceAllString(v, "")
	if len(v) > 32 || !rubyVersionPattern.MatchString(v) {
		return "", false
	}
	return v, true
}

func numericVersion(v string) bool {
	return v != "" && !strings.Contains(v, "-") && rubyVersionPattern.MatchString(v)
}

func majorMinor(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// rubyVersionMatches reports whether the installed version satisfies req. A
// symbolic request (latest, a mise alias) is not checked against contents.
func rubyVersionMatches(installed string, req rubyRequest) bool {
	want := req.version
	if req.locked {
		want = majorMinor(want)
	}
	if !numericVersion(want) {
		return true
	}
	return installed == want || strings.HasPrefix(installed, want+".")
}

// compareRubyVersions orders numeric versions segment by segment.
func compareRubyVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(as), len(bs)); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}
	return len(as) - len(bs)
}

// rubyInstallRoot is one version manager's directory of installed Rubies,
// whose entries are named prefix+version.
type rubyInstallRoot struct{ dir, prefix string }

func rubyInstallRoots(env RubyEnvironment) []rubyInstallRoot {
	usable := func(dir string) bool { return filepath.IsAbs(dir) && bounded(dir, 4096) && !hasParent(dir) }
	or := func(dir, fallback string) string {
		if usable(dir) {
			return dir
		}
		return fallback
	}
	var roots []rubyInstallRoot
	if usable(env.Home) {
		roots = append(roots,
			rubyInstallRoot{filepath.Join(or(env.RbenvRoot, filepath.Join(env.Home, ".rbenv")), "versions"), ""},
			rubyInstallRoot{filepath.Join(or(env.MiseDataDir, filepath.Join(env.Home, ".local", "share", "mise")), "installs", "ruby"), ""},
			rubyInstallRoot{filepath.Join(or(env.AsdfDataDir, filepath.Join(env.Home, ".asdf")), "installs", "ruby"), ""},
			rubyInstallRoot{filepath.Join(env.Home, ".rubies"), "ruby-"},
		)
	}
	roots = append(roots, rubyInstallRoot{rubySystemRubies, "ruby-"})
	if usable(env.Home) {
		roots = append(roots, rubyInstallRoot{filepath.Join(env.Home, ".rvm", "rubies"), "ruby-"})
	}
	return roots
}

// exact returns the canonical prefix of the entry named for version: a real
// directory, or a link (mise's 4 -> ./4.0.9, latest, global) that resolves to
// a sibling directory. Anything else is ignored.
func (r rubyInstallRoot) exact(version string) string {
	realRoot, err := filepath.EvalSymlinks(r.dir)
	if err != nil {
		return ""
	}
	entry := filepath.Join(realRoot, r.prefix+version)
	st, err := os.Lstat(entry)
	if err != nil {
		return ""
	}
	if st.IsDir() {
		return entry
	}
	if st.Mode()&os.ModeSymlink == 0 {
		return ""
	}
	real, err := filepath.EvalSymlinks(entry)
	if err != nil || filepath.Dir(real) != realRoot {
		return ""
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return ""
	}
	return real
}

// matching lists real (non-link) installs whose numeric version starts with
// partial, newest first; "latest" matches every numeric version.
func (r rubyInstallRoot) matching(partial string) []string {
	realRoot, err := filepath.EvalSymlinks(r.dir)
	if err != nil {
		return nil
	}
	f, err := os.Open(realRoot)
	if err != nil {
		return nil
	}
	entries, _ := f.ReadDir(maxRubyInstallEntries)
	_ = f.Close()
	type install struct{ version, dir string }
	var found []install
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), r.prefix) {
			continue
		}
		v, ok := normalizeRubyVersion(strings.TrimPrefix(e.Name(), r.prefix))
		if !ok || !numericVersion(v) {
			continue
		}
		if partial == "latest" || v == partial || strings.HasPrefix(v, partial+".") {
			found = append(found, install{v, filepath.Join(realRoot, e.Name())})
		}
	}
	slices.SortFunc(found, func(a, b install) int { return compareRubyVersions(b.version, a.version) })
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.dir
	}
	return out
}

// inspectRubyPrefix identifies <prefix>/bin/ruby, which must be a native
// executable inside prefix, and reads its rbconfig.rb. state is "", "tooOld"
// or "foreign".
func inspectRubyPrefix(prefix string) (rubyRuntime, string) {
	real, state := nativeRuby(filepath.Join(prefix, "bin", "ruby"))
	if state != "" || !within(prefix, real) {
		return rubyRuntime{}, "foreign"
	}
	return identifyRuby(real)
}

// identifyRuby reads the version of a native interpreter from its prefix.
func identifyRuby(real string) (rubyRuntime, string) {
	prefix := filepath.Dir(filepath.Dir(real))
	version, abi, ok := findRbConfig(prefix)
	if !ok {
		return rubyRuntime{}, "foreign"
	}
	rt := rubyRuntime{Interpreter: real, Prefix: prefix, Version: version, ABI: abi}
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	if major < minRubyMajor || major == minRubyMajor && minor < minRubyMinor {
		return rt, "tooOld"
	}
	return rt, ""
}

var rubyBinaryName = regexp.MustCompile(`^ruby([0-9]+(\.[0-9]+)*)?$`)

// nativeRuby resolves link and accepts only an executable named ruby (or
// Debian's ruby3.1) that is not a script. state is "" or "foreign"; a missing
// file is "missing".
func nativeRuby(link string) (string, string) {
	real, err := filepath.EvalSymlinks(link)
	if err != nil {
		return "", "missing"
	}
	if !rubyBinaryName.MatchString(filepath.Base(real)) || !isExecutable(real) || isScript(real) {
		return "", "foreign"
	}
	return real, ""
}

// rubyFromPath takes the first usable native ruby on the reviewed path. Shims
// (rbenv/asdf scripts, mise links to the mise binary) are skipped. A ruby that
// is too old is skipped too and reported only when nothing usable follows: a
// Finder-launched app inherits a PATH with macOS's /usr/bin (Ruby 2.6) before
// the login shell's version-manager directories.
func rubyFromPath(reviewedPath []string) rubyRuntime {
	state, shim := "missing", false
	var old *rubyRuntime
	for _, dir := range reviewedPath {
		if !filepath.IsAbs(dir) || hasParent(dir) {
			continue
		}
		real, s := nativeRuby(filepath.Join(dir, "ruby"))
		if s == "foreign" {
			state, shim = "foreign", true
		}
		if s != "" {
			continue
		}
		rt, s := identifyRuby(real)
		switch s {
		case "foreign":
			state = "foreign"
		case "tooOld":
			if old == nil {
				rt.State = s
				old = &rt
			}
		default:
			rt.ShimSkipped = shim
			return rt
		}
	}
	if old != nil {
		old.ShimSkipped = shim
		return *old
	}
	return rubyRuntime{State: state, ShimSkipped: shim}
}

// findRbConfig reads rbconfig.rb from <prefix>/lib/ruby/<abi>/<arch>/ or
// Debian's <prefix>/lib/<multiarch>/ruby/<abi>/. Exactly one consistent file
// must exist; links are followed (macOS's /usr/lib/ruby is one) because the
// file is only read.
func findRbConfig(prefix string) (version, abi string, ok bool) {
	lib := filepath.Join(prefix, "lib")
	var candidates []string
	for _, abiDir := range listDirs(filepath.Join(lib, "ruby")) {
		if !rubyABI.MatchString(abiDir) {
			continue
		}
		for _, arch := range listDirs(filepath.Join(lib, "ruby", abiDir)) {
			candidates = append(candidates, filepath.Join(lib, "ruby", abiDir, arch, "rbconfig.rb"))
		}
	}
	for _, multiarch := range listDirs(lib) {
		if multiarch == "ruby" {
			continue
		}
		for _, abiDir := range listDirs(filepath.Join(lib, multiarch, "ruby")) {
			if !rubyABI.MatchString(abiDir) {
				continue
			}
			candidates = append(candidates, filepath.Join(lib, multiarch, "ruby", abiDir, "rbconfig.rb"))
		}
	}
	found := 0
	for _, c := range candidates {
		st, err := os.Stat(c)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		v, a, good := parseRbConfig(c)
		if !good || filepath.Base(filepath.Dir(filepath.Dir(c))) != a && filepath.Base(filepath.Dir(c)) != a {
			return "", "", false
		}
		if found++; found > 1 && (v != version || a != abi) {
			return "", "", false
		}
		version, abi = v, a
	}
	return version, abi, found > 0
}

// listDirs returns up to maxRubyInstallEntries subdirectory names.
func listDirs(dir string) []string {
	f, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	entries, _ := f.ReadDir(maxRubyInstallEntries)
	var out []string
	for _, e := range entries {
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil && st.IsDir() {
				out = append(out, e.Name())
			}
		}
	}
	slices.Sort(out)
	return out
}

var (
	rbConfigLine   = regexp.MustCompile(`^\s*CONFIG\["(MAJOR|MINOR|TEENY|ruby_version)"\]\s*=\s*"([^"]*)"\s*$`)
	rbConfigDigits = regexp.MustCompile(`^[0-9]{1,4}$`)
	rubyABI        = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}$`)
)

// parseRbConfig reads the literal MAJOR, MINOR, TEENY and ruby_version
// assignments; RUBY_PROGRAM_VERSION itself is "$(MAJOR).$(MINOR).$(TEENY)".
func parseRbConfig(file string) (version, abi string, ok bool) {
	data, err := readLimited(file, maxRbConfig)
	if err != nil {
		return "", "", false
	}
	values := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), maxRbConfig)
	for sc.Scan() {
		if m := rbConfigLine.FindStringSubmatch(sc.Text()); m != nil {
			if _, dup := values[m[1]]; dup {
				return "", "", false
			}
			values[m[1]] = m[2]
		}
	}
	if sc.Err() != nil {
		return "", "", false
	}
	major, minor, teeny, abi := values["MAJOR"], values["MINOR"], values["TEENY"], values["ruby_version"]
	if !rbConfigDigits.MatchString(major) || !rbConfigDigits.MatchString(minor) || !rbConfigDigits.MatchString(teeny) || !rubyABI.MatchString(abi) || !strings.HasPrefix(abi, major+"."+minor+".") {
		return "", "", false
	}
	return major + "." + minor + "." + teeny, abi, true
}

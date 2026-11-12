package managedtools

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Discover is read-only: no directory creation, probes, network or repair. A
// receipt alone is never authority: validate the install AND its main file's
// bytes. Missing (os.IsNotExist) and corrupt installs are deliberately
// distinct. tool is a catalog name, including "node".
func (i *Installer) Discover(tool string) (string, error) {
	switch tool {
	case "gopls":
		// Any catalog release counts: the host Go decides which one a
		// proposal builds, and a later Go upgrade must not hide it.
		var first error
		for _, r := range goplsReleases {
			path, err := i.discover(goplsSpec(i, TrustedGo{}, r))
			if err == nil {
				return path, nil
			}
			if first == nil || (os.IsNotExist(first) && !os.IsNotExist(err)) {
				first = err
			}
		}
		return "", first
	case "ruff":
		return i.discover(ruffSpec())
	case "rust-analyzer":
		return i.discover(rustAnalyzerSpec())
	case "vtsls", "pyright", "svelte-language-server", "vue-typescript-plugin", "claude-agent-acp", "codex-acp":
		return i.discover(npmSpec(npmPackages[tool]()))
	case "node":
		return i.discover(nodeSpec())
	}
	return "", errors.New("managed tools: unknown tool")
}

// Pinned returns the version of tool, a chat adapter, that this app
// installs, or "" for another tool.
func (i *Installer) Pinned(tool string) string {
	if !slices.Contains(adapterTools, tool) {
		return ""
	}
	return npmPackages[tool]().version
}

// Adapter returns the main script and version of the app's copy of tool, a
// chat adapter: the pinned version, else the newest valid copy of another
// version, which an earlier app release installed. Read-only like Discover.
// A damaged pinned copy is reported, not passed over; damaged other copies
// are skipped, and none at all is os.ErrNotExist.
func (i *Installer) Adapter(tool string) (main, version string, err error) {
	if !slices.Contains(adapterTools, tool) {
		return "", "", errors.New("managed tools: unknown tool")
	}
	pkg := npmPackages[tool]()
	main, err = i.Discover(tool)
	if !os.IsNotExist(err) {
		return main, pkg.version, err
	}
	for _, v := range i.otherVersions(pkg) {
		if main, err := i.discover(npmSpec(otherVersion(pkg, v))); err == nil {
			return main, v, nil
		}
	}
	return "", "", os.ErrNotExist
}

// otherVersions lists the versions of pkg installed beside the pinned one
// for this platform, newest first. Only plain x.y.z versions count.
func (i *Installer) otherVersions(pkg npmPackage) []string {
	if readPrivateTree(i.root) != nil {
		return nil
	}
	entries, err := os.ReadDir(i.root)
	if err != nil {
		return nil
	}
	var out []string
	suffix := "-" + runtime.GOOS + "-" + runtime.GOARCH
	for _, e := range entries {
		v, ok := strings.CutPrefix(e.Name(), pkg.name+"-")
		if !ok {
			continue
		}
		if v, ok = strings.CutSuffix(v, suffix); ok && v != pkg.version && plainVersion(v) != nil {
			out = append(out, v)
		}
	}
	slices.SortFunc(out, func(a, b string) int { return slices.Compare(plainVersion(b), plainVersion(a)) })
	return out
}

// otherVersion is pkg as an install of version: the same layout and policy,
// with only the adapter's own package version checked, since that version's
// lockfile is not in this app. Its packages were checked against their
// hashes when it was installed.
func otherVersion(pkg npmPackage, version string) npmPackage {
	versions := map[string]string{}
	for name, v := range pkg.versions {
		if v == pkg.version && strings.HasPrefix(pkg.main, "node_modules/"+name+"/") {
			versions[name] = version
		}
	}
	pkg.version, pkg.versions = version, versions
	return pkg
}

// plainVersion parses an x.y.z version, or returns nil.
func plainVersion(v string) []int {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for k, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return nil
		}
		out[k] = n
	}
	return out
}

// OlderThan reports whether version a is older than b; both are x.y.z.
func OlderThan(a, b string) bool {
	va, vb := plainVersion(a), plainVersion(b)
	return va != nil && vb != nil && slices.Compare(va, vb) < 0
}

// Runtime returns the Node.js executable that runs tool, an npm entry: the
// host's when it is new enough for tool, else the app's own. Read-only like
// Discover; the packages have no native code, so any such Node.js runs them.
func (i *Installer) Runtime(tool string) (string, error) {
	pkg, ok := npmPackages[tool]
	if !ok {
		return "", errors.New("managed tools: unknown tool")
	}
	if prefix, _ := i.usableHostNode(pkg().nodeMajor()); prefix != "" {
		return filepath.Join(prefix, "bin", "node"), nil
	}
	return i.Discover("node")
}

func (i *Installer) discover(s spec) (string, error) {
	dir := filepath.Join(i.root, installDir(s.name, s.version))
	// Not installed is "missing" however the app data directory looks; only an
	// existing install is judged by its directories.
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return "", err
	}
	if err := readAppData(filepath.Dir(i.root)); err != nil {
		return "", err
	}
	for _, d := range []string{i.root, dir} {
		if err := readPrivateTree(d); err != nil {
			return "", err
		}
	}
	path, err := i.existing(s, dir)
	if err != nil {
		return "", errors.New("managed tools: invalid installation")
	}
	return path, nil
}

// IsPinnedRustAnalyzer reports whether path is the current host's exact pinned
// managed rust-analyzer installation. It checks the managed directory layout,
// private directories, executable and version/policy receipt with its SHA-256.
// Like Discover, it only reads metadata and bytes: no execution, network, writes
// or repair. Unrelated analyzers and symlink aliases do not qualify. This is not
// an execution approval; callers must still bind the executable to their policy.
func IsPinnedRustAnalyzer(path string) bool {
	if _, ok := rustAnalyzerArchives[platform()]; !ok || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	s := rustAnalyzerSpec()
	dir := filepath.Dir(path)
	root := filepath.Dir(dir)
	if filepath.Base(path) != s.main || filepath.Base(dir) != installDir(s.name, s.version) || filepath.Base(root) != "managed-tools" {
		return false
	}
	i := &Installer{root: root}
	got, err := i.discover(s)
	return err == nil && got == path
}

// Dirs returns the directories of valid installs that tool discovery searches
// after the user's own: gopls's, Ruff's and rust-analyzer's directories, npm servers'
// node_modules/.bin and Node.js's bin. The chat adapters are not listed: code
// diagnostics never runs them.
func (i *Installer) Dirs() []string {
	var dirs []string
	for _, tool := range []string{"gopls", "ruff", "rust-analyzer", "vtsls", "pyright", "svelte-language-server", "node"} {
		path, err := i.Discover(tool)
		if err != nil {
			continue
		}
		if pkg, ok := npmPackages[tool]; ok {
			// Main files live below the package root; command shims are in
			// node_modules/.bin rather than next to those main files.
			p := pkg()
			dirs = append(dirs, filepath.Join(i.root, installDir(p.name, p.version), "node_modules", ".bin"))
			continue
		}
		dirs = append(dirs, filepath.Dir(path))
	}
	return dirs
}

func readPrivateTree(path string) error {
	if err := noLinks(filepath.Dir(path)); err != nil {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0o077 != 0 {
		return errors.New("managed tools: unsafe installation directory")
	}
	return nil
}

// readAppData checks the app data directory holding managed-tools. It is
// shared with other app storage (the Linux desktop's web view creates it with
// the umask), so it may be readable by others but must be this account's and
// not writable by anyone else, who could otherwise replace managed-tools.
func readAppData(path string) error {
	if err := noLinks(filepath.Dir(path)); err != nil {
		return err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || st.Mode().Perm()&0o022 != 0 || !ownedByUser(st) {
		return errors.New("managed tools: unsafe app data directory")
	}
	return nil
}

// readBounded reads a regular file of at most limit bytes.
func readBounded(path string, limit int64) ([]byte, error) {
	if err := regularOrAbsent(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > limit {
		return nil, errors.New("invalid file")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errors.New("invalid file")
	}
	return data, nil
}

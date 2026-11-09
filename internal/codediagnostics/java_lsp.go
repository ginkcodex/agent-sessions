package codediagnostics

// JDT LS discovery reads metadata only. The installed jdtls wrapper is a location
// hint, never executed. Build import happens only in the approved Java process;
// approval is consent to project code execution, not a sandbox.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	javaProviderID         = "jdtls"
	javaPolicy             = "jdtls-v1:java25-native:shared-config-readonly:bounded-content-manifest-stat-revalidation:no-edits:no-commands:approved-build-import:source-file-navigation:"
	javaInstallHint        = "Install Eclipse JDT Language Server (jdtls) and a JDK 25 or newer, then refresh."
	javaScopeNotice        = "After execution approval, Java build import may execute Maven/Gradle plugins, wrappers and annotation processors and download dependencies. Project code is not sandboxed. Navigation supports source-file targets only; archive/decompiled jdt: targets are unsupported."
	javaOpenedNotice       = "Java diagnostics cover opened documents only; unopened files and external changes are not continuously synchronized."
	maxJavaIdentityEntries = 2048
)

var javaDocumentExtensions = []string{".java"}
var javaProjectMarkers = projectMarkers{
	configs: []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts", "gradle.properties", "gradle-wrapper.properties", "gradle-wrapper.jar", "mvnw", "mvnw.cmd", "gradlew", "gradlew.bat", ".classpath", ".project"},
	skipDir: []string{"target", ".gradle", "build", "vendor", "node_modules"},
	sources: javaDocumentExtensions,
}

// JavaEnvironment contains reviewed absolute host paths, not inherited variables.
// Cache is the app-owned cache base. JavaHome optionally pins a JDK installation.
type JavaEnvironment struct{ Home, Cache, JavaHome string }

type JavaPlan struct {
	root                                       AuthorizedRoot
	availability, notice, version, javaVersion string
	config                                     ServerConfig
	projects                                   []string
	partial                                    bool
}

var _ livePlan = (*JavaPlan)(nil)

func DiscoverJava(root AuthorizedRoot, reviewedPath []string, env JavaEnvironment) (*JavaPlan, error) {
	return discoverJavaWorkspace(root, reviewedPath, env, nil)
}

func discoverJavaWorkspace(root AuthorizedRoot, reviewedPath []string, env JavaEnvironment, workspace *tsWorkspace) (*JavaPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	p := &JavaPlan{root: root, availability: "available"}
	unavailable := func(state, notice string) (*JavaPlan, error) {
		p.availability, p.notice, p.partial = state, notice, true
		p.config = ServerConfig{}
		return p, nil
	}
	w, err := workspaceMetadata(workspace, func() (tsWorkspace, error) { return discoverProjectDirs(root, javaProjectMarkers) })
	if err != nil {
		return nil, err
	}
	p.partial = w.partial
	if !w.sources && len(w.configs) == 0 {
		return unavailable("unsupported", "No Java source or build marker was found in this project.")
	}
	valid := func(p string) bool { return filepath.IsAbs(p) && bounded(p, 4096) && !hasParent(p) }
	if env.Cache == "" && valid(env.Home) {
		env.Cache = filepath.Join(env.Home, ".cache", "agent-sessions")
	}
	if !valid(env.Home) || !valid(env.Cache) || env.JavaHome != "" && !valid(env.JavaHome) {
		return nil, failure("invalidArgument", "Reviewed absolute home, cache and optional Java home paths are required")
	}
	p.projects = []string{""}
	dist, found := resolveJavaDistribution(reviewedPath)
	if dist == "" {
		if found {
			return unavailable("unsupported", "The jdtls installation has no supported distribution layout. "+javaInstallHint)
		}
		return unavailable("missingTool", "JDT Language Server (jdtls) was not found. "+javaInstallHint)
	}
	platform := map[string]string{"linux": "linux", "darwin": "mac", "windows": "win"}[runtime.GOOS]
	shared := filepath.Join(dist, "config_"+platform)
	launcher, _ := filepath.Glob(filepath.Join(dist, "plugins", "org.eclipse.equinox.launcher_*.jar"))
	core, _ := filepath.Glob(filepath.Join(dist, "plugins", "org.eclipse.jdt.ls.core_*.jar"))
	if platform == "" || len(launcher) != 1 || len(core) != 1 || !javaRegular(filepath.Join(shared, "config.ini")) {
		return unavailable("unsupported", "The JDT LS installation is incomplete or ambiguous (launcher, core bundle or platform configuration). "+javaInstallHint)
	}
	p.version = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(core[0]), "org.eclipse.jdt.ls.core_"), ".jar")
	if !plainVersion(p.version) {
		return unavailable("unsupported", "The JDT LS core bundle version could not be verified.")
	}
	java, jdk := resolveJavaRuntime(env.JavaHome, reviewedPath)
	if java == "" {
		return unavailable("missingTool", "A native Java runtime was not found; scripts and version-manager shims are not executed. "+javaInstallHint)
	}
	release := filepath.Join(jdk, "release")
	major, version := javaReleaseVersion(release)
	p.javaVersion = version
	if version == "" || major < 25 {
		return unavailable("unsupported", "JDT LS requires Java 25 or newer, verified from the JDK release metadata. "+javaInstallHint)
	}
	jvm := filepath.Join(jdk, "lib", "server", "libjvm.so")
	jli := filepath.Join(jdk, "lib", "libjli.so")
	if runtime.GOOS == "darwin" {
		jvm, jli = filepath.Join(jdk, "lib", "server", "libjvm.dylib"), filepath.Join(jdk, "lib", "libjli.dylib")
	}
	if runtime.GOOS == "windows" {
		jvm, jli = filepath.Join(jdk, "bin", "server", "jvm.dll"), filepath.Join(jdk, "bin", "jli.dll")
	}
	if !javaRegular(jvm) || !javaRegular(jli) {
		return unavailable("unsupported", "The selected JDK is missing its native JVM or launcher library. "+javaInstallHint)
	}
	cache, err := javaCachePath(env.Cache)
	if err != nil || within(root.Project().Root, cache) || within(dist, cache) {
		return unavailable("unsupported", "Java needs an app cache outside the project and JDT LS distribution.")
	}
	key := sha256.Sum256([]byte(root.Project().HostID + "\x00" + root.Project().Root))
	base := filepath.Join(cache, "jdtls", hex.EncodeToString(key[:]))
	// Resolve existing ancestors without making directories; reject cache links
	// back into executable project/distribution content.
	base, err = javaCachePath(base)
	if err != nil || within(root.Project().Root, base) || within(dist, base) {
		return unavailable("unsupported", "The Java cache location is not usable outside the project and distribution.")
	}
	// JDT LS may import every build below its root, including builds outside a
	// filtered markerless-document ownership set. Bind the full contained config
	// set in that case as well, rather than trusting the ownership filter as a
	// restriction on what the approved importer can execute.
	if workspace != nil {
		full, err := discoverProjectDirs(root, javaProjectMarkers)
		if err != nil {
			return nil, err
		}
		w.configs = full.configs
		p.partial = p.partial || full.partial
	}
	for _, name := range []string{"configuration", "data"} {
		dir := filepath.Join(base, name)
		if real, err := javaCachePath(dir); err != nil || real != dir {
			return unavailable("unsupported", "Java cache data and configuration must not be redirected by symlinks.")
		}
	}
	configs, err := javaApprovalConfigs(root, w.configs)
	if err != nil {
		return unavailable("unsupported", "Java build configuration cannot be identified for approval.")
	}
	paths := []string{java, release, jvm, jli, launcher[0]}
	// Reserve project config bytes in the aggregate budget too; Fingerprint
	// reads their contents using rooted opens, not unrooted manifest reads.
	var configBytes int64
	for _, rel := range configs {
		file, err := root.ResolvePath(root.Project().HostID, rel)
		if err != nil {
			return unavailable("unsupported", "Java build configuration cannot be identified for approval.")
		}
		st, err := os.Stat(file)
		if err != nil || st.Size() > maxIdentityFile {
			return unavailable("unsupported", "Java build configuration exceeds approval limits.")
		}
		configBytes += st.Size()
	}
	manifest, err := javaDistributionManifest([]string{filepath.Join(dist, "plugins"), shared}, paths, configBytes)
	if err != nil {
		return unavailable("unsupported", "The JDT LS runtime or bundle identity exceeds the bounded approval limits or is incomplete.")
	}
	tools := []ToolInput{}
	for i, f := range paths[1:] {
		tools = append(tools, ToolInput{ProviderID: "jdtls-toolchain-" + strconv.Itoa(i), Executable: f, Policy: "jdtls-loaded-file-v1"})
	}
	settings := json.RawMessage(`{"java":{"autobuild":{"enabled":false},"import":{"maven":{"enabled":true},"gradle":{"enabled":true,"wrapper":{"enabled":true}}},"configuration":{"updateBuildConfiguration":"automatic"},"referencesCodeLens":{"enabled":false},"implementationsCodeLens":{"enabled":false},"signatureHelp":{"enabled":false},"format":{"enabled":false}}}`)
	options, _ := json.Marshal(map[string]any{"bundles": []string{}, "settings": json.RawMessage(settings), "extendedClientCapabilities": map[string]any{"classFileContentsSupport": false, "clientHoverProvider": false}})
	p.config = ServerConfig{
		ProviderID: javaProviderID, Executable: java,
		Args:          []string{"-Declipse.application=org.eclipse.jdt.ls.core.id1", "-Dosgi.bundles.defaultStartLevel=4", "-Declipse.product=org.eclipse.jdt.ls.core.product", "-Dosgi.checkConfiguration=true", "-Dosgi.sharedConfiguration.area=" + shared, "-Dosgi.sharedConfiguration.area.readOnly=true", "-Dosgi.configuration.cascaded=true", "-Xmx1G", "--add-modules=ALL-SYSTEM", "--add-opens", "java.base/java.util=ALL-UNNAMED", "--add-opens", "java.base/java.lang=ALL-UNNAMED", "-jar", launcher[0], "-configuration", filepath.Join(base, "configuration"), "-data", filepath.Join(base, "data")},
		Env:           []string{"HOME=" + env.Home, "JAVA_HOME=" + jdk, "PATH=" + filepath.Dir(java) + string(os.PathListSeparator) + "/usr/bin:/bin"},
		AdapterPolicy: javaPolicy + runtime.GOOS + "/" + runtime.GOARCH + ":" + manifest.digest,
		ConfigPaths:   configs, Toolchain: tools, Settings: settings, InitializationOptions: options, DeclareDiagnostics: true,
		ValidateStart: func() error {
			if err := root.Revalidate(); err != nil {
				return err
			}
			if err := manifest.unchanged(); err != nil {
				return err
			}
			current, err := discoverProjectDirs(root, javaProjectMarkers)
			if err != nil {
				return err
			}
			currentConfigs, err := javaApprovalConfigs(root, current.configs)
			if err != nil || !slices.Equal(configs, currentConfigs) {
				return failure("approvalChanged", "Java build configuration set changed; review again")
			}
			for _, dir := range []string{filepath.Join(base, "configuration"), filepath.Join(base, "data")} {
				real, err := javaCachePath(dir)
				if err != nil || real != dir {
					return failure("approvalChanged", "Java cache location changed; review again")
				}
			}
			return nil
		},
	}
	if _, err := Fingerprint(root, p.config.Tools()); err != nil {
		return unavailable("unsupported", "The Java or JDT LS files could not be identified for approval.")
	}
	return p, nil
}

// Resolve only reviewed layout hints, never arbitrary wrapper contents. A linked
// bin/jdtls identifies an extracted distribution; package managers often keep the
// distribution in <prefix>/share/java/jdtls or <prefix>/share/jdtls instead.
func resolveJavaDistribution(dirs []string) (string, bool) {
	found := false
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) || hasParent(dir) {
			continue
		}
		var candidates []string
		if real, err := filepath.EvalSymlinks(filepath.Join(dir, "jdtls")); err == nil {
			found = true
			if filepath.Base(real) == "jdtls" && filepath.Base(filepath.Dir(real)) == "bin" {
				candidates = append(candidates, filepath.Dir(filepath.Dir(real)))
			}
		}
		prefix := filepath.Dir(dir)
		for _, rel := range []string{"share/java/jdtls", "share/jdtls", "lib/jdtls", "libexec/jdtls"} {
			candidates = append(candidates, filepath.Join(prefix, filepath.FromSlash(rel)))
		}
		for _, candidate := range candidates {
			real, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				continue
			}
			if st, err := os.Stat(filepath.Join(real, "plugins")); err == nil && st.IsDir() {
				return real, true
			}
		}
	}
	return "", found
}

func resolveJavaRuntime(home string, dirs []string) (string, string) {
	if home != "" {
		dirs = []string{filepath.Join(home, "bin")}
	}
	name := "java"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, dir := range dirs {
		if !filepath.IsAbs(dir) || hasParent(dir) {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(dir, name))
		if err != nil || !isExecutable(real) || !javaNative(real) {
			continue
		}
		return real, filepath.Dir(filepath.Dir(real))
	}
	return "", ""
}

func javaNative(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	switch runtime.GOOS {
	case "linux", "freebsd":
		return string(b[:]) == "\x7fELF"
	case "darwin":
		return slices.Contains([]string{"\xcf\xfa\xed\xfe", "\xfe\xed\xfa\xcf", "\xca\xfe\xba\xbe", "\xbe\xba\xfe\xca"}, string(b[:]))
	case "windows":
		return string(b[:2]) == "MZ"
	}
	return false
}

func javaReleaseVersion(file string) (int, string) {
	data, present, ok := readBoundedFile(file, filepath.Dir(file), 64<<10)
	if !present || !ok {
		return 0, ""
	}
	version := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "JAVA_VERSION="); ok {
			if version != "" {
				return 0, ""
			}
			v, err := strconv.Unquote(strings.TrimSpace(value))
			if err != nil || !plainVersion(v) {
				return 0, ""
			}
			version = v
		}
	}
	first, _, _ := strings.Cut(version, ".")
	first, _, _ = strings.Cut(first, "-")
	first, _, _ = strings.Cut(first, "+")
	major, _ := strconv.Atoi(first)
	return major, version
}

func javaRegular(file string) bool {
	st, err := os.Stat(file)
	return err == nil && st.Mode().IsRegular()
}

// Canonicalize the existing portion, leaving missing suffixes uncreated.
func javaCachePath(p string) (string, error) {
	real, err := filepath.EvalSymlinks(p)
	if err == nil {
		st, e := os.Stat(real)
		if e != nil || !st.IsDir() {
			return "", failure("unsupported", "Cache is not a directory")
		}
		return real, nil
	}
	if !os.IsNotExist(err) || filepath.Dir(p) == p {
		return "", err
	}
	parent, err := javaCachePath(filepath.Dir(p))
	if err != nil {
		return "", err
	}
	// A dangling symlink is not a missing directory.
	if _, err := os.Lstat(p); err == nil {
		return "", failure("unsupported", "Cache contains an unresolved link")
	}
	return filepath.Join(parent, filepath.Base(p)), nil
}

func javaApprovalConfigs(root AuthorizedRoot, configs []string) ([]string, error) {
	out := slices.Clone(configs)
	moduleDirs := []string{"."}
	for _, config := range configs {
		if path.Base(config) == "pom.xml" {
			moduleDirs = append(moduleDirs, path.Dir(config))
		}
	}
	slices.Sort(moduleDirs)
	moduleDirs = slices.Compact(moduleDirs)
	for _, dir := range moduleDirs {
		for _, name := range []string{".mvn/maven.config", ".mvn/jvm.config", ".mvn/extensions.xml", ".mvn/wrapper/maven-wrapper.properties", ".mvn/wrapper/maven-wrapper.jar"} {
			rel := path.Join(dir, name)
			if _, err := os.Lstat(filepath.Join(root.Project().Root, filepath.FromSlash(rel))); os.IsNotExist(err) {
				continue
			}
			file, err := root.ResolvePath(root.Project().HostID, rel)
			if err != nil {
				return nil, err
			}
			if !javaRegular(file) {
				return nil, failure("unsupported", "Invalid Java build configuration")
			}
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > maxTSConfigs {
		return nil, failure("limitExceeded", "Too many Java build configurations")
	}
	return out, nil
}

// A single policy digest binds hundreds of Eclipse bundles without exhausting
// the composite tool budget. Hashing retains the existing 256MiB/file and 512MiB
// aggregate limits. Revalidation re-enumerates names and checks file identities,
// not hundreds of megabytes on every status/publication. Fresh discovery hashes
// content again. As for the shared identity code, this is not a hostile-host
// sandbox: stat-preserving replacement by a privileged local actor is out of scope.
type javaManifest struct {
	dirs, extra, paths []string
	stats              []os.FileInfo
	real               []string
	digest             string
}

func javaIdentityPaths(dirs, extra []string) ([]string, error) {
	files := slices.Clone(extra)
	entries := 0
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 32 || entries >= maxJavaIdentityEntries {
			return failure("limitExceeded", "Too many JDT LS bundle entries")
		}
		st, err := os.Lstat(dir)
		if err != nil || !st.IsDir() {
			return failure("unsupported", "JDT LS bundle directory is missing or linked")
		}
		f, err := os.Open(dir)
		if err != nil {
			return err
		}
		ds, err := f.ReadDir(maxJavaIdentityEntries + 1 - entries)
		f.Close()
		if err != nil && err != io.EOF {
			return err
		}
		entries += len(ds)
		if entries > maxJavaIdentityEntries {
			return failure("limitExceeded", "Too many JDT LS bundle entries")
		}
		for _, d := range ds {
			p := filepath.Join(dir, d.Name())
			if d.IsDir() {
				if err := walk(p, depth+1); err != nil {
					return err
				}
			} else {
				if !javaRegular(p) {
					return failure("unsupported", "JDT LS contains an unsupported bundle entry")
				}
				files = append(files, p)
			}
		}
		return nil
	}
	for _, dir := range dirs {
		if err := walk(dir, 0); err != nil {
			return nil, err
		}
	}
	slices.Sort(files)
	return slices.Compact(files), nil
}

func javaDistributionManifest(dirs, extra []string, reservedBytes int64) (*javaManifest, error) {
	paths, err := javaIdentityPaths(dirs, extra)
	if err != nil {
		return nil, err
	}
	m := &javaManifest{dirs: slices.Clone(dirs), extra: slices.Clone(extra), paths: paths}
	identities := make([]fileIdentity, 0, len(paths))
	total := reservedBytes
	for _, p := range paths {
		id, err := identifyFile(p, &total, nil, "")
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(p)
		if err != nil || st.Size() != id.Size || st.ModTime().UnixNano() != id.Modified || uint32(st.Mode()) != id.Mode {
			return nil, failure("approvalChanged", "JDT LS changed during discovery")
		}
		m.stats = append(m.stats, st)
		m.real = append(m.real, id.Path)
		identities = append(identities, id)
	}
	data, _ := json.Marshal(identities)
	sum := sha256.Sum256(data)
	m.digest = hex.EncodeToString(sum[:])
	return m, nil
}

func (m *javaManifest) unchanged() error {
	paths, err := javaIdentityPaths(m.dirs, m.extra)
	if err != nil || !slices.Equal(paths, m.paths) {
		return failure("approvalChanged", "JDT LS bundle set changed; review again")
	}
	for i, p := range paths {
		real, err := filepath.EvalSymlinks(p)
		if err != nil || real != m.real[i] {
			return failure("approvalChanged", "JDT LS runtime or bundle link changed; review again")
		}
		st, err := os.Stat(p)
		before := m.stats[i]
		if err != nil || !os.SameFile(before, st) || before.Size() != st.Size() || before.Mode() != st.Mode() || !before.ModTime().Equal(st.ModTime()) {
			return failure("approvalChanged", "JDT LS runtime or bundle changed; review again")
		}
	}
	return nil
}

func JavaLanguageID(file string) (string, bool) {
	if strings.EqualFold(path.Ext(file), ".java") {
		return "java", true
	}
	return "", false
}
func (p *JavaPlan) Availability() string         { return p.availability }
func (p *JavaPlan) Version() string              { return p.version }
func (p *JavaPlan) JavaVersion() string          { return p.javaVersion }
func (p *JavaPlan) Projects() []string           { return slices.Clone(p.projects) }
func (p *JavaPlan) Server() (ServerConfig, bool) { return p.config, p.availability == "available" }
func (p *JavaPlan) covers(file string) bool      { return coversDir(p.projects, file) }
func (p *JavaPlan) Provider() ProviderState {
	roots := slices.Clone(p.projects)
	if roots == nil {
		roots = []string{}
	}
	return ProviderState{ProviderID: javaProviderID, Availability: p.availability, Phase: "idle", Freshness: "uncertain", Coverage: Coverage{Kind: "openedDocuments", Roots: roots, Partial: p.partial, ExternalChanges: CoverageExternalChanges}, DocumentExtensions: slices.Clone(javaDocumentExtensions), Notice: shorten(p.notice+" "+javaScopeNotice+" "+javaOpenedNotice, 2048), MissingTool: p.availability == "missingTool"}
}

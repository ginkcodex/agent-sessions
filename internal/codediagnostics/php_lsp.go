package codediagnostics

// Intelephense discovery reads metadata only. It never invokes PHP, Composer,
// npm, a command wrapper or a version probe. After approval, reviewed native
// Node runs the npm package's own bundled entry directly with --stdio.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	phpProviderID      = "intelephense"
	phpPolicy          = "intelephense-v1:" + runtime.GOOS + "/" + runtime.GOARCH + ":node-direct:push-only:no-edits:opened-documents"
	phpToolchainPolicy = "intelephense-toolchain-v1"
	// The current npm bundle includes lru-cache 11 (Node 20 or >=22).
	// Require Node 22+ as a conservative adapter baseline rather than probing
	// or relying on the older Node 12 requirement in upstream installation docs.
	phpMinNodeMajor  = 22
	partialPHPNotice = "Some PHP diagnostics could not be verified; results are partial."
	phpOpenedNotice  = "Intelephense diagnostics cover opened documents only; unopened files are not continuously synchronized and external changes are unwatched. This is not a whole-project PHP lint or PHPStan check."
	phpScopeNotice   = "Intelephense is proprietary software. Definition, references, hover and opened-file diagnostics are free; premium features are outside this integration. No PHP runtime, Composer execution or paid licence is required. The server indexes project and dependency sources; bundled PHP stubs and index caches are not part of approval."
	phpInstallHint   = "Install Node.js 22 or newer and Intelephense with npm install -g intelephense, then refresh. The app does not install this server automatically."
)

var phpDocumentExtensions = []string{".php", ".phtml"}

// Composer marks workspace boundaries; both metadata and lockfile identities
// are bound without invoking Composer or following autoload paths in discovery.
var phpProjectMarkers = projectMarkers{
	configs: []string{"composer.json", "composer.lock"},
	skipDir: []string{"vendor", "node_modules", "target", ".gradle"},
	sources: phpDocumentExtensions,
}

// PHPEnvironment is reviewed host metadata, not the inherited environment.
type PHPEnvironment struct{ Home string }

// PHPPlan is an independent, user-installed Intelephense discovery result.
type PHPPlan struct {
	root                  AuthorizedRoot
	availability, notice  string
	config                ServerConfig
	version, node         string
	projects              []string
	partial, projectLocal bool
}

var _ livePlan = (*PHPPlan)(nil)

// DiscoverPHP inspects contained PHP projects and installed server/runtime
// metadata without executing tools or writing discovery state.
func DiscoverPHP(root AuthorizedRoot, reviewedPath []string, env PHPEnvironment) (*PHPPlan, error) {
	return discoverPHPWorkspace(root, reviewedPath, env, nil)
}

func discoverPHPWorkspace(root AuthorizedRoot, reviewedPath []string, env PHPEnvironment, workspace *tsWorkspace) (*PHPPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	p := &PHPPlan{root: root, availability: "available", notice: phpScopeNotice}
	unavailable := func(state, notice string) (*PHPPlan, error) {
		p.availability, p.notice, p.partial = state, notice, true
		return p, nil
	}
	w, err := workspaceMetadata(workspace, func() (tsWorkspace, error) { return discoverProjectDirs(root, phpProjectMarkers) })
	if err != nil {
		return nil, err
	}
	p.partial = w.partial
	if !w.sources && len(w.configs) == 0 {
		return unavailable("unsupported", "No PHP file or Composer project was found in this project.")
	}
	if !filepath.IsAbs(env.Home) || !bounded(env.Home, 4096) || hasParent(env.Home) {
		return nil, failure("invalidArgument", "A reviewed absolute home path is required")
	}
	p.projects = []string{""}
	entry, local, foreign := phpResolve(root, reviewedPath)
	if entry == "" {
		if foreign {
			return unavailable("unsupported", "The intelephense command found is not a directly installed Intelephense npm entry (for example a version-manager shim), so it is not used. "+phpInstallHint)
		}
		return unavailable("missingTool", "Intelephense is not installed. "+phpInstallHint)
	}
	version, pkgDir, state, notice := phpInspect(entry)
	if state != "" {
		return unavailable(state, notice)
	}
	p.version, p.projectLocal = version, local
	node := phpNode(pkgDir, reviewedPath)
	if node == "" {
		return unavailable("missingTool", "Node.js, required by Intelephense, was not found. "+phpInstallHint)
	}
	major, nodeVer := nodeVersion(node)
	if nodeVer == "" {
		return unavailable("unsupported", "The Node.js version could not be verified from its installed metadata.")
	}
	if major < phpMinNodeMajor {
		return unavailable("unsupported", "Node.js "+nodeVer+" is too old for this Intelephense adapter. "+phpInstallHint)
	}
	p.node = nodeVer
	nodeBin := filepath.Dir(node)
	if strings.Contains(nodeBin, ":") {
		return unavailable("unsupported", "The Node.js directory cannot be placed on a fixed search path.")
	}
	p.config = ServerConfig{
		ProviderID:    phpProviderID,
		Executable:    node,
		Args:          []string{entry, "--stdio"},
		Env:           []string{"HOME=" + env.Home, "PATH=" + nodeBin, "NODE_DISABLE_COMPILE_CACHE=1"},
		AdapterPolicy: phpPolicy,
		ConfigPaths:   slices.Clone(w.configs),
		Toolchain: []ToolInput{
			{ProviderID: "intelephense-toolchain-entry", Executable: entry, Policy: phpToolchainPolicy},
			{ProviderID: "intelephense-toolchain-package", Executable: filepath.Join(pkgDir, "package.json"), Policy: phpToolchainPolicy},
			{ProviderID: "intelephense-toolchain-node-metadata", Executable: filepath.Join(filepath.Dir(nodeBin), "include", "node", "node_version.h"), Policy: phpToolchainPolicy},
		},
		Settings:           phpSettings(),
		DeclareDiagnostics: true,
	}
	if _, err := Fingerprint(root, p.config.Tools()); err != nil {
		if err := root.Revalidate(); err != nil {
			return nil, err
		}
		p.config = ServerConfig{}
		return unavailable("unsupported", "The Intelephense files could not be identified for approval.")
	}
	return p, nil
}

func phpSettings() json.RawMessage {
	return json.RawMessage(`{"intelephense":{"files":{"associations":["*.php","*.phtml"]},"diagnostics":{"enable":true,"run":"onType"},"format":{"enable":false},"telemetry":{"enabled":false},"environment":{"includePaths":[]}}}`)
}

// phpResolve accepts only the known npm layout, not shell scripts, npm shims,
// or arbitrary package bin targets. Package metadata is verified separately.
func phpResolve(root AuthorizedRoot, reviewedPath []string) (entry string, local, foreign bool) {
	try := func(command string) string {
		real, err := filepath.EvalSymlinks(command)
		if err != nil {
			return ""
		}
		st, err := os.Stat(real)
		if err != nil || !st.Mode().IsRegular() {
			foreign = true
			return ""
		}
		if !strings.HasSuffix(filepath.ToSlash(real), "/node_modules/intelephense/lib/intelephense.js") {
			foreign = true
			return ""
		}
		return real
	}
	base := root.Project().Root
	if real := try(filepath.Join(base, "node_modules", ".bin", "intelephense")); real != "" {
		if rel, err := filepath.Rel(base, real); err == nil && filepath.IsLocal(rel) {
			return real, true, false
		}
		foreign = true
	}
	for _, dir := range reviewedPath {
		if !filepath.IsAbs(dir) || !bounded(dir, 4096) || hasParent(dir) {
			continue
		}
		if real := try(filepath.Join(dir, "intelephense")); real != "" {
			return real, false, false
		}
	}
	return "", false, foreign
}

// phpInspect verifies the package name/version and its bin mapping. The
// published package has one bundled JavaScript entry plus data-only stubs.
// No code is evaluated, and no arbitrary bin path is accepted from metadata.
func phpInspect(entry string) (version, pkgDir, state, notice string) {
	pkgDir = filepath.Dir(filepath.Dir(entry))
	unsupported := func() (string, string, string, string) {
		return "", "", "unsupported", "The Intelephense npm package name, version or entry could not be verified. " + phpInstallHint
	}
	var pkg struct {
		Name, Version string
		Bin           json.RawMessage
	}
	meta := filepath.Join(pkgDir, "package.json")
	if st, err := os.Lstat(meta); err != nil || !st.Mode().IsRegular() {
		return unsupported()
	}
	if readJSONFile(meta, &pkg) != nil || pkg.Name != phpProviderID || !plainVersion(pkg.Version) {
		return unsupported()
	}
	var bin string
	if json.Unmarshal(pkg.Bin, &bin) != nil {
		var bins map[string]string
		if json.Unmarshal(pkg.Bin, &bins) != nil {
			return unsupported()
		}
		bin = bins[phpProviderID]
	}
	if bin != "lib/intelephense.js" && bin != "./lib/intelephense.js" {
		return unsupported()
	}
	// npm's symlink must land exactly on the declared standard entry; no
	// symlinked lib, package metadata or stub directory is accepted.
	for _, name := range []string{"lib", "lib/stub"} {
		st, err := os.Lstat(filepath.Join(pkgDir, name))
		if err != nil || !st.IsDir() {
			return "", "", "unsupported", "The Intelephense installation is incomplete. " + phpInstallHint
		}
	}
	st, err := os.Lstat(entry)
	if entry != filepath.Join(pkgDir, "lib", "intelephense.js") || err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return unsupported()
	}
	return pkg.Version, pkgDir, "", ""
}

// Prefer Node beside a global npm prefix, then the first native runtime in
// reviewed absolute PATH directories. realNode rejects version-manager shims.
func phpNode(pkgDir string, reviewedPath []string) string {
	var dirs []string
	if prefix, ok := strings.CutSuffix(pkgDir, filepath.Join("lib", "node_modules", phpProviderID)); ok {
		dirs = append(dirs, filepath.Join(prefix, "bin"))
	}
	dirs = append(dirs, reviewedPath...)
	for _, dir := range dirs {
		if node := realNode(dir); node != "" {
			return node
		}
	}
	return ""
}

// PHPLanguageID assigns PHP ownership to .php and .phtml documents.
func PHPLanguageID(file string) (string, bool) {
	if slices.Contains(phpDocumentExtensions, strings.ToLower(filepath.Ext(file))) {
		return "php", true
	}
	return "", false
}

func (p *PHPPlan) Availability() string         { return p.availability }
func (p *PHPPlan) Version() string              { return p.version }
func (p *PHPPlan) NodeVersion() string          { return p.node }
func (p *PHPPlan) ProjectLocal() bool           { return p.projectLocal }
func (p *PHPPlan) Projects() []string           { return slices.Clone(p.projects) }
func (p *PHPPlan) Server() (ServerConfig, bool) { return p.config, p.availability == "available" }
func (p *PHPPlan) covers(file string) bool      { return coversDir(p.projects, file) }
func (p *PHPPlan) Provider() ProviderState {
	roots := slices.Clone(p.projects)
	if roots == nil {
		roots = []string{}
	}
	notice := phpOpenedNotice
	if p.notice != "" {
		notice = p.notice + " " + notice
	}
	return ProviderState{
		ProviderID: phpProviderID, Availability: p.availability, Phase: "idle", Freshness: "uncertain",
		Coverage: Coverage{Kind: "openedDocuments", Roots: roots, Partial: p.partial, ExternalChanges: CoverageExternalChanges},
		Notice:   shorten(notice, 2048), DocumentExtensions: slices.Clone(phpDocumentExtensions),
	}
}

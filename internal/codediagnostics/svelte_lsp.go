package codediagnostics

// Svelte language-server adapter (CD-24).
//
// Only verified server/runtime combinations run (supportedSvelteLSVersions):
// the app's managed install pins one, verified on darwin/arm64. Node runs the
// package's own bin/server.js directly, like vtsls in ts_lsp.go. Once the
// project is approved the server loads its svelte.config.* and vite.config.*
// files and may use the project's own svelte compiler, which is project code
// like a workspace TypeScript; those config files are bound into approval.
// Implemented here: contained package detection (metadata only, config never
// evaluated by this file), fixed-argv resolution, deterministic
// Svelte/TypeScript ownership, session keying and a publishDiagnostics bridge.
// Evidence and limits: docs/plan/code-diagnostics-svelte.md and
// testdata/svelte-lsp/evidence.json. This file never installs, downloads, runs
// npx/package scripts or executes anything to probe versions.

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	svelteLSProviderID = "svelte"
	svelteLSName       = "svelte-language-server"
	svelteLSPolicy     = "svelte-ls-v2:%s:svelte%s:ts%s:darwin/arm64:node-direct:push-only:no-edits:opened-documents:source-coordinates-only"
	svelteToolPolicy   = "svelte-ls-toolchain-v1"
	maxSveltePackages  = 48
	maxSvelteConfigs   = 192
	maxSvelteEntries   = 4096

	partialSvelteNotice = "Some Svelte diagnostics could not be verified; results are partial."
	svelteOpenedNotice  = "Svelte diagnostics cover opened .svelte documents only; unopened files are not continuously synchronized."
	svelteInstallHint   = "The app can download the verified svelte-language-server into its own folder (using your Node.js 18 or newer, or its own)."
	// SvelteVirtualNotice is shown when a server reports on a virtual/generated
	// document. No mapping to source offsets is verified, so nothing is linked.
	SvelteVirtualNotice = "Some Svelte findings were reported on generated documents and cannot be mapped to source; coverage is limited."
)

// SvelteLSVersion is a verified server/runtime combination: the server and the
// svelte and typescript packages it resolves from its own directory.
type SvelteLSVersion struct{ Server, Svelte, TypeScript string }

// supportedSvelteLSVersions lists the combinations verified against a real
// server (testdata/svelte-lsp/evidence.json). It matches the managed install
// pinned by internal/managedtools/svelte-language-server.
var supportedSvelteLSVersions = []SvelteLSVersion{{"0.18.5", "4.2.20", "6.0.3"}}

// svelteToolNames label the loaded files bound into approval, in the order
// svelteToolchain lists them after the entry point.
var svelteToolNames = []string{"server", "typescript"}

// SvelteLSEnvironment is reviewed host metadata, not the inherited environment.
type SvelteLSEnvironment struct {
	Home string
	// NodeDir optionally pins the reviewed directory containing the node
	// runtime. Empty uses the node beside a global npm install, then the first
	// real node in the reviewed path.
	NodeDir string

	// versionOf reads package metadata without executing anything. Nil reads
	// the installed package.json files.
	versionOf func(entry string) (name, server, svelte, typescript string, err error)
	// supported overrides the verified list; tests only.
	supported []SvelteLSVersion
}

// SvelteLSPlan is the discovery result. Availability is independent of the
// svelte-check, TypeScript and Go providers.
type SvelteLSPlan struct {
	root         AuthorizedRoot
	availability string
	notice       string
	config       ServerConfig
	version      SvelteLSVersion
	packages     []string
	partial      bool
	projectLocal bool
}

// DiscoverSvelteLS inspects metadata only. Errors are reserved for an invalid
// host description or a root that is no longer authorized.
func DiscoverSvelteLS(root AuthorizedRoot, reviewedPath []string, env SvelteLSEnvironment) (*SvelteLSPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	reviewed := func(p string) bool { return filepath.IsAbs(p) && bounded(p, 4096) && !hasParent(p) }
	if !reviewed(env.Home) || (env.NodeDir != "" && !reviewed(env.NodeDir)) {
		return nil, failure("invalidArgument", "Reviewed absolute home and node paths are required")
	}
	p := &SvelteLSPlan{root: root, availability: "available"}
	unavailable := func(state, notice string) (*SvelteLSPlan, error) {
		p.availability, p.notice, p.partial = state, notice, true
		return p, nil
	}
	// Packages first: a project without Svelte stays a muted "unsupported"
	// rather than asking for an install it does not need.
	w, err := discoverSveltePackages(root)
	if err != nil {
		return nil, err
	}
	p.packages, p.partial = w.packages, w.partial
	if len(w.packages) == 0 {
		return unavailable("unsupported", "No contained Svelte package was found.")
	}
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return unavailable("unsupported", "Svelte language server support has not been verified on this platform.")
	}
	versionOf := env.versionOf
	if versionOf == nil {
		versionOf = svelteServerVersions
	}
	supported := supportedSvelteLSVersions
	if env.supported != nil {
		supported = env.supported
	}
	entry, local, state, notice := resolveSvelteServer(root, reviewedPath, func(entry string) (string, string) {
		name, server, svelte, typescript, err := versionOf(entry)
		switch {
		case err != nil || name != svelteLSName:
			return "unsupported", "The Svelte language server version could not be verified."
		case svelte == "" || typescript == "":
			return "missingTool", "The Svelte or TypeScript package required by the language server is not installed. " + svelteInstallHint
		case !slices.Contains(supported, SvelteLSVersion{server, svelte, typescript}):
			return "unsupported", "Svelte language server " + versionText(server) + " has not been verified; only the version the app installs is used. " + svelteInstallHint
		}
		p.version = SvelteLSVersion{server, svelte, typescript}
		return "", ""
	})
	if entry == "" {
		return unavailable(state, notice)
	}
	p.projectLocal = local
	files, ok := svelteToolchain(entry)
	if !ok {
		return unavailable("unsupported", "The Svelte language server installation is incomplete. "+svelteInstallHint)
	}
	serverDir := filepath.Dir(filepath.Dir(entry))
	node := serverNode(env.NodeDir, serverDir, svelteLSName, reviewedPath)
	if node == "" {
		return unavailable("missingTool", "Node.js 18 or newer, required by the Svelte language server, was not found. Install Node.js, then refresh.")
	}
	switch major, version := nodeVersion(node); {
	case version == "":
		return unavailable("unsupported", "The Node.js version could not be verified from its installed metadata.")
	case major < minTSNodeMajor:
		return unavailable("unsupported", "Node.js "+version+" is too old for the Svelte language server; version 18 or newer is required.")
	}
	nodeBin := filepath.Dir(node)
	if strings.Contains(nodeBin, ":") {
		return unavailable("unsupported", "The Node.js directory cannot be placed on a fixed search path.")
	}
	tools := []ToolInput{{ProviderID: "svelte-toolchain-entry", Executable: entry, Policy: svelteToolPolicy}}
	for i, f := range files {
		tools = append(tools, ToolInput{ProviderID: "svelte-toolchain-" + svelteToolNames[i], Executable: f, Policy: svelteToolPolicy})
	}
	p.config = ServerConfig{
		ProviderID:    svelteLSProviderID,
		Executable:    node,
		Args:          []string{entry, "--stdio"},
		Env:           []string{"HOME=" + env.Home, "PATH=" + nodeBin + ":/usr/bin:/bin", "NODE_DISABLE_COMPILE_CACHE=1"},
		AdapterPolicy: sprintfPolicy(p.version.Server, p.version.Svelte, p.version.TypeScript),
		ConfigPaths:   w.configs,
		Toolchain:     tools,
		// Like vtsls, it is told the client accepts pushed diagnostics.
		DeclareDiagnostics: true,
		// Without it the server watches the workspace with its own watcher.
		AcknowledgeWatchers: true,
	}
	if _, err := Fingerprint(root, p.config.Tools()); err != nil {
		if err := root.Revalidate(); err != nil {
			return nil, err
		}
		p.config = ServerConfig{}
		return unavailable("unsupported", "The Svelte language server files could not be identified for approval.")
	}
	return p, nil
}

func sprintfPolicy(server, svelte, ts string) string {
	parts := strings.SplitN(svelteLSPolicy, "%s", 4)
	return parts[0] + server + parts[1] + svelte + parts[2] + ts + parts[3]
}

// resolveSvelteServer finds the package's own bin/server.js. The contained
// project-local link (project content covered by CD-02 trust) is tried first,
// then reviewed absolute PATH directories; the first candidate whose versions
// verify wins, so an unverified project-local server falls back to the app's
// install. A command that does not resolve to a real svelte-language-server
// entry (a version-manager shim or any other wrapper) is skipped; nothing is
// ever executed. With no usable candidate it reports the first rejection.
func resolveSvelteServer(root AuthorizedRoot, reviewedPath []string, verify func(entry string) (state, notice string)) (entry string, local bool, state, notice string) {
	state, notice = "missingTool", "No Svelte language server (svelte-language-server) is installed. "+svelteInstallHint
	rejected := false
	try := func(link string) string {
		real, err := filepath.EvalSymlinks(link)
		if err != nil {
			return ""
		}
		if st, err := os.Stat(real); err != nil || !st.Mode().IsRegular() {
			return ""
		}
		if !strings.HasSuffix(filepath.ToSlash(real), "/node_modules/svelte-language-server/bin/server.js") {
			if !rejected {
				rejected = true
				state, notice = "unsupported", "The svelteserver command found is not a directly installed svelte-language-server package (for example a version-manager shim), so it is not used. "+svelteInstallHint
			}
			return ""
		}
		if s, n := verify(real); s != "" {
			if !rejected {
				rejected = true
				state, notice = s, n
			}
			return ""
		}
		return real
	}
	base := root.Project().Root
	if real, err := filepath.EvalSymlinks(filepath.Join(base, "node_modules", ".bin", "svelteserver")); err == nil {
		if rel, err := filepath.Rel(base, real); err == nil && filepath.IsLocal(rel) {
			if real := try(real); real != "" {
				return real, true, "", ""
			}
		}
	}
	for _, dir := range reviewedPath {
		if !filepath.IsAbs(dir) || hasParent(dir) {
			continue
		}
		if real := try(filepath.Join(dir, "svelteserver")); real != "" {
			return real, false, "", ""
		}
	}
	return "", false, state, notice
}

// svelteServerVersions reads package metadata along the path Node takes from
// <pkg>/bin/server.js: the package itself, then the svelte and typescript
// packages its dist/src code resolves (nested or flattened installs alike). It
// never executes the server. Missing or foreign packages report an empty
// version.
func svelteServerVersions(entry string) (string, string, string, string, error) {
	pkgDir := filepath.Dir(filepath.Dir(entry))
	var pkg struct{ Name, Version string }
	if err := readJSONFile(filepath.Join(pkgDir, "package.json"), &pkg); err != nil {
		return "", "", "", "", err
	}
	resolve := func(name string) string {
		var m struct{ Name, Version string }
		dir := nodeResolvePackage(filepath.Join(pkgDir, "dist", "src"), name)
		if dir == "" || readJSONFile(filepath.Join(dir, "package.json"), &m) != nil || m.Name != name {
			return ""
		}
		return m.Version
	}
	return pkg.Name, pkg.Version, resolve("svelte"), resolve("typescript"), nil
}

// svelteToolchain lists the files the entry point loads that approval binds
// (see svelteToolNames): bin/server.js requires dist/src/server.js, which
// requires typescript.
func svelteToolchain(entry string) ([]string, bool) {
	pkgDir := filepath.Dir(filepath.Dir(entry))
	tsDir := nodeResolvePackage(filepath.Join(pkgDir, "dist", "src"), "typescript")
	if tsDir == "" {
		return nil, false
	}
	files := []string{filepath.Join(pkgDir, "dist", "src", "server.js"), filepath.Join(tsDir, "lib", "typescript.js")}
	for _, f := range files {
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			return nil, false
		}
	}
	return files, true
}

func (p *SvelteLSPlan) Availability() string         { return p.availability }
func (p *SvelteLSPlan) Version() SvelteLSVersion     { return p.version }
func (p *SvelteLSPlan) Packages() []string           { return slices.Clone(p.packages) }
func (p *SvelteLSPlan) ProjectLocal() bool           { return p.projectLocal }
func (p *SvelteLSPlan) Server() (ServerConfig, bool) { return p.config, p.availability == "available" }

// Provider describes honest opened-documents coverage. Data only; no
// production route consumes it, and an empty push is never reported as clean.
func (p *SvelteLSPlan) Provider() ProviderState {
	notice := svelteOpenedNotice
	if p.notice != "" {
		notice = p.notice + " " + svelteOpenedNotice
	}
	roots := slices.Clone(p.packages)
	if roots == nil {
		roots = []string{}
	}
	return ProviderState{
		ProviderID: svelteLSProviderID, Availability: p.availability, Phase: "idle", Freshness: "uncertain",
		Coverage:           Coverage{Kind: "openedDocuments", Roots: roots, Partial: p.partial, ExternalChanges: CoverageExternalChanges},
		Notice:             shorten(notice, 2048),
		DocumentExtensions: slices.Clone(svelteDocumentExtensions),
	}
}

// ---- ownership ----------------------------------------------------------------

// DocumentOwner is the single deterministic routing rule shared with CD-23.
// .svelte belongs to the Svelte provider; JS/TS (including Svelte 5 rune
// modules such as x.svelte.ts, whose last extension is .ts) belongs to the
// TypeScript provider; everything else has no LSP owner. A document is sent to
// exactly one provider, so one file never gets two didOpen streams.
func DocumentOwner(file string) (providerID, languageID string) {
	if strings.ToLower(path.Ext(file)) == ".svelte" {
		return svelteLSProviderID, "svelte"
	}
	if lang, ok := TSLanguageID(file); ok {
		return tsProviderID, lang
	}
	return "", ""
}

// OwnedDocuments builds the single resolver over both plans (either may be
// nil or unavailable). Session keying is keyOf: (project, provider, policy),
// so there is at most one Svelte session and one TypeScript session per
// authorized root regardless of package count; packages are coverage, not
// session keys.
func OwnedDocuments(sessions *SessionManager, ts *TSPlan, sv *SvelteLSPlan) SessionDocuments {
	return SessionDocuments{Sessions: sessions, Resolve: func(root AuthorizedRoot, file string) (DocumentServer, bool) {
		owner, lang := DocumentOwner(file)
		switch owner {
		case svelteLSProviderID:
			if sv == nil || root.Project() != sv.root.Project() || !coversPackage(sv.packages, file) {
				return DocumentServer{}, false
			}
			cfg, ok := sv.Server()
			return DocumentServer{Config: cfg, LanguageID: lang}, ok
		case tsProviderID:
			if ts == nil || root.Project() != ts.root.Project() || !ts.covers(file) {
				return DocumentServer{}, false
			}
			cfg, ok := ts.Server()
			return DocumentServer{Config: cfg, LanguageID: lang}, ok
		}
		return DocumentServer{}, false
	}}
}

// Documents resolves only .svelte files inside a contained Svelte package.
func (p *SvelteLSPlan) Documents(sessions *SessionManager) SessionDocuments {
	return OwnedDocuments(sessions, nil, p)
}

func coversPackage(packages []string, file string) bool {
	dir := path.Dir(file)
	for {
		check := dir
		if dir == "." {
			check = ""
		}
		if slices.Contains(packages, check) {
			return true
		}
		if dir == "." || dir == "/" {
			return false
		}
		dir = path.Dir(dir)
	}
}

// ---- contained package detection ----------------------------------------------

type svelteWorkspace struct {
	packages, configs []string
	partial           bool
}

// discoverSveltePackages walks only inside the authorized root with bounded
// rooted reads. A directory is a Svelte package when it has a svelte.config.*
// file (existence only; never imported or evaluated here) or a package.json
// that declares svelte or @sveltejs/kit (parsed as JSON, bounded). Companion
// package/tsconfig/jsconfig/vite.config files bind into the approval
// fingerprint. Parent
// directories are never promoted; node_modules, hidden directories and
// symlinks are skipped (hidden/symlink ⇒ partial).
func discoverSveltePackages(root AuthorizedRoot) (svelteWorkspace, error) {
	var w svelteWorkspace
	r, err := os.OpenRoot(root.Project().Root)
	if err != nil {
		return w, failure("unauthorized", "Svelte discovery root cannot be opened")
	}
	defer func() { _ = r.Close() }()
	entries := 0
	var walk func(string, int)
	walk = func(dir string, depth int) {
		f, err := r.Open(dir)
		if err != nil {
			w.partial = true
			return
		}
		defer func() { _ = f.Close() }()
		ds, err := f.ReadDir(maxSvelteEntries + 1 - entries)
		if err != nil && !errors.Is(err, io.EOF) {
			w.partial = true
			return
		}
		slices.SortFunc(ds, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
		var files []string
		svelte := false
		for _, d := range ds {
			entries++
			if entries > maxSvelteEntries {
				w.partial = true
				return
			}
			rel := filepath.Join(dir, d.Name())
			portable := filepath.ToSlash(rel)
			if ValidatePath(portable) != nil || d.Type()&os.ModeSymlink != 0 {
				w.partial = true
				continue
			}
			if d.IsDir() {
				n := d.Name()
				if n == "node_modules" {
					continue
				}
				if strings.HasPrefix(n, ".") || depth >= 32 {
					w.partial = true
					continue
				}
				walk(rel, depth+1)
				continue
			}
			switch d.Name() {
			case "svelte.config.js", "svelte.config.mjs", "svelte.config.cjs", "svelte.config.ts":
				svelte = true
				files = append(files, portable)
			case "tsconfig.json", "jsconfig.json",
				// The server loads these too; they bind like svelte.config.*.
				"vite.config.js", "vite.config.mjs", "vite.config.cjs", "vite.config.ts", "vite.config.mts", "vite.config.cts":
				files = append(files, portable)
			case "package.json":
				files = append(files, portable)
				if declaresSvelte(root, portable) {
					svelte = true
				}
			}
		}
		if !svelte {
			return
		}
		if len(w.packages) >= maxSveltePackages || len(w.configs)+len(files) > maxSvelteConfigs {
			w.partial = true
			return
		}
		pkg := filepath.ToSlash(dir)
		if pkg == "." {
			pkg = ""
		}
		w.packages = append(w.packages, pkg)
		w.configs = append(w.configs, files...)
	}
	walk(".", 0)
	slices.Sort(w.packages)
	slices.Sort(w.configs)
	return w, nil
}

// declaresSvelte parses package.json as data only; unreadable or oversized
// manifests simply do not declare Svelte.
func declaresSvelte(root AuthorizedRoot, rel string) bool {
	data, err := rootedRead(root, rel, 1<<20)
	if err != nil {
		return false
	}
	var m struct {
		Dependencies, DevDependencies, PeerDependencies map[string]json.RawMessage
	}
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	for _, deps := range []map[string]json.RawMessage{m.Dependencies, m.DevDependencies, m.PeerDependencies} {
		for _, name := range []string{"svelte", "@sveltejs/kit"} {
			if _, ok := deps[name]; ok {
				return true
			}
		}
	}
	return false
}

// ---- notification bridge --------------------------------------------------------

// SvelteBridge turns session notifications into document-set replacements. It
// reuses the CD-22 normalizer with the "svelte" attribution prefix and
// accepts source coordinates for real, contained .svelte files only. Virtual
// documents (non-file URIs) and files owned by TypeScript are never published;
// virtual URIs raise SvelteVirtualNotice instead of fabricated links/offsets.
type SvelteBridge struct {
	root   AuthorizedRoot
	sink   goplsSink
	Notice func(string)
}

func NewSvelteBridge(root AuthorizedRoot, m *Manager) *SvelteBridge {
	return &SvelteBridge{root: root, sink: m}
}

func (b *SvelteBridge) note(s string) {
	if b.Notice != nil {
		b.Notice(s)
	}
}

// Handle must be quick and runs on the session goroutine.
func (b *SvelteBridge) Handle(n SessionNotification) {
	if n.Project != b.root.Project() || n.ProviderID != svelteLSProviderID || n.Method != "textDocument/publishDiagnostics" {
		return
	}
	var head struct {
		URI string `json:"uri"`
	}
	if len(n.Params) <= maxGoplsParams && json.Unmarshal(n.Params, &head) == nil {
		if u, err := url.Parse(head.URI); err == nil && u.Scheme != "" && u.Scheme != "file" {
			b.note(SvelteVirtualNotice)
			return
		}
	}
	pub, err := normalizeLSPPublication(b.root, n.Params, svelteLSProviderID)
	switch {
	case errors.Is(err, errGoplsOutside):
		b.note(OutsideRootNotice)
		return
	case errors.Is(err, errGoplsUnknown):
		return
	case err != nil:
		b.note(partialSvelteNotice)
		return
	}
	if owner, _ := DocumentOwner(pub.Path); owner != svelteLSProviderID {
		return // TypeScript-owned and config files never enter the Svelte set
	}
	if err := publishLivePublication(b.sink, n, pub); err != nil {
		var e *Error
		if !errors.As(err, &e) || (e.Code != "notFound" && e.Code != "staleContext" && e.Code != "unsupported") {
			b.note(partialSvelteNotice)
		}
		return
	}
	if pub.Dropped > 0 || pub.Partial || pub.Truncated {
		b.note(partialSvelteNotice)
	}
}

// svelteDocumentExtensions lists the extension DocumentOwner assigns to Svelte.
var svelteDocumentExtensions = []string{".svelte"}

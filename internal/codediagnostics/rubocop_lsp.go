package codediagnostics

// Ruby adapter: RuboCop's built-in language server (rubocop --lsp).
//
// Like ruff_lsp.go this file owns contained project discovery, a fixed-argv
// server identity with independent availability, and extension ownership;
// pushes go through the shared push bridge. RuboCop is Ruby code: the
// interpreter is resolved from the project's version request and the version
// managers' install directories (ruby_runtime.go), RuboCop and Bundler from
// gem directories (ruby_gems.go). Nothing here runs Ruby, Bundler or a shim.
// Bundler mode loads the project's Gemfile, which is Ruby code; approval
// covers that, as it covers Cargo build scripts. Observed server behavior is
// recorded in testdata/rubocop/evidence.json and
// docs/plan/code-diagnostics-ruby.md. Tests in rubocop_lsp_test.go are
// SYNTHETIC.

import (
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	rubocopProviderID = "rubocop"
	// rubocopPolicy binds the launch shape: the resolved interpreter runs the
	// gem's exe/rubocop directly (no shell, shim, bundle exec or binstub), in
	// Bundler mode through -rbundler/setup with the project's Gemfile.
	rubocopPolicy          = "rubocop-v1:any-version:" + runtime.GOOS + "/" + runtime.GOARCH + ":ruby-direct:"
	rubocopPolicyTail      = ":push-only:no-edits:opened-documents"
	rubocopToolchainPolicy = "rubocop-toolchain-v1:loaded-file"
	partialRubocopNotice   = "Some Ruby diagnostics could not be verified; results are partial."
	rubocopOpenedNotice    = "Ruby diagnostics cover opened documents only; unopened files are not continuously synchronized."
	rubocopScopeNotice     = "RuboCop reports lint and syntax problems, not type errors, and gives no navigation. Plugins, inherit_from and inherit_gem targets and your user RuboCop configuration still apply but are not part of approval."
	rubocopBundlerNotice   = "RuboCop runs through Bundler, which loads the project's Gemfile."
	rubocopInstallHint     = "Add it to your Gemfile (bundle add rubocop --group development) and run bundle install, or run gem install rubocop, then refresh."
	rubocopMinRuby         = "2.7"
	rubyShimHint           = "Version-manager shims are not run; add a .ruby-version file to the project, or activate the version manager in your login shell, then refresh."
)

// rubocopProjectMarkers: RuboCop's and Bundler's project files are bound into
// approval. Any Ruby source enables RuboCop; a Gemfile alone does not (iOS and
// Jekyll projects carry one for tooling). Installed gems, Bundler's and
// ruby-lsp's state, and Rails' tmp and log are skipped silently.
var rubocopProjectMarkers = projectMarkers{
	configs:    []string{"Gemfile", "Gemfile.lock", ".rubocop.yml", ".rubocop_todo.yml"},
	skipDir:    []string{".bundle", ".ruby-lsp", "node_modules"},
	skipPath:   []string{"vendor/bundle"},
	skipBeside: map[string]string{"tmp": "Gemfile", "log": "Gemfile"},
	sources:    rubyDocumentExtensions,
}

// RubocopPlan is the discovery result. Availability is independent of every
// other provider.
type RubocopPlan struct {
	root         AuthorizedRoot
	availability string
	notice       string
	config       ServerConfig
	version      string
	rubyVersion  string
	mode         string
	projects     []string
	partial      bool
	projectLocal bool
}

// DiscoverRubocop inspects metadata only. Errors are reserved for an invalid
// host description or a root that is no longer authorized.
func DiscoverRubocop(root AuthorizedRoot, reviewedPath []string, env RubyEnvironment) (*RubocopPlan, error) {
	return discoverRubocopWorkspace(root, reviewedPath, env, nil)
}

func discoverRubocopWorkspace(root AuthorizedRoot, reviewedPath []string, env RubyEnvironment, workspace *tsWorkspace) (*RubocopPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(env.Home) || !bounded(env.Home, 4096) || hasParent(env.Home) {
		return nil, failure("invalidArgument", "A reviewed absolute home path is required")
	}
	p := &RubocopPlan{root: root, availability: "available", notice: rubocopScopeNotice}
	unavailable := func(state, notice string) (*RubocopPlan, error) {
		p.availability, p.notice, p.partial = state, notice, true
		return p, nil
	}
	rw, state, notice, err := inspectRubyWorkspace(root, reviewedPath, env, workspace, rubocopProjectMarkers, "RuboCop", rubocopMinRuby)
	if err != nil {
		return nil, err
	}
	p.partial = rw.w.partial
	if rw.w.sources {
		p.projects = []string{""}
	}
	if state != "" {
		return unavailable(state, notice)
	}
	w, base, lock, lockPresent, lockOK, rt := rw.w, rw.base, rw.lock, rw.lockPresent, rw.lockOK, rw.rt
	p.rubyVersion = rt.Version
	inst := findRubocop(base, lock, lockPresent, lockOK, rt, env)
	switch inst.State {
	case "missing":
		return unavailable("missingTool", "RuboCop is not installed for Ruby "+rt.Version+". "+rubocopInstallHint)
	case "notInstalled":
		return unavailable("missingTool", "RuboCop "+inst.LockedVersion+" from Gemfile.lock is not installed. Run bundle install, then refresh.")
	case "missingBundler":
		return unavailable("missingTool", "Bundler, required by Gemfile.lock, was not found for Ruby "+rt.Version+". Run gem install bundler, then refresh.")
	case "unsupportedSource":
		return unavailable("unsupported", "Gemfile.lock takes RuboCop from a git or path source, which is not supported.")
	case "invalidLockfile":
		return unavailable("unsupported", "Gemfile.lock could not be read.")
	case "invalidBundleConfig":
		return unavailable("unsupported", ".bundle/config could not be read; only plain KEY: value settings are supported.")
	case "bundlePathOutside":
		return unavailable("unsupported", "BUNDLE_PATH in .bundle/config points outside the project.")
	}
	p.version, p.mode = inst.Rubocop.Version, inst.Mode
	p.projectLocal = within(base, inst.Rubocop.Path)

	rubyBin := filepath.Dir(rt.Interpreter)
	env1 := []string{"HOME=" + env.Home, "PATH=" + rubyBin + ":/usr/bin:/bin"}
	args := []string{inst.Rubocop.Entry, "--lsp"}
	tools := []ToolInput{
		{ProviderID: "rubocop-toolchain-entry", Executable: inst.Rubocop.Entry, Policy: rubocopToolchainPolicy},
		{ProviderID: "rubocop-toolchain-gemspec", Executable: inst.Rubocop.Spec, Policy: rubocopToolchainPolicy},
	}
	searchPath := []string{rubyBin}
	switch inst.Mode {
	case "bundler":
		args = append([]string{"-rbundler/setup"}, args...)
		env1 = append(env1, "BUNDLE_GEMFILE="+filepath.Join(base, "Gemfile"), "BUNDLE_FROZEN=true")
		if inst.BundlerEnv != "" {
			env1 = append(env1, "BUNDLER_VERSION="+inst.BundlerEnv)
		}
		tools = append(tools,
			ToolInput{ProviderID: "rubocop-toolchain-bundler-entry", Executable: inst.Bundler.Entry, Policy: rubocopToolchainPolicy},
			ToolInput{ProviderID: "rubocop-toolchain-bundler-gemspec", Executable: inst.Bundler.Spec, Policy: rubocopToolchainPolicy})
		p.notice = rubocopBundlerNotice + " " + p.notice
	default:
		env1 = append(env1, "GEM_HOME="+inst.Rubocop.Home, "GEM_PATH="+strings.Join(inst.GemDirs, ":"))
		searchPath = append(searchPath, inst.GemDirs...)
	}
	for _, dir := range searchPath {
		if strings.Contains(dir, ":") {
			return unavailable("unsupported", "The Ruby or gem directory cannot be placed on a fixed search path.")
		}
	}
	if inst.Mode == "bundler" && !slices.Contains(w.configs, "Gemfile") {
		return unavailable("unsupported", "Gemfile.lock locks RuboCop, but the project has no Gemfile.")
	}
	configs, versionFile := rubyApprovalFiles(rw)
	if versionFile != "" {
		tools = append(tools, ToolInput{ProviderID: "rubocop-toolchain-version-file", Executable: versionFile, Policy: rubocopToolchainPolicy})
	}
	p.config = ServerConfig{
		ProviderID:    rubocopProviderID,
		Executable:    rt.Interpreter,
		Args:          args,
		Env:           env1,
		AdapterPolicy: rubocopPolicy + inst.Mode + rubocopPolicyTail,
		ConfigPaths:   configs,
		Toolchain:     tools,
	}
	if _, err := Fingerprint(root, p.config.Tools()); err != nil {
		if err := root.Revalidate(); err != nil {
			return nil, err
		}
		p.config = ServerConfig{}
		return unavailable("unsupported", "The Ruby or RuboCop files could not be identified for approval.")
	}
	return p, nil
}

func (p *RubocopPlan) Availability() string { return p.availability }

// Version is RuboCop's gem version; RubyVersion the interpreter's.
func (p *RubocopPlan) Version() string     { return p.version }
func (p *RubocopPlan) RubyVersion() string { return p.rubyVersion }

// Mode is "bundler" or "standalone" when available.
func (p *RubocopPlan) Mode() string       { return p.mode }
func (p *RubocopPlan) Projects() []string { return slices.Clone(p.projects) }

// ProjectLocal reports that RuboCop itself is project content (vendor/bundle).
func (p *RubocopPlan) ProjectLocal() bool { return p.projectLocal }

// Server is the fixed server identity, only when the plan is available.
func (p *RubocopPlan) Server() (ServerConfig, bool) {
	return p.config, p.availability == "available"
}

// Provider describes honest opened-documents coverage. An empty push means
// RuboCop found no offense, never "type-clean".
func (p *RubocopPlan) Provider() ProviderState {
	notice := rubocopOpenedNotice
	if p.notice != "" {
		notice = p.notice + " " + rubocopOpenedNotice
	}
	roots := slices.Clone(p.projects)
	if roots == nil {
		roots = []string{}
	}
	return ProviderState{
		ProviderID: rubocopProviderID, Availability: p.availability, Phase: "idle", Freshness: "uncertain",
		Coverage:           Coverage{Kind: "openedDocuments", Roots: roots, Partial: p.partial, ExternalChanges: CoverageExternalChanges},
		Notice:             shorten(notice, 2048),
		DocumentExtensions: slices.Clone(rubyDocumentExtensions),
		MissingTool:        p.availability == "missingTool",
	}
}

func (p *RubocopPlan) covers(file string) bool { return coversDir(p.projects, file) }

// rubyWorkspace is a Ruby workspace's metadata and resolved interpreter,
// shared by the RuboCop and ruby-lsp adapters.
type rubyWorkspace struct {
	w                   tsWorkspace
	base                string
	lock                gemLockfile
	lockPresent, lockOK bool
	rt                  rubyRuntime
}

// inspectRubyWorkspace reads the workspace's markers and lockfile and resolves
// its Ruby. state and notice are set when the tool cannot run there; tool names
// it in the notices ("RuboCop", "ruby-lsp") and minRuby is the oldest Ruby it
// supports, named in them.
func inspectRubyWorkspace(root AuthorizedRoot, reviewedPath []string, env RubyEnvironment, workspace *tsWorkspace, markers projectMarkers, tool, minRuby string) (rw rubyWorkspace, state, notice string, err error) {
	rw.w, err = workspaceMetadata(workspace, func() (tsWorkspace, error) { return discoverProjectDirs(root, markers) })
	if err != nil {
		return rw, "", "", err
	}
	if !rw.w.sources {
		return rw, "unsupported", "No Ruby file was found in this project.", nil
	}
	rw.base = root.Project().Root
	rw.lock, rw.lockPresent, rw.lockOK = readGemfileLock(filepath.Join(rw.base, "Gemfile.lock"), rw.base)
	locked := ""
	if rw.lockOK {
		locked = rw.lock.rubyVersion
	}
	rt := resolveRuby(append([]string{rw.base}, rw.w.ancestors...), locked, reviewedPath, env)
	rw.rt = rt
	requestedBy := func() string {
		if rel, err := filepath.Rel(rw.base, rt.RequestFile); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
		return rt.RequestFile
	}
	switch rt.State {
	case "missing":
		return rw, "missingTool", "Ruby, required by " + tool + ", is not installed. Install Ruby " + minRuby + " or newer (for example with mise, rbenv or Homebrew), then refresh.", nil
	case "foreign":
		return rw, "unsupported", "The ruby commands found are version-manager shims, scripts or installations the app cannot identify. " + rubyShimHint, nil
	case "notInstalled":
		return rw, "missingTool", "Ruby " + rt.Request + ", requested by " + requestedBy() + ", is not installed. Install it with your version manager, then refresh.", nil
	case "invalidRequest":
		return rw, "unsupported", requestedBy() + " does not name a Ruby version the app can use; only plain CRuby versions such as 3.3.5 are read.", nil
	case "tooOld":
		notice := "Ruby " + rt.Version + " (" + rt.Interpreter + ") is too old for " + tool + "; version " + minRuby + " or newer is required."
		if rt.ShimSkipped {
			notice += " " + rubyShimHint
		}
		return rw, "unsupported", notice, nil
	}
	return rw, "", "", nil
}

// rubyApprovalFiles are the workspace files bound into a Ruby server's
// approval: the markers found, .bundle/config when present and the version
// file inside the workspace (sorted). versionFile is an ancestor's version
// file outside the workspace, bound as a toolchain input instead.
func rubyApprovalFiles(rw rubyWorkspace) (configs []string, versionFile string) {
	configs = slices.Clone(rw.w.configs)
	if _, present, _ := readBoundedFile(filepath.Join(rw.base, ".bundle", "config"), rw.base, maxBundleConfig); present {
		configs = append(configs, ".bundle/config")
	}
	if rw.rt.RequestFile != "" {
		if rel, err := filepath.Rel(rw.base, rw.rt.RequestFile); err == nil && filepath.IsLocal(rel) {
			if rel = filepath.ToSlash(rel); !slices.Contains(configs, rel) {
				configs = append(configs, rel)
			}
		} else {
			versionFile = rw.rt.RequestFile
		}
	}
	slices.Sort(configs)
	return configs, versionFile
}

// RubyLanguageID is the extension ownership rule. Extensionless Ruby files
// (Gemfile, Rakefile) and ERB templates are not owned.
func RubyLanguageID(file string) (string, bool) {
	switch strings.ToLower(path.Ext(file)) {
	case ".rb", ".rake", ".gemspec", ".ru":
		return "ruby", true
	}
	return "", false
}

var rubyDocumentExtensions = []string{".rb", ".rake", ".gemspec", ".ru"}

// rubySkipped reports a directory the nested-workspace scan skips silently for
// Ruby: installed gems (vendor/bundle) and Rails' tmp and log beside a
// Gemfile. files are the entries already listed in the same directory;
// "Gemfile" sorts before both.
func rubySkipped(rel, name string, files []string) bool {
	if matchesSkipPath(rubocopProjectMarkers.skipPath, rel) {
		return true
	}
	beside := rubocopProjectMarkers.skipBeside[name]
	return beside != "" && slices.Contains(files, beside)
}

package codediagnostics

// Ruby navigation adapter: Shopify's ruby-lsp, for Go to Definition,
// references and hover. RuboCop (rubocop_lsp.go) stays the source of Ruby
// diagnostics; ruby-lsp publishes none here.
//
// ruby-lsp's own exe composes a bundle in <project>/.ruby-lsp, runs bundle
// install and loads add-ons, project ones included (RL-00). The app never runs
// it: the interpreter runs rubyLspWrapper, which loads the gem's library from
// ARGV[0], replaces RubyLsp::Server#load_addons with a no-op and starts the
// server in-process. The wrapper is part of argv, so its text is part of the
// approved policy. Ruby and the gem are found from metadata only
// (ruby_runtime.go, ruby_gems.go). ruby-lsp evaluates the project's Gemfile
// through Bundler.definition for its index; approval covers that, as for
// RuboCop's Bundler mode. Observed behavior: testdata/ruby-lsp/evidence.json
// and docs/plan/code-diagnostics-ruby-lsp.md. Tests in ruby_lsp_test.go are
// SYNTHETIC.

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const (
	rubyLspProviderID = "ruby-lsp"
	// rubyLspPolicy binds the launch shape: the resolved interpreter runs the
	// app's wrapper on the gem's lib directory, never exe/ruby-lsp, a shim or
	// bundle exec; add-ons are not loaded and only navigation is enabled.
	rubyLspPolicy          = "ruby-lsp-v1:min-" + rubyLspMinVersion + ":" + runtime.GOOS + "/" + runtime.GOARCH + ":ruby-wrapper:no-addons:navigation-only:no-edits:opened-documents"
	rubyLspToolchainPolicy = "ruby-lsp-toolchain-v1:loaded-file"
	// rubyLspWrapperExit is the wrapper's exit status for a ruby-lsp whose
	// RubyLsp::Server has no load_addons. Discovery rejects such a gem first
	// (state incompatible); this guards a gem changed after discovery.
	rubyLspWrapperExit = 3
	rubyLspScopeNotice = "ruby-lsp reads the project's Ruby files, Ruby's standard library and the gems in Gemfile.lock to answer Go to Definition, references and hover; it reports no problems. Add-ons are not loaded."
	rubyLspGemfile     = "It evaluates the project's Gemfile."
	rubyLspOpened      = "Ruby navigation covers opened documents only."
	rubyLspInstallHint = "Run gem install ruby-lsp, then refresh."
	// rubyLspReindex makes ruby-lsp index an open draft: it does so only while
	// answering one of its combined requests (RL-00). documentSymbols stays
	// enabled in rubyLspOptions for it.
	rubyLspReindex = "textDocument/documentSymbol"
)

func rubyLspIncompatible(version string) string {
	return "ruby-lsp " + version + " is not compatible with this app: the app cannot start it without add-ons."
}

// rubyLspWrapper starts ruby-lsp as its exe's in-process branch does, without
// add-ons. ARGV[0] is the gem's lib directory.
const rubyLspWrapper = `# agent-sessions: ruby-lsp without add-ons, navigation only. ARGV[0] is the gem's lib directory.
$stdin.sync = $stdout.sync = $stderr.sync = true
[$stdin, $stdout, $stderr].each(&:binmode)
$LOAD_PATH.unshift(ARGV.fetch(0))
require "ruby_lsp/internal"
unless RubyLsp::Server.method_defined?(:load_addons) || RubyLsp::Server.private_method_defined?(:load_addons)
  $stderr.puts "agent-sessions: this ruby-lsp is not compatible: RubyLsp::Server#load_addons is missing"
  exit 3
end
RubyLsp::Server.prepend(Module.new { def load_addons(*, **); end })
server = RubyLsp::Server.new
$> = $stderr
server.start
`

// rubyLspOptions turns off every ruby-lsp feature except navigation, its
// linters and its formatter. documentSymbols stays on: the app requests it to
// make ruby-lsp re-index an open draft (RL-04).
const rubyLspOptions = `{"enabledFeatures":{"definition":true,"hover":true,"documentSymbols":true,` +
	`"codeActions":false,"codeLens":false,"completion":false,"diagnostics":false,"documentHighlights":false,` +
	`"documentLink":false,"foldingRanges":false,"formatting":false,"inlayHint":false,"onTypeFormatting":false,` +
	`"selectionRanges":false,"semanticHighlighting":false,"signatureHelp":false,"typeHierarchy":false,` +
	`"workspaceSymbol":false},"linters":[],"formatter":"none"}`

// rubyLspProjectMarkers: Bundler's files select the Ruby, the gems indexed and
// a locked ruby-lsp, so they are bound into approval. Any Ruby source enables
// ruby-lsp; the skips are RuboCop's.
var rubyLspProjectMarkers = projectMarkers{
	configs:    []string{"Gemfile", "Gemfile.lock"},
	skipDir:    rubocopProjectMarkers.skipDir,
	skipPath:   rubocopProjectMarkers.skipPath,
	skipBeside: rubocopProjectMarkers.skipBeside,
	sources:    rubyDocumentExtensions,
}

// RubyLspPlan is the discovery result. Availability is independent of every
// other provider, RuboCop included.
type RubyLspPlan struct {
	root         AuthorizedRoot
	availability string
	notice       string
	config       ServerConfig
	version      string
	rubyVersion  string
	projects     []string
	partial      bool
	projectLocal bool
}

// DiscoverRubyLsp inspects metadata only. Errors are reserved for an invalid
// host description or a root that is no longer authorized.
func DiscoverRubyLsp(root AuthorizedRoot, reviewedPath []string, env RubyEnvironment) (*RubyLspPlan, error) {
	return discoverRubyLspWorkspace(root, reviewedPath, env, nil)
}

func discoverRubyLspWorkspace(root AuthorizedRoot, reviewedPath []string, env RubyEnvironment, workspace *tsWorkspace) (*RubyLspPlan, error) {
	if err := root.Revalidate(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(env.Home) || !bounded(env.Home, 4096) || hasParent(env.Home) {
		return nil, failure("invalidArgument", "A reviewed absolute home path is required")
	}
	p := &RubyLspPlan{root: root, availability: "available", notice: rubyLspScopeNotice}
	unavailable := func(state, notice string) (*RubyLspPlan, error) {
		p.availability, p.notice, p.partial = state, notice, true
		return p, nil
	}
	rw, state, notice, err := inspectRubyWorkspace(root, reviewedPath, env, workspace, rubyLspProjectMarkers, "ruby-lsp", rubyLspMinRuby)
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
	base, rt := rw.base, rw.rt
	p.rubyVersion = rt.Version
	inst := findRubyLsp(base, rw.lock, rw.lockPresent, rw.lockOK, rt, env)
	v := inst.RubyLsp.Version
	switch inst.State {
	case "missing":
		return unavailable("missingTool", "ruby-lsp is not installed for Ruby "+rt.Version+". "+rubyLspInstallHint)
	case "notInstalled":
		return unavailable("missingTool", "ruby-lsp "+inst.LockedVersion+" from Gemfile.lock is not installed. Run bundle install, then refresh.")
	case "missingDependency":
		return unavailable("missingTool", "ruby-lsp "+v+" needs the "+inst.Dependency+" gem, which is not installed for Ruby "+rt.Version+". Run gem install "+inst.Dependency+", then refresh.")
	case "rubyTooOld":
		return unavailable("unsupported", "Ruby "+rt.Version+" ("+rt.Interpreter+") is too old for ruby-lsp; version "+rubyLspMinRuby+" or newer is required.")
	case "tooOld":
		hint := "run gem update ruby-lsp, then refresh."
		if inst.LockedVersion != "" {
			hint = "run bundle update ruby-lsp, then refresh."
		}
		return unavailable("unsupported", "ruby-lsp "+v+" is older than "+rubyLspMinVersion+"; "+hint)
	case "incompatible":
		return unavailable("unsupported", rubyLspIncompatible(v))
	case "invalidGemspec":
		return unavailable("unsupported", "The ruby-lsp "+v+" gemspec could not be read.")
	case "unsupportedSource":
		return unavailable("unsupported", "Gemfile.lock takes ruby-lsp from a git or path source, which is not supported.")
	case "invalidLockfile":
		return unavailable("unsupported", "Gemfile.lock could not be read.")
	case "invalidBundleConfig":
		return unavailable("unsupported", ".bundle/config could not be read; only plain KEY: value settings are supported.")
	case "bundlePathOutside":
		return unavailable("unsupported", "BUNDLE_PATH in .bundle/config points outside the project.")
	}
	p.version = v
	p.projectLocal = within(base, inst.RubyLsp.Path)

	rubyBin := filepath.Dir(rt.Interpreter)
	for _, dir := range append([]string{rubyBin}, inst.GemDirs...) {
		if strings.Contains(dir, ":") {
			return unavailable("unsupported", "The Ruby or gem directory cannot be placed on a fixed search path.")
		}
	}
	configs, versionFile := rubyApprovalFiles(rw)
	if slices.Contains(configs, "Gemfile") {
		p.notice += " " + rubyLspGemfile
	}
	tools := []ToolInput{
		{ProviderID: "ruby-lsp-toolchain-entry", Executable: inst.RubyLsp.Entry, Policy: rubyLspToolchainPolicy},
		{ProviderID: "ruby-lsp-toolchain-gemspec", Executable: inst.RubyLsp.Spec, Policy: rubyLspToolchainPolicy},
		{ProviderID: "ruby-lsp-toolchain-server", Executable: inst.Server, Policy: rubyLspToolchainPolicy},
	}
	if versionFile != "" {
		tools = append(tools, ToolInput{ProviderID: "ruby-lsp-toolchain-version-file", Executable: versionFile, Policy: rubyLspToolchainPolicy})
	}
	p.config = ServerConfig{
		ProviderID: rubyLspProviderID,
		Executable: rt.Interpreter,
		Args:       []string{"-e", rubyLspWrapper, filepath.Join(inst.RubyLsp.Path, "lib")},
		// BUNDLE_GEMFILE keeps ruby-lsp from composing .ruby-lsp/ and running
		// bundle install, also when the Gemfile does not exist (RL-00 A2).
		Env: []string{
			"HOME=" + env.Home, "PATH=" + rubyBin + ":/usr/bin:/bin",
			"GEM_HOME=" + inst.RubyLsp.Home, "GEM_PATH=" + strings.Join(inst.GemDirs, ":"),
			"BUNDLE_GEMFILE=" + filepath.Join(base, "Gemfile"), "BUNDLE_FROZEN=true",
		},
		AdapterPolicy:         rubyLspPolicy,
		ConfigPaths:           configs,
		Toolchain:             tools,
		InitializationOptions: json.RawMessage(rubyLspOptions),
		ExitFailures:          map[int]string{rubyLspWrapperExit: rubyLspIncompatible(v)},
		Reindex:               rubyLspReindex,
		// The standard library, default and bundled gems are under lib/ruby;
		// every other gem, and RBS core types in the rbs gem, under GEM_PATH.
		ExternalRoots: append([]string{filepath.Join(rt.Prefix, "lib", "ruby")}, inst.GemDirs...),
	}
	if _, err := Fingerprint(root, p.config.Tools()); err != nil {
		if err := root.Revalidate(); err != nil {
			return nil, err
		}
		p.config = ServerConfig{}
		return unavailable("unsupported", "The Ruby or ruby-lsp files could not be identified for approval.")
	}
	return p, nil
}

func (p *RubyLspPlan) Availability() string { return p.availability }

// Version is ruby-lsp's gem version; RubyVersion the interpreter's.
func (p *RubyLspPlan) Version() string     { return p.version }
func (p *RubyLspPlan) RubyVersion() string { return p.rubyVersion }
func (p *RubyLspPlan) Projects() []string  { return slices.Clone(p.projects) }

// ProjectLocal reports that ruby-lsp itself is project content (vendor/bundle).
func (p *RubyLspPlan) ProjectLocal() bool { return p.projectLocal }

// Server is the fixed server identity, only when the plan is available.
func (p *RubyLspPlan) Server() (ServerConfig, bool) {
	return p.config, p.availability == "available"
}

// Provider describes opened-documents navigation. ruby-lsp publishes no
// diagnostics here, so it never claims a document is clean.
func (p *RubyLspPlan) Provider() ProviderState {
	notice := rubyLspOpened
	if p.notice != "" {
		notice = p.notice + " " + rubyLspOpened
	}
	roots := slices.Clone(p.projects)
	if roots == nil {
		roots = []string{}
	}
	return ProviderState{
		ProviderID: rubyLspProviderID, Availability: p.availability, Phase: "idle", Freshness: "uncertain",
		Coverage:           Coverage{Kind: "openedDocuments", Roots: roots, Partial: p.partial, ExternalChanges: CoverageExternalChanges},
		Notice:             shorten(notice, 2048),
		DocumentExtensions: slices.Clone(rubyDocumentExtensions),
		MissingTool:        p.availability == "missingTool",
	}
}

func (p *RubyLspPlan) covers(file string) bool { return coversDir(p.projects, file) }

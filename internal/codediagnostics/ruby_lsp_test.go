package codediagnostics

// SYNTHETIC fixtures: the fake interpreters, gems, project trees and fake
// server below prove this adapter's handling only. Nothing is executed and
// nothing here claims real-server behavior. Observed ruby-lsp behavior is
// recorded in testdata/ruby-lsp/evidence.json; see
// docs/plan/code-diagnostics-ruby-lsp.md.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// rubyLspUser is the rubyLspHost interpreter with ruby-lsp 0.26.11 in the
// user gem directory, as RL-00 installed it.
func rubyLspUser(t *testing.T) (rubyRuntime, RubyEnvironment, string) {
	t.Helper()
	rt, env := rubyLspHost(t)
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	writeRubyLsp(t, user, "0.26.11")
	return rt, env, user
}

func TestDiscoverRubyLsp(t *testing.T) {
	rt, env, user := rubyLspUser(t)
	gem := filepath.Join(user, "gems/ruby-lsp-0.26.11")
	interpreterGems := filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0")
	bin := filepath.Join(rt.Prefix, "bin")
	root := rubocopRoot(t, map[string]string{
		"Gemfile": "source \"https://rubygems.org\"\n", "Gemfile.lock": lockWith("1.92.2", ""),
		".rubocop.yml": "", ".ruby-version": "4.0.9\n",
	})
	base := root.Project().Root

	p, err := DiscoverRubyLsp(root, []string{bin}, env)
	if err != nil || p.Availability() != "available" || p.Version() != "0.26.11" || p.RubyVersion() != "4.0.9" || p.ProjectLocal() || !slices.Equal(p.Projects(), []string{""}) {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, ok := p.Server()
	// The interpreter runs the app's wrapper on the gem's lib directory, never
	// exe/ruby-lsp, a shim or bundle exec, and without -rbundler/setup.
	if !ok || cfg.ProviderID != rubyLspProviderID || cfg.Executable != rt.Interpreter ||
		!slices.Equal(cfg.Args, []string{"-e", rubyLspWrapper, filepath.Join(gem, "lib")}) ||
		!slices.Equal(cfg.Env, []string{
			"HOME=" + env.Home, "PATH=" + bin + ":/usr/bin:/bin",
			"GEM_HOME=" + user, "GEM_PATH=" + user + ":" + interpreterGems,
			"BUNDLE_GEMFILE=" + filepath.Join(base, "Gemfile"), "BUNDLE_FROZEN=true",
		}) ||
		// RuboCop's configuration is not ruby-lsp's input.
		!slices.Equal(cfg.ConfigPaths, []string{".ruby-version", "Gemfile", "Gemfile.lock"}) ||
		cfg.Reindex != "textDocument/documentSymbol" ||
		!slices.Equal(cfg.ExternalRoots, []string{filepath.Join(rt.Prefix, "lib/ruby"), user, interpreterGems}) ||
		cfg.Settings != nil || cfg.DeclareDiagnostics || cfg.AcknowledgeWatchers || cfg.ValidateStart != nil || cfg.Directory != "" ||
		cfg.AdapterPolicy != "ruby-lsp-v1:min-0.26:"+runtime.GOOS+"/"+runtime.GOARCH+":ruby-wrapper:no-addons:navigation-only:no-edits:opened-documents" ||
		!reflect.DeepEqual(cfg.ExitFailures, map[int]string{3: "ruby-lsp 0.26.11 is not compatible with this app: the app cannot start it without add-ons."}) {
		t.Fatalf("%+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Toolchain, []ToolInput{
		{ProviderID: "ruby-lsp-toolchain-entry", Executable: filepath.Join(gem, "lib/ruby_lsp/internal.rb"), Policy: rubyLspToolchainPolicy},
		{ProviderID: "ruby-lsp-toolchain-gemspec", Executable: filepath.Join(user, "specifications/ruby-lsp-0.26.11.gemspec"), Policy: rubyLspToolchainPolicy},
		{ProviderID: "ruby-lsp-toolchain-server", Executable: filepath.Join(gem, "lib/ruby_lsp/server.rb"), Policy: rubyLspToolchainPolicy},
	}) {
		t.Fatalf("%+v", cfg.Toolchain)
	}
	if err := validateCommand(root, cfg.Tools(), cfg.command()); err != nil {
		t.Fatal(err)
	}
	if pol := cfg.command().AdapterPolicy; !strings.HasPrefix(pol, rubyLspPolicy+"|init-options:") || len(pol) > 2048 {
		t.Fatalf("%d %s", len(pol), pol)
	}
	st := p.Provider()
	if st.ProviderID != rubyLspProviderID || st.Availability != "available" || st.Coverage.Kind != "openedDocuments" || st.Coverage.Partial ||
		!slices.Equal(st.Coverage.Roots, []string{""}) || !slices.Equal(st.DocumentExtensions, []string{".rb", ".rake", ".gemspec", ".ru"}) || st.MissingTool ||
		st.Notice != "ruby-lsp reads the project's Ruby files, Ruby's standard library and the gems in Gemfile.lock to answer Go to Definition, references and hover; it reports no problems. Add-ons are not loaded. It evaluates the project's Gemfile. Ruby navigation covers opened documents only." {
		t.Fatalf("%+v", st)
	}
	if !p.covers("lib/a.rb") {
		t.Fatal("coverage")
	}

	// The bound gem files and project files change approval.
	before, _ := Fingerprint(root, cfg.Tools())
	for _, change := range []func(){
		func() { writeTree(t, base, map[string]string{"Gemfile": "source \"https://example.com\"\n"}) },
		func() { writeTree(t, gem, map[string]string{"lib/ruby_lsp/server.rb": rubyLspServer + "# changed\n"}) },
		func() { writeTree(t, gem, map[string]string{"lib/ruby_lsp/internal.rb": "# changed\n"}) },
		func() {
			writeTree(t, user, map[string]string{"specifications/ruby-lsp-0.26.11.gemspec": rubyLspSpec + "# changed\n"})
		},
	} {
		change()
		after, err := Fingerprint(root, cfg.Tools())
		if err != nil || after == before {
			t.Fatal("change kept the fingerprint", err)
		}
		before = after
	}
}

// Without a Gemfile, BUNDLE_GEMFILE still names <workspace>/Gemfile: that
// keeps ruby-lsp from composing .ruby-lsp/ (RL-00 A2). The notice then makes
// no Gemfile claim.
func TestDiscoverRubyLspWithoutGemfile(t *testing.T) {
	rt, env, _ := rubyLspUser(t)
	root := rubocopRoot(t, nil)
	p, err := DiscoverRubyLsp(root, []string{filepath.Join(rt.Prefix, "bin")}, env)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, _ := p.Server()
	if !slices.Contains(cfg.Env, "BUNDLE_GEMFILE="+filepath.Join(root.Project().Root, "Gemfile")) || !slices.Contains(cfg.Env, "BUNDLE_FROZEN=true") ||
		len(cfg.ConfigPaths) != 0 || strings.Contains(p.Provider().Notice, rubyLspGemfile) {
		t.Fatalf("%+v %q", cfg, p.Provider().Notice)
	}
	if err := validateCommand(root, cfg.Tools(), cfg.command()); err != nil {
		t.Fatal(err)
	}
}

// A ruby-lsp locked in Gemfile.lock and installed in the project's
// BUNDLE_PATH is project content; it still loads from GEM_PATH, never through
// the bundle, and the shared directories follow BUNDLE_PATH.
func TestDiscoverRubyLspLockedProjectLocal(t *testing.T) {
	rt, env := rubyLspHost(t)
	root := rubocopRoot(t, map[string]string{
		"Gemfile": "", "Gemfile.lock": lockRubyLsp("0.26.11"), ".bundle/config": "---\nBUNDLE_PATH: \"vendor/bundle\"\n",
	})
	base := root.Project().Root
	vendor := filepath.Join(base, "vendor/bundle/ruby/4.0.0")
	writeRubyLsp(t, vendor, "0.26.11")
	writeRubyLsp(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "0.27.0") // newer, but not locked
	p, err := DiscoverRubyLsp(root, []string{filepath.Join(rt.Prefix, "bin")}, env)
	if err != nil || p.Availability() != "available" || p.Version() != "0.26.11" || !p.ProjectLocal() {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, _ := p.Server()
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	if !slices.Contains(cfg.Env, "GEM_HOME="+vendor) ||
		!slices.Contains(cfg.Env, "GEM_PATH="+vendor+":"+user+":"+filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0")) ||
		cfg.Args[2] != filepath.Join(vendor, "gems/ruby-lsp-0.26.11/lib") || slices.Contains(cfg.Args, "-rbundler/setup") ||
		!slices.Equal(cfg.ConfigPaths, []string{".bundle/config", "Gemfile", "Gemfile.lock"}) {
		t.Fatalf("%+v", cfg)
	}
}

func TestRubyLspWrapperAndOptions(t *testing.T) {
	// The wrapper does what the plan says, in order.
	steps := []string{
		"$stdin.sync = $stdout.sync = $stderr.sync = true\n",
		"[$stdin, $stdout, $stderr].each(&:binmode)\n",
		"$LOAD_PATH.unshift(ARGV.fetch(0))\n",
		"require \"ruby_lsp/internal\"\n",
		"unless RubyLsp::Server.method_defined?(:load_addons) || RubyLsp::Server.private_method_defined?(:load_addons)\n",
		"exit 3\n",
		"RubyLsp::Server.prepend(Module.new { def load_addons(*, **); end })\n",
		"server = RubyLsp::Server.new\n",
		"$> = $stderr\n",
		"server.start\n",
	}
	at := 0
	for _, step := range steps {
		i := strings.Index(rubyLspWrapper[at:], step)
		if i < 0 {
			t.Fatalf("wrapper lacks %q after offset %d", step, at)
		}
		at += i + len(step)
	}
	if rubyLspWrapperExit != 3 || strings.Contains(rubyLspWrapper, "exe/ruby-lsp") || strings.Contains(rubyLspWrapper, "bundler") || strings.ContainsRune(rubyLspWrapper, 0) {
		t.Fatal("wrapper")
	}

	// Every feature RL-00 saw in ruby-lsp 0.26.11 is named; only navigation
	// and the documentSymbol re-index stay on.
	var opts struct {
		EnabledFeatures map[string]bool
		Linters         []string
		Formatter       string
	}
	if err := json.Unmarshal([]byte(rubyLspOptions), &opts); err != nil || opts.Linters == nil || len(opts.Linters) != 0 || opts.Formatter != "none" {
		t.Fatal(err, opts)
	}
	var on, off []string
	for k, v := range opts.EnabledFeatures {
		if v {
			on = append(on, k)
		} else {
			off = append(off, k)
		}
	}
	slices.Sort(on)
	slices.Sort(off)
	if !slices.Equal(on, []string{"definition", "documentSymbols", "hover"}) || !slices.Equal(off, []string{
		"codeActions", "codeLens", "completion", "diagnostics", "documentHighlights", "documentLink", "foldingRanges", "formatting",
		"inlayHint", "onTypeFormatting", "selectionRanges", "semanticHighlighting", "signatureHelp", "typeHierarchy", "workspaceSymbol",
	}) {
		t.Fatal(on, off)
	}

	// The wrapper text and the options are part of the approved policy.
	rt, env, _ := rubyLspUser(t)
	p, err := DiscoverRubyLsp(rubocopRoot(t, nil), []string{filepath.Join(rt.Prefix, "bin")}, env)
	if err != nil || p.Availability() != "available" {
		t.Fatal(err, p.Provider().Notice)
	}
	cfg, _ := p.Server()
	wrapper := cfg
	wrapper.Args = slices.Clone(cfg.Args)
	wrapper.Args[1] = strings.Replace(rubyLspWrapper, "def load_addons(*, **); end", "def load_addons(*, **) = super", 1)
	options := cfg
	options.InitializationOptions = json.RawMessage(strings.Replace(rubyLspOptions, `"completion":false`, `"completion":true`, 1))
	exits := cfg
	exits.ExitFailures = nil
	if wrapper.Policy() == cfg.Policy() || options.Policy() == cfg.Policy() || exits.Policy() != cfg.Policy() {
		t.Fatal("policy binding")
	}
}

func TestDiscoverRubyLspUnavailable(t *testing.T) {
	type host struct {
		path []string
		env  RubyEnvironment
	}
	installed := func(versions ...string) func() host {
		return func() host {
			rt, env := rubyLspHost(t)
			for _, v := range versions {
				writeRubyLsp(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), v)
			}
			return host{[]string{filepath.Join(rt.Prefix, "bin")}, env}
		}
	}
	changed := func(file, text string) func() host {
		return func() host {
			h := installed("0.26.11")()
			writeTree(t, filepath.Join(h.env.Home, ".gem/ruby/4.0.0"), map[string]string{file: text})
			return h
		}
	}
	noProtocol := func() host {
		h := installed("0.26.11")()
		if err := os.Remove(filepath.Join(h.env.Home, ".gem/ruby/4.0.0/specifications/language_server-protocol-3.17.0.6.gemspec")); err != nil {
			t.Fatal(err)
		}
		return h
	}
	ruby := func(version string) func() host {
		return func() host {
			home, env := rubyHost(t)
			prefix := filepath.Join(home, "ruby-"+version)
			writeRuby(t, prefix, version)
			abi := strings.Join(strings.Split(version, ".")[:2], ".") + ".0"
			writeRubyLsp(t, filepath.Join(prefix, "lib/ruby/gems", abi), "0.26.11")
			return host{[]string{filepath.Join(prefix, "bin")}, env}
		}
	}
	colonHome := func() host {
		rt, _ := rubyLspHost(t)
		base, _ := filepath.EvalSymlinks(t.TempDir())
		home := filepath.Join(base, "a:b")
		writeRubyLsp(t, filepath.Join(home, ".gem/ruby/4.0.0"), "0.26.11")
		writeTree(t, home, map[string]string{".gem/ruby/4.0.0/specifications/language_server-protocol-3.17.0.6.gemspec": ""})
		return host{[]string{filepath.Join(rt.Prefix, "bin")}, RubyEnvironment{Home: home}}
	}
	for name, c := range map[string]struct {
		host   func() host
		files  map[string]string
		state  string
		notice string
	}{
		"no Ruby source":  {installed("0.26.11"), nil, "unsupported", "No Ruby file was found in this project."},
		"not installed":   {installed(), nil, "missingTool", "ruby-lsp is not installed for Ruby 4.0.9. Run gem install ruby-lsp, then refresh."},
		"locked, absent":  {installed("0.26.11"), map[string]string{"Gemfile.lock": lockRubyLsp("0.27.1")}, "missingTool", "ruby-lsp 0.27.1 from Gemfile.lock is not installed. Run bundle install, then refresh."},
		"dependency":      {noProtocol, nil, "missingTool", "ruby-lsp 0.26.11 needs the language_server-protocol gem, which is not installed for Ruby 4.0.9. Run gem install language_server-protocol, then refresh."},
		"too old":         {installed("0.25.9"), nil, "unsupported", "ruby-lsp 0.25.9 is older than 0.26; run gem update ruby-lsp, then refresh."},
		"locked, too old": {installed("0.25.9"), map[string]string{"Gemfile.lock": lockRubyLsp("0.25.9")}, "unsupported", "ruby-lsp 0.25.9 is older than 0.26; run bundle update ruby-lsp, then refresh."},
		"incompatible":    {changed("gems/ruby-lsp-0.26.11/lib/ruby_lsp/server.rb", "module RubyLsp\n  class Server\n  end\nend\n"), nil, "unsupported", "ruby-lsp 0.26.11 is not compatible with this app: the app cannot start it without add-ons."},
		"gemspec":         {changed("specifications/ruby-lsp-0.26.11.gemspec", strings.Repeat("#", maxGemspec+1)), nil, "unsupported", "The ruby-lsp 0.26.11 gemspec could not be read."},
		"git source":      {installed("0.26.11"), map[string]string{"Gemfile.lock": "GIT\n  remote: x\n  specs:\n    ruby-lsp (0.26.11)\n"}, "unsupported", "Gemfile.lock takes ruby-lsp from a git or path source, which is not supported."},
		"bundle path":     {installed("0.26.11"), map[string]string{"Gemfile.lock": lockRubyLsp("0.26.11"), ".bundle/config": "BUNDLE_PATH: \"../x\"\n"}, "unsupported", "BUNDLE_PATH in .bundle/config points outside the project."},
		"Ruby 2.7":        {ruby("2.7.8"), nil, "unsupported", "is too old for ruby-lsp; version 3.0 or newer is required."},
		"Ruby 2.6":        {ruby("2.6.10"), nil, "unsupported", "is too old for ruby-lsp; version 3.0 or newer is required."},
		"colon":           {colonHome, nil, "unsupported", "The Ruby or gem directory cannot be placed on a fixed search path."},
	} {
		h := c.host()
		root := rubocopRoot(t, c.files)
		if name == "no Ruby source" {
			root = goplsRoot(t, map[string]string{"Gemfile": ""})
		}
		p, err := DiscoverRubyLsp(root, h.path, h.env)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		st := p.Provider()
		cfg, ok := p.Server()
		if p.Availability() != c.state || st.Availability != c.state || !strings.Contains(st.Notice, c.notice) || ok || cfg.Executable != "" ||
			st.MissingTool != (c.state == "missingTool") || !st.Coverage.Partial {
			t.Errorf("%s: %s %q %+v", name, p.Availability(), st.Notice, cfg)
		}
	}
}

// The wrapper's exit 3 (a ruby-lsp changed after discovery so that it no
// longer has load_addons) reads as an unsupported version, not a crash.
// Other exits keep the generic failure.
func TestRubyLspWrapperExitMapsToUnsupported(t *testing.T) {
	for _, c := range []struct {
		code       int
		state, msg string
	}{
		{3, "unsupported", "ruby-lsp 0.26.11 is not compatible with this app: the app cannot start it without add-ons."},
		{1, "toolFailed", "Language server exited"},
	} {
		e := newEnv(t, SessionOptions{})
		e.script = func(f *fakeSrv) {
			_, _ = f.read() // initialize, unanswered
			f.exit(c.code)
		}
		root, cfg := e.project()
		cfg.ExitFailures = map[int]string{rubyLspWrapperExit: rubyLspIncompatible("0.26.11")}
		_, err := e.mgr.Attach(context.Background(), root, cfg)
		var got *Error
		if !errors.As(err, &got) || got.Code != c.state || got.Message != c.msg {
			t.Fatalf("exit %d: %v", c.code, err)
		}
		eventually(t, "failed snapshot", func() bool {
			info := e.mgr.Info(root, cfg)
			var snap *Error
			return info.Phase == phaseFailed && errors.As(info.Err, &snap) && snap.Code == c.state && snap.Message == c.msg
		})
	}
}

// rubyLspLiveEnv discovers Ruby alone, every other language off, with RuboCop
// and ruby-lsp both installed in the user gem directory.
func rubyLspLiveEnv(t *testing.T) (LocalLiveEnvironment, rubyRuntime) {
	t.Helper()
	rt, env, user := rubyLspUser(t)
	writeGem(t, user, "rubocop", "1.92.2")
	return LocalLiveEnvironment{Ruby: env, Disabled: NewDisabledLanguages([]string{"java", "php", "go", "typescript", "svelte", "python", "rust"})}, rt
}

// RuboCop and ruby-lsp share the Ruby workspaces: one ruby-lsp server per
// Gemfile, its own approval inputs, and both own each opened Ruby file.
func TestRubyLspLocalLive(t *testing.T) {
	env, rt := rubyLspLiveEnv(t)
	root := goplsRoot(t, map[string]string{
		".ruby-version": "4.0.9\n", "Gemfile": "", "Gemfile.lock": lockWith("1.92.2", ""), ".rubocop.yml": "", "lib/a.rb": "",
		"api/Gemfile": "", "api/.rubocop.yml": "", "api/app/a.rb": "",
		"api/vendor/bundle/ruby/4.0.0/gems/x-1/Gemfile": "", "api/vendor/bundle/ruby/4.0.0/gems/x-1/lib/x.rb": "",
		"api/.ruby-lsp/Gemfile": "", "api/.ruby-lsp/main_lsp.rb": "",
		"docs/Gemfile": "gem \"jekyll\"\n", "docs/index.md": "",
	})
	regs, err := DiscoverLocalLive(root, env)
	if err != nil || len(regs) != 2 || regs[0].ProviderID != rubocopProviderID || regs[1].ProviderID != rubyLspProviderID ||
		regs[0].Availability != "available" || regs[1].Availability != "available" {
		t.Fatal(err, regs)
	}
	cop, lsp := regs[0].Live, regs[1].Live
	if !slices.Equal(lsp.State.Coverage.Roots, []string{"", "api"}) || !slices.Equal(cop.State.Coverage.Roots, lsp.State.Coverage.Roots) ||
		lsp.State.Coverage.Partial || lsp.State.Coverage.Kind != "openedDocuments" || len(lsp.Workspaces) != 2 {
		t.Fatalf("%+v", lsp.State)
	}
	top, api := lsp.Workspaces[0].Server, lsp.Workspaces[1].Server
	version := ToolInput{ProviderID: "ruby-lsp-toolchain-version-file", Executable: filepath.Join(root.Project().Root, ".ruby-version"), Policy: rubyLspToolchainPolicy}
	hasVersion := func(c ServerConfig) bool {
		return slices.ContainsFunc(c.Toolchain, func(ti ToolInput) bool { return reflect.DeepEqual(ti, version) })
	}
	// Bundler's files only: .rubocop.yml is RuboCop's input, not ruby-lsp's.
	if top.ProviderID != rubyLspProviderID || top.Directory != "" || top.Executable != rt.Interpreter ||
		!slices.Equal(top.ConfigPaths, []string{".ruby-version", "Gemfile", "Gemfile.lock", "docs/Gemfile"}) || hasVersion(top) ||
		!slices.Contains(top.Env, "BUNDLE_GEMFILE="+filepath.Join(root.Project().Root, "Gemfile")) {
		t.Fatalf("root %+v", top)
	}
	if api.Directory != "api" || !slices.Equal(api.ConfigPaths, []string{"api/Gemfile"}) || !hasVersion(api) ||
		!slices.Contains(api.Env, "BUNDLE_GEMFILE="+filepath.Join(root.Project().Root, "api/Gemfile")) {
		t.Fatalf("api %+v", api)
	}
	docs, err := DiscoveryDocuments(root, regs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, dir := range map[string]string{"lib/a.rb": "", "api/app/a.rb": "api"} {
		all := docs.ResolveAll(root, file)
		if len(all) != 2 || all[0].Config.ProviderID != rubocopProviderID || all[1].Config.ProviderID != rubyLspProviderID ||
			all[0].Config.Directory != dir || all[1].Config.Directory != dir || all[0].LanguageID != "ruby" || all[1].LanguageID != "ruby" {
			t.Errorf("%s: %+v", file, all)
		}
		// Neither is the single owner.
		if _, ok := docs.Resolve(root, file); ok {
			t.Errorf("%s: single owner", file)
		}
	}
	if all := docs.ResolveAll(root, "docs/Gemfile"); len(all) != 0 {
		t.Fatal("Gemfile owned", all)
	}

	// Turning Ruby off stops both; nothing reads the Ruby environment.
	off := LocalLiveEnvironment{Ruby: RubyEnvironment{Home: "relative"}, Disabled: NewDisabledLanguages([]string{"java", "php", "go", "typescript", "svelte", "python", "rust", "ruby"})}
	if regs, err := DiscoverLocalLive(root, off); err != nil || len(regs) != 0 {
		t.Fatal(err, regs)
	}
	if disabled := DisabledRegistrations(root, off.Disabled); len(disabled) != 2 || disabled[1].ProviderID != rubyLspProviderID || disabled[1].Availability != "disabled" {
		t.Fatalf("%+v", disabled)
	}
}

// Without ruby-lsp, RuboCop keeps sole ownership; without RuboCop, ruby-lsp
// owns the file alone. Each is available independently of the other.
func TestRubyLspIndependentOfRubocop(t *testing.T) {
	files := map[string]string{"Gemfile": "", ".ruby-version": "4.0.9\n", "lib/a.rb": ""}
	for _, c := range []struct {
		name         string
		rubocop, lsp bool
		owner        string
	}{
		{"rubocop only", true, false, rubocopProviderID},
		{"ruby-lsp only", false, true, rubyLspProviderID},
	} {
		_, env := rubyLspHost(t)
		user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
		if c.rubocop {
			writeGem(t, user, "rubocop", "1.92.2")
		}
		if c.lsp {
			writeRubyLsp(t, user, "0.26.11")
		}
		root := goplsRoot(t, files)
		regs, err := DiscoverLocalLive(root, LocalLiveEnvironment{Ruby: env, Disabled: NewDisabledLanguages([]string{"java", "php", "go", "typescript", "svelte", "python", "rust"})})
		if err != nil || len(regs) != 2 || (regs[0].Availability == "available") != c.rubocop || (regs[1].Availability == "available") != c.lsp {
			t.Fatal(c.name, err, regs)
		}
		docs, err := DiscoveryDocuments(root, regs, nil)
		if err != nil {
			t.Fatal(err)
		}
		if s, ok := docs.Resolve(root, "lib/a.rb"); !ok || s.Config.ProviderID != c.owner {
			t.Errorf("%s: %+v %v", c.name, s, ok)
		}
	}
}

// With both owning a Ruby document, navigation goes to ruby-lsp (RuboCop
// advertises none, RB-00) and RuboCop's findings still arrive; ruby-lsp
// publishes nothing.
func TestRubyNavigationRoutesToRubyLsp(t *testing.T) {
	m, s, f := docFixture(t, incrementalCaps)
	lspCaps := incrementalCaps
	lspCaps.Definition, lspCaps.References, lspCaps.Hover = true, true, true
	p := &multiDocs{fakeDocs: f, isolated: map[string]bool{rubocopProviderID: true, rubyLspProviderID: true}, entries: []DocumentSession{
		{Owner: DocumentOwnerIdentity{rubocopProviderID, "policy-rubocop"}, Generation: "g-rubocop", Capabilities: incrementalCaps},
		{Owner: DocumentOwnerIdentity{rubyLspProviderID, "policy-ruby-lsp"}, Generation: "g-ruby-lsp", Capabilities: lspCaps},
	}}
	m.SetDocumentProvider(p)
	ctx, c := context.Background(), s.Context
	if _, err := m.OpenDocument(ctx, c, "source.go", "d1", 1, "def x; end\n", ""); err != nil {
		t.Fatal(err)
	}
	if len(f.steps) != 2 || f.steps[0].Owner == f.steps[1].Owner {
		t.Fatalf("opens: %+v", f.steps)
	}
	v := int64(1)
	if err := m.PublishPathDiagnostics(m.attachments[c.AttachmentID].root.Project(), rubocopProviderID, "g-rubocop", "source.go", &v,
		[]Diagnostic{{Path: "source.go", Source: rubocopProviderID, Code: "Lint/Syntax", Message: "x", Severity: "error"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetSnapshot(c)
	if len(got.ResultSets) != 1 || len(got.ResultSets[0].Diagnostics) != 1 || got.ResultSets[0].Diagnostics[0].Code != "Lint/Syntax" {
		t.Fatalf("%+v", got.ResultSets)
	}
	nav := &navDocs{MultiOwnedDocumentProvider: p}
	m.SetDocumentProvider(nav)
	for _, op := range []string{NavigateDefinition, NavigateReferences, NavigateHover} {
		if _, err := m.Navigate(ctx, c, "d1", 1, op, Position{}); err != nil {
			t.Fatal(op, err)
		}
		if req := nav.reqs[len(nav.reqs)-1]; req.Owner.ProviderID != rubyLspProviderID || req.Generation != "g-ruby-lsp" {
			t.Fatalf("%s went to %+v", op, req)
		}
	}
	// ruby-lsp not running: navigation is not handed to RuboCop.
	p.entries[1].Err = failure("staleContext", "Language session is not running")
	if _, err := m.Navigate(ctx, c, "d1", 1, NavigateDefinition, Position{}); errCode(err) != "notRunning" {
		t.Fatal(err)
	}
	if req := nav.reqs[len(nav.reqs)-1]; req.Owner.ProviderID != rubyLspProviderID {
		t.Fatal("RuboCop asked")
	}
}

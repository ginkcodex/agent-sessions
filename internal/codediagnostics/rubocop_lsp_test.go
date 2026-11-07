package codediagnostics

// SYNTHETIC fixtures: the fake interpreters, gems and project trees below prove
// this adapter's handling only. Nothing is executed and nothing here claims
// real-server behavior. Observed RuboCop behavior is recorded in
// testdata/rubocop/evidence.json; see docs/plan/code-diagnostics-ruby.md.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestRubyLanguageOwnership(t *testing.T) {
	for _, f := range []string{"a.rb", "lib/tasks/db.rake", "x.gemspec", "config.ru", "A.RB"} {
		if lang, ok := RubyLanguageID(f); !ok || lang != "ruby" {
			t.Errorf("%s: %q %v", f, lang, ok)
		}
	}
	for _, f := range []string{"Gemfile", "Rakefile", "a.erb", "a.html.erb", "a.rbs", "Gemfile.lock", "rb"} {
		if _, ok := RubyLanguageID(f); ok {
			t.Errorf("%s must not be owned", f)
		}
	}
}

// Installed gems, Bundler's and ruby-lsp's state, and a Rails app's tmp and
// log are skipped silently: not scanned, not bound, not partial.
func TestRubocopProjectDiscovery(t *testing.T) {
	cases := map[string]struct {
		files    map[string]string
		configs  []string
		sources  bool
		partial  bool
		packages []string
	}{
		"rails app": {map[string]string{
			"Gemfile": "", "Gemfile.lock": "", ".rubocop.yml": "", "app/models/user.rb": "",
			"vendor/bundle/ruby/4.0.0/gems/x-1/Gemfile":      "",
			"vendor/bundle/ruby/4.0.0/gems/x-1/.rubocop.yml": "",
			"tmp/cache/Gemfile":                              "", "log/.rubocop.yml": "",
			".bundle/config": "", ".ruby-lsp/Gemfile": "", "node_modules/x/Gemfile": "",
		}, []string{".rubocop.yml", "Gemfile", "Gemfile.lock"}, true, false, []string{""}},
		"tmp without a Gemfile is scanned": {map[string]string{
			"lib/a.rb": "", "tmp/.rubocop.yml": "", "sub/Gemfile": "", "sub/log/.rubocop_todo.yml": "",
		}, []string{"sub/Gemfile", "tmp/.rubocop.yml"}, true, false, []string{"sub", "tmp"}},
		"vendor without bundle is scanned": {map[string]string{
			"Gemfile": "", "vendor/plugins/x/.rubocop.yml": "",
		}, []string{"Gemfile", "vendor/plugins/x/.rubocop.yml"}, false, false, []string{"", "vendor/plugins/x"}},
		"ruby sources only": {map[string]string{"Rakefile": "", "tasks/a.rake": ""}, nil, true, false, nil},
	}
	for name, tc := range cases {
		w, err := discoverProjectDirs(goplsRoot(t, tc.files), rubocopProjectMarkers)
		if err != nil || !slices.Equal(w.configs, tc.configs) || w.sources != tc.sources || w.partial != tc.partial || !slices.Equal(w.packages, tc.packages) {
			t.Errorf("%s: %v configs=%q sources=%v partial=%v packages=%q", name, err, w.configs, w.sources, w.partial, w.packages)
		}
	}
}

// rubocopRoot is a SYNTHETIC project on the rubocopHost interpreter: files
// plus a Ruby source.
func rubocopRoot(t *testing.T, files map[string]string) AuthorizedRoot {
	t.Helper()
	all := map[string]string{"lib/a.rb": "puts 1\n"}
	for k, v := range files {
		all[k] = v
	}
	return goplsRoot(t, all)
}

func TestDiscoverRubocopStandalone(t *testing.T) {
	rt, env := rubocopHost(t)
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	entry := writeGem(t, user, "rubocop", "1.92.2")
	interpreterGems := filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0")
	root := rubocopRoot(t, map[string]string{".rubocop.yml": "AllCops:\n  NewCops: enable\n"})
	path := []string{filepath.Join(rt.Prefix, "bin")}

	p, err := DiscoverRubocop(root, path, env)
	if err != nil || p.Availability() != "available" || p.Version() != "1.92.2" || p.RubyVersion() != "4.0.9" || p.Mode() != "standalone" || p.ProjectLocal() {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, ok := p.Server()
	// The interpreter runs the gem's own exe/rubocop: no shell, shim or binstub.
	if !ok || cfg.ProviderID != rubocopProviderID || cfg.Executable != rt.Interpreter ||
		!slices.Equal(cfg.Args, []string{entry, "--lsp"}) ||
		!slices.Equal(cfg.Env, []string{"HOME=" + env.Home, "PATH=" + filepath.Join(rt.Prefix, "bin") + ":/usr/bin:/bin", "GEM_HOME=" + user, "GEM_PATH=" + user + ":" + interpreterGems}) ||
		!slices.Equal(cfg.ConfigPaths, []string{".rubocop.yml"}) || cfg.Settings != nil || cfg.DeclareDiagnostics ||
		cfg.command().AdapterPolicy != rubocopPolicy+"standalone"+rubocopPolicyTail {
		t.Fatalf("%+v", cfg)
	}
	if !reflect.DeepEqual(cfg.Toolchain, []ToolInput{
		{ProviderID: "rubocop-toolchain-entry", Executable: entry, Policy: rubocopToolchainPolicy},
		{ProviderID: "rubocop-toolchain-gemspec", Executable: filepath.Join(user, "specifications/rubocop-1.92.2.gemspec"), Policy: rubocopToolchainPolicy},
	}) {
		t.Fatalf("%+v", cfg.Toolchain)
	}
	if err := validateCommand(root, cfg.Tools(), cfg.command()); err != nil {
		t.Fatal(err)
	}
	st := p.Provider()
	if st.ProviderID != rubocopProviderID || st.Coverage.Kind != "openedDocuments" || st.Coverage.Partial || !slices.Equal(st.Coverage.Roots, []string{""}) ||
		!slices.Equal(st.DocumentExtensions, []string{".rb", ".rake", ".gemspec", ".ru"}) || st.MissingTool ||
		!strings.Contains(st.Notice, "not type errors") || !strings.Contains(st.Notice, "not part of approval") ||
		!strings.Contains(st.Notice, "opened documents only") || strings.Contains(st.Notice, "Bundler") {
		t.Fatalf("%+v", st)
	}
	if !p.covers("lib/a.rb") {
		t.Fatal("coverage")
	}
	// A changed RuboCop file or project configuration changes approval.
	before, _ := Fingerprint(root, cfg.Tools())
	writeTree(t, root.Project().Root, map[string]string{".rubocop.yml": "AllCops:\n  NewCops: disable\n"})
	mid, err := Fingerprint(root, cfg.Tools())
	if err != nil || mid == before {
		t.Fatal("config change kept the fingerprint", err)
	}
	writeTree(t, user, map[string]string{"gems/rubocop-1.92.2/exe/rubocop": "# changed\n"})
	if after, err := Fingerprint(root, cfg.Tools()); err != nil || after == mid {
		t.Fatal("entry change kept the fingerprint", err)
	}

	// A .ruby-version request is bound as project configuration.
	root = rubocopRoot(t, map[string]string{".ruby-version": "4.0.9\n"})
	if p, err = DiscoverRubocop(root, nil, env); err != nil || p.Availability() != "available" {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	if cfg, _ = p.Server(); cfg.Executable != rt.Interpreter || !slices.Equal(cfg.ConfigPaths, []string{".ruby-version"}) {
		t.Fatalf("%+v", cfg)
	}
}

func TestDiscoverRubocopBundler(t *testing.T) {
	rt, env := rubocopHost(t)
	root := rubocopRoot(t, map[string]string{
		"Gemfile":        "source \"https://rubygems.org\"\ngem \"rubocop\"\n",
		"Gemfile.lock":   lockWith("1.92.2", "4.0.20"),
		".bundle/config": "---\nBUNDLE_PATH: \"vendor/bundle\"\n",
		".ruby-version":  "4.0.9\n",
		".rubocop.yml":   "",
	})
	base := root.Project().Root
	vendor := filepath.Join(base, "vendor/bundle/ruby/4.0.0")
	entry := writeGem(t, vendor, "rubocop", "1.92.2")
	// The newest Bundler in the user directory is still preferred.
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	writeGem(t, user, "rubocop", "1.99.0")
	bundlerSetup := filepath.Join(rt.Prefix, "lib/ruby/4.0.0/bundler/setup.rb")

	p, err := DiscoverRubocop(root, nil, env)
	if err != nil || p.Availability() != "available" || p.Version() != "1.92.2" || p.Mode() != "bundler" || !p.ProjectLocal() {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, ok := p.Server()
	if !ok || cfg.Executable != rt.Interpreter ||
		!slices.Equal(cfg.Args, []string{"-rbundler/setup", entry, "--lsp"}) ||
		!slices.Equal(cfg.Env, []string{"HOME=" + env.Home, "PATH=" + filepath.Join(rt.Prefix, "bin") + ":/usr/bin:/bin", "BUNDLE_GEMFILE=" + filepath.Join(base, "Gemfile"), "BUNDLE_FROZEN=true"}) ||
		!slices.Equal(cfg.ConfigPaths, []string{".bundle/config", ".rubocop.yml", ".ruby-version", "Gemfile", "Gemfile.lock"}) ||
		cfg.command().AdapterPolicy != rubocopPolicy+"bundler"+rubocopPolicyTail {
		t.Fatalf("%+v", cfg)
	}
	defaultSpec := filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0/specifications/default/bundler-4.0.20.gemspec")
	if !reflect.DeepEqual(cfg.Toolchain, []ToolInput{
		{ProviderID: "rubocop-toolchain-entry", Executable: entry, Policy: rubocopToolchainPolicy},
		{ProviderID: "rubocop-toolchain-gemspec", Executable: filepath.Join(vendor, "specifications/rubocop-1.92.2.gemspec"), Policy: rubocopToolchainPolicy},
		{ProviderID: "rubocop-toolchain-bundler-entry", Executable: bundlerSetup, Policy: rubocopToolchainPolicy},
		{ProviderID: "rubocop-toolchain-bundler-gemspec", Executable: defaultSpec, Policy: rubocopToolchainPolicy},
	}) {
		t.Fatalf("%+v", cfg.Toolchain)
	}
	if err := validateCommand(root, cfg.Tools(), cfg.command()); err != nil {
		t.Fatal(err)
	}
	if st := p.Provider(); !strings.Contains(st.Notice, "loads the project's Gemfile") || st.Coverage.Partial {
		t.Fatalf("%+v", st)
	}
	// The Gemfile is Ruby code Bundler runs: changing it, or the Bundler
	// configuration, changes approval.
	before, _ := Fingerprint(root, cfg.Tools())
	for _, change := range []map[string]string{
		{"Gemfile": "gem \"rubocop\"\nsystem(\"x\")\n"},
		{".bundle/config": "---\nBUNDLE_PATH: \"vendor/bundle\"\nBUNDLE_WITHOUT: \"x\"\n"},
	} {
		writeTree(t, base, change)
		after, err := Fingerprint(root, cfg.Tools())
		if err != nil || after == before {
			t.Fatal("change kept the fingerprint", change, err)
		}
		before = after
	}

	// The same project in standalone mode is a different approval.
	standalone := rubocopRoot(t, map[string]string{".ruby-version": "4.0.9\n"})
	sp, _ := DiscoverRubocop(standalone, nil, env)
	if scfg, _ := sp.Server(); sp.Mode() != "standalone" || scfg.Policy() == cfg.Policy() {
		t.Fatal("mode is not part of the policy", scfg.Policy())
	}

	// An older locked Bundler that is installed is selected explicitly.
	writeGem(t, user, "bundler", "2.6.9")
	writeTree(t, base, map[string]string{"Gemfile.lock": lockWith("1.92.2", "2.6.9")})
	p, _ = DiscoverRubocop(root, nil, env)
	if cfg, _ = p.Server(); !slices.Contains(cfg.Env, "BUNDLER_VERSION=2.6.9") || cfg.Toolchain[2].Executable != filepath.Join(user, "gems/bundler-2.6.9/lib/bundler/setup.rb") {
		t.Fatalf("%+v", cfg)
	}
}

// A nested workspace's version file above its scoped root is bound as a
// toolchain file, since project configuration paths cannot leave the root.
func TestDiscoverRubocopAncestorVersionFile(t *testing.T) {
	rt, env := rubocopHost(t)
	writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
	parent := goplsRoot(t, map[string]string{".ruby-version": "4.0.9\n", "svc/Gemfile": "", "svc/a.rb": ""})
	scoped, err := scopedDiscoveryRoot(parent, "svc")
	if err != nil {
		t.Fatal(err)
	}
	w, err := discoverProjectDirs(scoped, rubocopProjectMarkers)
	if err != nil {
		t.Fatal(err)
	}
	w.ancestors = []string{parent.Project().Root}
	p, err := discoverRubocopWorkspace(scoped, nil, env, &w)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %s %q", err, p.Availability(), p.Provider().Notice)
	}
	cfg, _ := p.Server()
	version := filepath.Join(parent.Project().Root, ".ruby-version")
	if cfg.Executable != rt.Interpreter || !slices.Equal(cfg.ConfigPaths, []string{"Gemfile"}) ||
		!slices.ContainsFunc(cfg.Toolchain, func(ti ToolInput) bool {
			return reflect.DeepEqual(ti, ToolInput{ProviderID: "rubocop-toolchain-version-file", Executable: version, Policy: rubocopToolchainPolicy})
		}) {
		t.Fatalf("%+v", cfg)
	}
	before, _ := Fingerprint(scoped, cfg.Tools())
	writeTree(t, parent.Project().Root, map[string]string{".ruby-version": "4.0.9 \n"})
	if after, err := Fingerprint(scoped, cfg.Tools()); err != nil || after == before {
		t.Fatal("ancestor version file is not bound", err)
	}
}

func TestDiscoverRubocopUnavailable(t *testing.T) {
	type host struct {
		path []string
		env  RubyEnvironment
	}
	// withRubocop is the rubocopHost interpreter with a user-installed RuboCop.
	withRubocop := func() host {
		rt, env := rubocopHost(t)
		writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
		return host{[]string{filepath.Join(rt.Prefix, "bin")}, env}
	}
	bare := func() host {
		rt, env := rubocopHost(t)
		return host{[]string{filepath.Join(rt.Prefix, "bin")}, env}
	}
	noBundler := func() host {
		home, env := rubyHost(t)
		prefix := filepath.Join(home, ".local/share/mise/installs/ruby/4.0.9")
		writeRuby(t, prefix, "4.0.9")
		writeGem(t, filepath.Join(home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
		return host{[]string{filepath.Join(prefix, "bin")}, env}
	}
	oldRuby := func(shim bool) func() host {
		return func() host {
			home, env := rubyHost(t)
			prefix := filepath.Join(home, "old")
			writeRuby(t, prefix, "2.6.10")
			path := []string{filepath.Join(prefix, "bin")}
			if shim {
				shims := filepath.Join(home, ".rbenv/shims")
				writeTree(t, shims, map[string]string{"ruby": "#!/usr/bin/env bash\nexec rbenv exec ruby \"$@\"\n"})
				_ = os.Chmod(filepath.Join(shims, "ruby"), 0o755)
				path = append([]string{shims}, path...)
			}
			return host{path, env}
		}
	}
	scriptOnly := func() host {
		home, env := rubyHost(t)
		shims := filepath.Join(home, ".rbenv/shims")
		writeTree(t, shims, map[string]string{"ruby": "#!/usr/bin/env bash\nexec rbenv exec ruby \"$@\"\n"})
		_ = os.Chmod(filepath.Join(shims, "ruby"), 0o755)
		return host{[]string{shims}, env}
	}
	colonHome := func() host {
		// Only the user gem directory, joined into GEM_PATH, has the colon.
		base, _ := filepath.EvalSymlinks(t.TempDir())
		home := filepath.Join(base, "a:b")
		prefix := filepath.Join(base, "ruby-4.0.9")
		writeRuby(t, prefix, "4.0.9")
		writeGem(t, filepath.Join(home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
		return host{[]string{filepath.Join(prefix, "bin")}, RubyEnvironment{Home: home}}
	}
	bundled := func(lock string) map[string]string { return map[string]string{"Gemfile": "", "Gemfile.lock": lock} }
	gitLock := "GIT\n  remote: https://example.test/rubocop.git\n  specs:\n    rubocop (1.92.2)\n\nPLATFORMS\n  ruby\n"

	for name, tc := range map[string]struct {
		host          func() host
		files         map[string]string
		state, notice string
	}{
		"no ruby":              {func() host { _, env := rubyHost(t); return host{[]string{t.TempDir()}, env} }, nil, "missingTool", "Ruby, required by RuboCop, is not installed"},
		"only shims":           {scriptOnly, nil, "unsupported", "version-manager shims"},
		"requested missing":    {withRubocop, map[string]string{".ruby-version": "3.3.5\n"}, "missingTool", "Ruby 3.3.5, requested by .ruby-version, is not installed"},
		"invalid request":      {withRubocop, map[string]string{".ruby-version": "jruby-9.4.0.0\n"}, "unsupported", ".ruby-version does not name a Ruby version"},
		"too old":              {oldRuby(false), nil, "unsupported", "Ruby 2.6.10"},
		"too old behind shim":  {oldRuby(true), nil, "unsupported", "shims are not run"},
		"no rubocop":           {bare, nil, "missingTool", "gem install rubocop"},
		"locked missing":       {withRubocop, bundled(lockWith("1.80.0", "")), "missingTool", "RuboCop 1.80.0 from Gemfile.lock is not installed. Run bundle install"},
		"no bundler":           {noBundler, bundled(lockWith("1.92.2", "")), "missingTool", "Bundler, required by Gemfile.lock"},
		"git source":           {withRubocop, bundled(gitLock), "unsupported", "git or path source"},
		"conflicted lock":      {withRubocop, bundled("<<<<<<< ours\n" + lockWith("1.92.2", "")), "unsupported", "Gemfile.lock could not be read"},
		"yaml bundle config":   {withRubocop, map[string]string{"Gemfile": "", "Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "BUNDLE_PATH: [x]\n"}, "unsupported", ".bundle/config could not be read"},
		"bundle path outside":  {withRubocop, map[string]string{"Gemfile": "", "Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "BUNDLE_PATH: ../gems\n"}, "unsupported", "points outside the project"},
		"lock without Gemfile": {withRubocop, map[string]string{"Gemfile.lock": lockWith("1.92.2", "")}, "unsupported", "has no Gemfile"},
		"colon in path":        {colonHome, nil, "unsupported", "fixed search path"},
		"colon in ruby path": {func() host {
			_, env := rubyHost(t)
			base, _ := filepath.EvalSymlinks(t.TempDir())
			prefix := filepath.Join(base, "r:b")
			writeRuby(t, prefix, "4.0.9")
			writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
			writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "bundler", "4.0.20")
			// Bundler mode puts no gem directory on a search path.
			return host{[]string{filepath.Join(prefix, "bin")}, env}
		}, bundled(lockWith("1.92.2", "")), "unsupported", "fixed search path"},
	} {
		h := tc.host()
		p, err := DiscoverRubocop(rubocopRoot(t, tc.files), h.path, h.env)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		st := p.Provider()
		if p.Availability() != tc.state || !strings.Contains(st.Notice, tc.notice) || st.MissingTool != (tc.state == "missingTool") {
			t.Errorf("%s: %s %q", name, p.Availability(), st.Notice)
			continue
		}
		if _, ok := p.Server(); ok || !st.Coverage.Partial || !strings.Contains(st.Notice, "opened documents only") {
			t.Errorf("%s: unavailable plan offers a server or full coverage", name)
		}
	}

	// Files that cannot be read for approval make the plan unsupported.
	if os.Geteuid() != 0 {
		h := withRubocop()
		spec := filepath.Join(h.env.Home, ".gem/ruby/4.0.0/specifications/rubocop-1.92.2.gemspec")
		if err := os.Chmod(spec, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(spec, 0o600) })
		p, err := DiscoverRubocop(rubocopRoot(t, nil), h.path, h.env)
		if cfg, ok := p.Server(); err != nil || p.Availability() != "unsupported" || ok || cfg.Executable != "" || !strings.Contains(p.Provider().Notice, "could not be identified") {
			t.Fatalf("unreadable gemspec: %v %s %q", err, p.Availability(), p.Provider().Notice)
		}
	}

	// Without a Ruby file RuboCop is a muted unsupported, never a request to
	// install, whatever the host has: a Gemfile alone is iOS or Jekyll tooling.
	for _, h := range []host{withRubocop(), {[]string{t.TempDir()}, RubyEnvironment{Home: t.TempDir()}}} {
		p, err := DiscoverRubocop(goplsRoot(t, map[string]string{"Gemfile": "gem \"fastlane\"\n", "Podfile": ""}), h.path, h.env)
		if err != nil || p.Availability() != "unsupported" || p.Provider().MissingTool || !strings.Contains(p.Provider().Notice, "No Ruby file") {
			t.Fatal("no Ruby", err, p.Availability())
		}
	}
	for _, bad := range []RubyEnvironment{{}, {Home: "relative"}, {Home: "/x/../y"}} {
		if _, err := DiscoverRubocop(rubocopRoot(t, nil), nil, bad); errCode(err) != "invalidArgument" {
			t.Fatal(err)
		}
	}
}

// rubocopLiveEnv discovers Ruby alone, every other language off, on the
// rubocopHost interpreter with a user-installed RuboCop.
func rubocopLiveEnv(t *testing.T) (LocalLiveEnvironment, rubyRuntime) {
	t.Helper()
	rt, env := rubocopHost(t)
	writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
	return LocalLiveEnvironment{Ruby: env, Disabled: NewDisabledLanguages([]string{"go", "typescript", "svelte", "python", "rust"})}, rt
}

func TestRubocopLocalLiveSingleRoot(t *testing.T) {
	env, rt := rubocopLiveEnv(t)
	root := goplsRoot(t, map[string]string{"Gemfile": "", ".ruby-version": "4.0.9\n", "lib/a.rb": "", "tmp/cache/b.rb": "", ".bundle/config": "---\n"})
	regs, err := DiscoverLocalLive(root, env)
	if err != nil || len(regs) != 2 || regs[0].ProviderID != rubocopProviderID || regs[0].Availability != "available" || regs[1].ProviderID != rubyLspProviderID {
		t.Fatal(err, regs)
	}
	live := regs[0].Live
	if live.Server.Executable != rt.Interpreter || live.Server.Directory != "" || live.Workspaces != nil || live.State.Coverage.Partial ||
		!slices.Equal(live.State.DocumentExtensions, rubyDocumentExtensions) {
		t.Fatalf("%+v", live)
	}
	for file, want := range map[string]bool{"lib/a.rb": true, "Rakefile.rake": true, "Gemfile": false, "a.erb": false, "a.py": false} {
		if lang, ok := live.Resolve(file); ok != want || ok && lang != "ruby" {
			t.Errorf("%s: %q %v", file, lang, ok)
		}
	}
	docs, err := DiscoveryDocuments(root, regs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := docs.Resolve(root, "lib/a.rb"); !ok || s.Config.ProviderID != rubocopProviderID || s.LanguageID != "ruby" {
		t.Fatalf("%+v %v", s, ok)
	}
	// Ruby alone, with no tool lookup when it is off: an invalid Ruby
	// environment is never read.
	off := LocalLiveEnvironment{Ruby: RubyEnvironment{Home: "relative"}, Disabled: NewDisabledLanguages([]string{"go", "typescript", "svelte", "python", "rust", "ruby"})}
	if regs, err := DiscoverLocalLive(root, off); err != nil || len(regs) != 0 {
		t.Fatal(err, regs)
	}
	disabled := DisabledRegistrations(root, off.Disabled)
	// Turning Ruby off stops RuboCop and ruby-lsp alike.
	if len(disabled) != 2 || disabled[0].ProviderID != rubocopProviderID || disabled[1].ProviderID != rubyLspProviderID {
		t.Fatalf("%+v", disabled)
	}
	for _, r := range disabled {
		if r.Availability != "disabled" || r.Live.Server.Executable != "" {
			t.Fatalf("%+v", r)
		}
	}
	// A Gemfile without Ruby sources (fastlane) reports nothing as disabled.
	if regs := DisabledRegistrations(goplsRoot(t, map[string]string{"Gemfile": "", "Podfile": ""}), off.Disabled); len(regs) != 0 {
		t.Fatal(regs)
	}
}

// Each Gemfile with Ruby sources is its own Bundler context; a Gemfile
// without sources (a Jekyll docs site) is no instance; markerless sources keep
// a root instance; installed gems and Rails' tmp are not scanned.
func TestRubocopLocalLiveNested(t *testing.T) {
	env, rt := rubocopLiveEnv(t)
	root := goplsRoot(t, map[string]string{
		".ruby-version": "4.0.9\n",
		"api/Gemfile":   "", "api/.rubocop.yml": "", "api/app/a.rb": "",
		"api/vendor/bundle/ruby/4.0.0/gems/x-1/Gemfile":  "",
		"api/vendor/bundle/ruby/4.0.0/gems/x-1/lib/x.rb": "",
		"api/tmp/Gemfile": "", "api/tmp/c.rb": "",
		"api/.bundle/config": "---\n", "api/.ruby-lsp/Gemfile": "",
		"docs/Gemfile": "gem \"jekyll\"\n", "docs/index.md": "",
		"scripts/x.rb": "",
	})
	regs, err := DiscoverLocalLive(root, env)
	if err != nil || len(regs) != 2 || regs[0].Availability != "available" {
		t.Fatal(err, regs)
	}
	live := regs[0].Live
	if live.State.Coverage.Partial || !slices.Equal(live.State.Coverage.Roots, []string{"", "api"}) || len(live.Workspaces) != 2 {
		t.Fatalf("%+v", live.State)
	}
	top, api := live.Workspaces[0].Server, live.Workspaces[1].Server
	version := ToolInput{ProviderID: "rubocop-toolchain-version-file", Executable: filepath.Join(root.Project().Root, ".ruby-version"), Policy: rubocopToolchainPolicy}
	hasVersion := func(c ServerConfig) bool {
		return slices.ContainsFunc(c.Toolchain, func(ti ToolInput) bool { return reflect.DeepEqual(ti, version) })
	}
	if top.Directory != "" || !slices.Equal(top.ConfigPaths, []string{".ruby-version", "docs/Gemfile"}) || hasVersion(top) || top.Executable != rt.Interpreter {
		t.Fatalf("root %+v", top)
	}
	// The dropped docs boundary's Gemfile is the root's, as for Python.
	// The workspace's own files are bound under its directory; the version file
	// above it, which its scoped root cannot see, as a toolchain file.
	if api.Directory != "api" || !slices.Equal(api.ConfigPaths, []string{"api/.bundle/config", "api/.rubocop.yml", "api/Gemfile"}) || !hasVersion(api) || api.Executable != rt.Interpreter {
		t.Fatalf("api %+v", api)
	}
	docs, err := DiscoveryDocuments(root, regs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, dir := range map[string]string{"api/app/a.rb": "api", "scripts/x.rb": ""} {
		if s, ok := docs.Resolve(root, file); !ok || s.Config.Directory != dir || s.LanguageID != "ruby" {
			t.Errorf("%s: %+v %v", file, s.Config.Directory, ok)
		}
	}
	if _, ok := docs.Resolve(root, "docs/Gemfile"); ok {
		t.Fatal("Gemfile owned")
	}
}

// Synthetic fake server sending what the observed RuboCop sends (RB-00):
// unversioned pushes keep their UTF-16 ranges as uncertain observations bound
// to the mirror at receipt; an empty push clears; unowned and unopened files
// and other providers never create state.
func TestRubocopBridgeUnversionedPushes(t *testing.T) {
	b := &bridgeEnv{notices: make(chan string, 64)}
	var bridge *PushBridge
	e := newEnv(t, SessionOptions{Notifications: func(n SessionNotification) { bridge.Handle(n) }})
	b.sessionEnv = e
	b.root, b.cfg = e.project()
	b.cfg.ProviderID = rubocopProviderID
	e.approve(b.root, b.cfg)
	writeTree(t, b.root.Project().Root, map[string]string{"app.rb": "x", "Gemfile": ""})
	b.h = NewManagerForTest(t, b.root)
	bridge = newPushBridge(rubocopProviderID, b.root, b.h.Manager)
	bridge.Notice = func(s string) { b.notices <- s }
	b.h.SetDocumentProvider(SessionDocuments{Sessions: e.mgr, Resolve: func(r AuthorizedRoot, path string) (DocumentServer, bool) {
		lang, ok := RubyLanguageID(path)
		return DocumentServer{Config: b.cfg, LanguageID: lang}, ok
	}})
	if _, err := e.mgr.Attach(context.Background(), b.root, b.cfg); err != nil {
		t.Fatal(err)
	}
	b.srv = e.next()

	ctx, c := context.Background(), b.h.snap.Context
	if _, err := b.h.OpenDocument(ctx, c, "app.rb", "d1", 1, "def x\n  1 2\nend\n", ""); err != nil {
		t.Fatal(err)
	}
	b.push("app.rb", "", `{"range":{"start":{"line":1,"character":2},"end":{"line":1,"character":3}},"severity":1,"code":"Lint/Syntax","codeDescription":{"href":"https://docs.rubocop.org/rubocop/cops_lint.html#lintsyntax"},"source":"RuboCop","message":"Lint/Syntax: unexpected token tINTEGER\n\nThis offense is not autocorrectable.\n"}`)
	b.await(t, "syntax", func(r []ResultSet) bool { return len(r) == 1 && len(r[0].Diagnostics) == 1 })
	d := b.sets(t)[0].Diagnostics[0]
	if d.Freshness != "uncertain" || d.Range == nil || d.Basis.Version != nil || d.Basis.ObservedVersion == nil || *d.Basis.ObservedVersion != 1 ||
		d.Code != "Lint/Syntax" || d.Severity != "error" || !strings.HasPrefix(d.Source, "rubocop") {
		t.Fatalf("%+v", d)
	}
	if _, err := b.h.ChangeDocument(ctx, c, "d1", 2, "x = \"😀\"; y  = 1\n"); err != nil {
		t.Fatal(err)
	}
	b.push("app.rb", "", `{"range":{"start":{"line":0,"character":9},"end":{"line":0,"character":10}},"severity":3,"code":"Style/Semicolon","source":"RuboCop","message":"Style/Semicolon: Do not use semicolons to terminate expressions."}`)
	b.await(t, "offense", func(r []ResultSet) bool {
		return len(r[0].Diagnostics) == 1 && r[0].Diagnostics[0].Code == "Style/Semicolon"
	})
	if d := b.sets(t)[0].Diagnostics[0]; d.Freshness != "uncertain" || d.Range == nil || d.Basis.ObservedVersion == nil || *d.Basis.ObservedVersion != 2 {
		t.Fatalf("%+v", d)
	}
	b.push("Gemfile", "", errAt) // not owned
	b.push("b.rb", "", errAt)    // not opened
	b.push("app.rb", "", ``)
	b.await(t, "cleared", func(r []ResultSet) bool { return len(r) == 1 && len(r[0].Diagnostics) == 0 })
	if got := b.sets(t); len(got) != 1 || got[0].Scope.Path != "app.rb" {
		t.Fatalf("%+v", got)
	}
	b.srv.send(`{"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":"http://x/a.rb","diagnostics":[]}}`)
	if n := <-b.notices; n != partialRubocopNotice {
		t.Fatal(n)
	}
	// Another provider's notification is not RuboCop's.
	newPushBridge(rubocopProviderID, b.root, b.h.Manager).Handle(SessionNotification{
		Project: b.root.Project(), ProviderID: ruffProviderID, Method: "textDocument/publishDiagnostics",
		Params: json.RawMessage(`{"uri":"` + uriOf(b.root, "app.rb") + `","diagnostics":[` + errAt + `]}`),
	})
	if got := b.sets(t); len(got) != 1 || len(got[0].Diagnostics) != 0 {
		t.Fatalf("%+v", got)
	}
}

// TestRubocopEvidenceReplay replays recorded RuboCop publications through the
// shared normalizer. It proves normalization of observed payloads only.
func TestRubocopEvidenceReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/rubocop/evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	type summary struct {
		URI         string
		HasVersion  bool
		Diagnostics []struct {
			Code     string
			Severity int
			Range    json.RawMessage
		}
	}
	var ev struct {
		Versions  map[string]string
		Scenarios []struct {
			ID           string
			Publications []struct{ Params json.RawMessage }
		}
		Bundler struct{ Publications []summary }
		UTF16   struct{ Publications []summary }
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"rubocop", "ruby", "bundler"} {
		if ev.Versions[tool] == "" {
			t.Fatalf("evidence names no %s version: %v", tool, ev.Versions)
		}
	}
	root := goplsRoot(t, map[string]string{"app.rb": "x", "Gemfile": ""})
	replay := func(id string, params json.RawMessage) goplsPublication {
		t.Helper()
		fixed := strings.ReplaceAll(string(params), "file:///fixture/", uriOf(root, "")+"/")
		pub, err := normalizeLSPPublication(root, json.RawMessage(fixed), rubocopProviderID)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if pub.Version != nil {
			t.Fatalf("%s: observed RuboCop pushes are unversioned", id)
		}
		return pub
	}
	codes := []string{}
	for _, s := range ev.Scenarios {
		for _, p := range s.Publications {
			for _, d := range replay(s.ID, p.Params).Diagnostics {
				if d.Source != "rubocop/RuboCop" || d.Range == nil || !strings.HasPrefix(d.Message, d.Code+": ") {
					t.Fatalf("%s: %+v", s.ID, d)
				}
				codes = append(codes, d.Code+"/"+d.Severity)
			}
		}
	}
	want := []string{"Lint/Syntax/error", "Style/FrozenStringLiteralComment/information", "Style/StringLiterals/information", "Layout/TrailingWhitespace/information"}
	if !slices.Equal(codes, want) {
		t.Fatal(codes)
	}
	// The Bundler and UTF-16 runs were recorded as summaries: rebuilt into
	// pushes, they normalize alike, with columns kept as sent. In Bundler mode
	// the project's .rubocop.yml asks for double quotes, so v3's 'ok' is the
	// offense.
	summaries := func(id string, pubs []summary) [][]string {
		var got [][]string
		for i, s := range pubs {
			if s.HasVersion {
				t.Fatalf("%s %d: recorded as versioned", id, i)
			}
			ds := []map[string]any{}
			for _, d := range s.Diagnostics {
				ds = append(ds, map[string]any{"range": d.Range, "severity": d.Severity, "code": d.Code, "source": "RuboCop", "message": d.Code + ": recorded"})
			}
			params, _ := json.Marshal(map[string]any{"uri": s.URI, "diagnostics": ds})
			row := []string{}
			for _, d := range replay(id, params).Diagnostics {
				row = append(row, d.Code+"@"+strconv.Itoa(d.Range.Start.Line)+":"+strconv.Itoa(d.Range.Start.Character))
			}
			got = append(got, row)
		}
		return got
	}
	if got, want := summaries("bundler", ev.Bundler.Publications), [][]string{{"Lint/Syntax@1:2"}, {"Style/FrozenStringLiteralComment@0:0", "Layout/TrailingWhitespace@1:7"}, {"Style/StringLiterals@2:5"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bundler %v", got)
	}
	// After two emoji the semicolon is at UTF-16 column 10, after two accented letters at 8.
	if got, want := summaries("utf16", ev.UTF16.Publications), [][]string{{"Style/Semicolon@2:10", "Layout/ExtraSpacing@2:13"}, {"Style/Semicolon@2:8", "Layout/ExtraSpacing@2:11"}, {}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("utf16 %v", got)
	}
}

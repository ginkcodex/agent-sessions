package codediagnostics

// SYNTHETIC fixtures: lockfiles, Bundler configs and gem directories below
// prove RuboCop and Bundler lookup only; gem files are never loaded. The
// Bundler-mode layout follows the RB-00 run recorded in
// testdata/rubocop/evidence.json.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleGemfileLock = `GIT
  remote: https://github.com/example/tool.git
  revision: 0123456789abcdef
  specs:
    tool (0.3.0)
      json

PATH
  remote: .
  specs:
    app (1.0.0)

GEM
  remote: https://rubygems.org/
  specs:
    ast (2.4.3)
    json (2.15.2)
    nokogiri (1.18.10-arm64-darwin)
      racc (~> 1.4)
    nokogiri (1.18.10-x86_64-linux-gnu)
      racc (~> 1.4)
    rubocop (1.92.2)
      json (~> 2.3)
      rubocop-ast (>= 1.47.1, < 2.0)
    rubocop-ast (1.48.0)
      parser (>= 3.3.7.2)
    rubocop-rails (2.0.0.pre1)

PLATFORMS
  arm64-darwin-25
  ruby

DEPENDENCIES
  rubocop (= 1.92.2)

CHECKSUMS
  rubocop (1.92.2) sha256=00

RUBY VERSION
   ruby 4.0.9p0

BUNDLED WITH
   4.0.20
`

func TestParseGemfileLock(t *testing.T) {
	lock, ok := parseGemfileLock(strings.ReplaceAll(sampleGemfileLock, "\n", "\r\n"))
	if !ok || lock.rubyVersion != "ruby 4.0.9p0" || lock.bundledWith != "4.0.20" {
		t.Fatalf("%+v %v", lock, ok)
	}
	want := map[string]lockedGem{
		"tool": {"0.3.0", "", "GIT"}, "app": {"1.0.0", "", "PATH"}, "ast": {"2.4.3", "", "GEM"}, "json": {"2.15.2", "", "GEM"},
		"nokogiri": {"1.18.10", "arm64-darwin", "GEM"}, "rubocop": {"1.92.2", "", "GEM"}, "rubocop-ast": {"1.48.0", "", "GEM"},
		"rubocop-rails": {"2.0.0.pre1", "", "GEM"},
	}
	if len(lock.gems) != len(want) {
		t.Fatal(lock.gems)
	}
	for name, g := range want {
		if lock.gems[name] != g {
			t.Errorf("%s: %+v", name, lock.gems[name])
		}
	}
	if _, ok := lock.gems["racc"]; ok {
		t.Fatal("dependency lines are not specs")
	}
	noBundled, ok := parseGemfileLock(strings.Split(sampleGemfileLock, "BUNDLED WITH")[0])
	if !ok || noBundled.bundledWith != "" || noBundled.gems["rubocop"].version != "1.92.2" {
		t.Fatal("BUNDLED WITH missing", noBundled)
	}
	for name, body := range map[string]string{
		"empty":          "",
		"conflict":       "GEM\n  specs:\n<<<<<<< HEAD\n    rubocop (1.92.2)\n=======\n    rubocop (1.91.0)\n>>>>>>> branch\n",
		"spec shape":     "GEM\n  specs:\n    rubocop 1.92.2\n",
		"spec version":   "GEM\n  specs:\n    rubocop (1.92.2; rm)\n",
		"version shape":  "GEM\n  specs:\n    rubocop (../1)\n",
		"two bundlers":   "BUNDLED WITH\n   4.0.20\n   2.5.0\n",
		"bundler shape":  "BUNDLED WITH\n   four\n",
		"two rubies":     "RUBY VERSION\n   ruby 3.3.5\n   ruby 3.4.1\n",
		"indented start": "  specs:\n    rubocop (1.92.2)\n",
	} {
		if lock, ok := parseGemfileLock(body); ok {
			t.Errorf("%s accepted: %+v", name, lock)
		}
	}
}

func TestReadGemfileLockBounds(t *testing.T) {
	dir := rubyProject(t, map[string]string{"Gemfile.lock": sampleGemfileLock, "big/Gemfile.lock": "GEM\n" + strings.Repeat(" ", maxGemfileLock)})
	if lock, present, ok := readGemfileLock(filepath.Join(dir, "Gemfile.lock"), dir); !present || !ok || lock.gems["rubocop"].version != "1.92.2" {
		t.Fatal(lock, present, ok)
	}
	if _, present, ok := readGemfileLock(filepath.Join(dir, "big/Gemfile.lock"), dir); !present || ok {
		t.Fatal("oversized")
	}
	if _, present, _ := readGemfileLock(filepath.Join(dir, "none/Gemfile.lock"), dir); present {
		t.Fatal("absent")
	}
	outside := rubyProject(t, map[string]string{"Gemfile.lock": sampleGemfileLock})
	symlink(t, filepath.Join(outside, "Gemfile.lock"), filepath.Join(dir, "linked/Gemfile.lock"))
	if _, present, ok := readGemfileLock(filepath.Join(dir, "linked/Gemfile.lock"), dir); !present || ok {
		t.Fatal("link leaving the boundary")
	}
}

func TestBundleSetting(t *testing.T) {
	for body, want := range map[string]string{
		"---\nBUNDLE_PATH: \"vendor/bundle\"\n":                "vendor/bundle",
		"---\nBUNDLE_WITHOUT: \"test\"\nBUNDLE_PATH: vendor\n": "vendor",
		"BUNDLE_PATH: '/abs/gems'\r\n":                         "/abs/gems",
	} {
		if v, found, ok := bundleSetting([]byte(body), "BUNDLE_PATH"); !found || !ok || v != want {
			t.Errorf("%q: %q %v %v", body, v, found, ok)
		}
	}
	for _, body := range []string{"---\nBUNDLE_WITHOUT: \"test\"\n", "---\nnested:\n  BUNDLE_PATH: vendor\n"} {
		if _, found, ok := bundleSetting([]byte(body), "BUNDLE_PATH"); found || !ok {
			t.Errorf("%q found", body)
		}
	}
	for _, body := range []string{"BUNDLE_PATH: &anchor vendor\n", "BUNDLE_PATH: [a, b]\n", "BUNDLE_PATH:\n", "BUNDLE_PATH: \"a\" # c\n"} {
		if _, found, ok := bundleSetting([]byte(body), "BUNDLE_PATH"); !found || ok {
			t.Errorf("%q accepted", body)
		}
	}
}

// writeGem installs a SYNTHETIC gem into the gem directory home: its entry
// file and gemspec, nothing else.
func writeGem(t *testing.T, home, name, version string) string {
	t.Helper()
	dir := name + "-" + version
	entry := filepath.ToSlash(gemEntry(name))
	writeTree(t, home, map[string]string{"gems/" + dir + "/" + entry: "# synthetic\n", "specifications/" + dir + ".gemspec": "# synthetic\n"})
	return filepath.Join(home, "gems", dir, filepath.FromSlash(entry))
}

func TestFindGem(t *testing.T) {
	a, _ := filepath.EvalSymlinks(t.TempDir())
	b, _ := filepath.EvalSymlinks(t.TempDir())
	writeGem(t, a, "rubocop", "1.80.0")
	newest := writeGem(t, b, "rubocop", "1.92.2")
	writeGem(t, b, "rubocop", "1.100.0.pre1")
	writeGem(t, a, "rubocop-ast", "9.0.0")
	native := writeGem(t, a, "nokogiri", "1.18.10-arm64-darwin")
	if g, ok := newestGem([]string{a, b}, "rubocop"); !ok || g.Entry != newest || g.Version != "1.92.2" || g.Home != b || g.Spec != filepath.Join(b, "specifications/rubocop-1.92.2.gemspec") {
		t.Fatal("newest", g)
	}
	if g, ok := findGem([]string{a, b}, "rubocop", "1.80.0"); !ok || g.Home != a {
		t.Fatal("exact", g)
	}
	if _, ok := findGem([]string{a, b}, "rubocop", "1.81.0"); ok {
		t.Fatal("absent version")
	}
	if g, ok := findGem([]string{a}, "nokogiri", "1.18.10"); !ok || g.Entry != native {
		t.Fatal("platform suffix", g)
	}
	// A gem directory without its gemspec, or with an entry linking out of the
	// gem directory, is not installed.
	writeTree(t, a, map[string]string{"gems/rubocop-1.95.0/exe/rubocop": ""})
	outside := rubyProject(t, map[string]string{"rubocop": ""})
	writeTree(t, a, map[string]string{"specifications/rubocop-1.96.0.gemspec": ""})
	symlink(t, filepath.Join(outside, "rubocop"), filepath.Join(a, "gems/rubocop-1.96.0/exe/rubocop"))
	writeGem(t, a, "rubocop", "1.97.0")
	if err := os.Remove(filepath.Join(a, "specifications/rubocop-1.97.0.gemspec")); err != nil {
		t.Fatal(err)
	}
	symlink(t, filepath.Join(outside, "rubocop"), filepath.Join(a, "specifications/rubocop-1.97.0.gemspec"))
	for _, v := range []string{"1.95.0", "1.96.0", "1.97.0"} {
		if g, ok := findGem([]string{a}, "rubocop", v); ok {
			t.Errorf("%s accepted: %+v", v, g)
		}
	}
	if compareGemVersions("2.0.0.pre1", "2.0.0") >= 0 || compareGemVersions("1.10", "1.9") <= 0 || compareGemVersions("1.0", "1.0.0") != 0 {
		t.Fatal("version order")
	}
}

// rubocopHost builds a SYNTHETIC mise Ruby 4.0.9 with the default Bundler
// 4.0.20 as observed on the development host.
func rubocopHost(t *testing.T) (rubyRuntime, RubyEnvironment) {
	t.Helper()
	home, env := rubyHost(t)
	prefix := filepath.Join(home, ".local/share/mise/installs/ruby/4.0.9")
	writeRuby(t, prefix, "4.0.9")
	writeTree(t, prefix, map[string]string{
		"lib/ruby/4.0.0/bundler/setup.rb":                                   "# synthetic\n",
		"lib/ruby/gems/4.0.0/specifications/default/bundler-4.0.20.gemspec": "# synthetic\n",
		"lib/ruby/gems/4.0.0/gems/bundler-4.0.20/exe/bundle":                "# synthetic\n",
	})
	rt, state := inspectRubyPrefix(prefix)
	if state != "" {
		t.Fatal(state)
	}
	return rt, env
}

func lockWith(rubocop, bundled string) string {
	s := "GEM\n  remote: https://rubygems.org/\n  specs:\n    rubocop (" + rubocop + ")\n\nPLATFORMS\n  ruby\n"
	if bundled != "" {
		s += "\nBUNDLED WITH\n   " + bundled + "\n"
	}
	return s
}

func discoverRubocopFiles(t *testing.T, ws string, rt rubyRuntime, env RubyEnvironment) rubocopInstall {
	t.Helper()
	lock, present, ok := readGemfileLock(filepath.Join(ws, "Gemfile.lock"), ws)
	return findRubocop(ws, lock, present, ok, rt, env)
}

func TestFindRubocopStandalone(t *testing.T) {
	rt, env := rubocopHost(t)
	interpreter := filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0")
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	ws := rubyProject(t, map[string]string{"Gemfile": "", "Gemfile.lock": "GEM\n  specs:\n    rake (13.0.0)\n"})
	if inst := discoverRubocopFiles(t, ws, rt, env); inst.Mode != "standalone" || inst.State != "missing" {
		t.Fatal("none installed", inst)
	}
	writeGem(t, interpreter, "rubocop", "1.90.0")
	newest := writeGem(t, user, "rubocop", "1.92.2")
	inst := discoverRubocopFiles(t, ws, rt, env)
	if inst.State != "" || inst.Mode != "standalone" || inst.Rubocop.Entry != newest || len(inst.GemDirs) != 2 || inst.GemDirs[0] != user || inst.GemDirs[1] != interpreter {
		t.Fatal("standalone", inst)
	}
	// No lockfile at all is standalone too.
	if inst := discoverRubocopFiles(t, rubyProject(t, map[string]string{"a.rb": ""}), rt, env); inst.Mode != "standalone" || inst.Rubocop.Version != "1.92.2" {
		t.Fatal("no lockfile", inst)
	}
	// chruby's per-version GEM_HOME.
	rt2, env2 := rubocopHost(t)
	chruby := writeGem(t, filepath.Join(env2.Home, ".gem/ruby/4.0.9"), "rubocop", "1.91.0")
	if inst := discoverRubocopFiles(t, ws, rt2, env2); inst.Rubocop.Entry != chruby {
		t.Fatal("chruby GEM_HOME", inst)
	}
}

func TestFindRubocopBundler(t *testing.T) {
	rt, env := rubocopHost(t)
	ws := rubyProject(t, map[string]string{
		"Gemfile":        "gem \"rubocop\"\n",
		"Gemfile.lock":   lockWith("1.92.2", "4.0.20"),
		".bundle/config": "---\nBUNDLE_PATH: \"vendor/bundle\"\n",
	})
	vendor := filepath.Join(ws, "vendor/bundle/ruby/4.0.0")
	// A RuboCop outside BUNDLE_PATH is not what Bundler would load.
	writeGem(t, filepath.Join(env.Home, ".gem/ruby/4.0.0"), "rubocop", "1.92.2")
	inst := discoverRubocopFiles(t, ws, rt, env)
	if inst.Mode != "bundler" || inst.State != "notInstalled" || inst.LockedVersion != "1.92.2" || inst.BundlePath != vendor {
		t.Fatal("locked not installed", inst)
	}
	writeGem(t, vendor, "rubocop", "1.91.0")
	entry := writeGem(t, vendor, "rubocop", "1.92.2")
	inst = discoverRubocopFiles(t, ws, rt, env)
	if inst.State != "" || inst.Rubocop.Entry != entry || inst.Bundler.Version != "4.0.20" || inst.BundlerEnv != "" ||
		inst.Bundler.Entry != filepath.Join(rt.Prefix, "lib/ruby/4.0.0/bundler/setup.rb") ||
		inst.Bundler.Spec != filepath.Join(rt.Prefix, "lib/ruby/gems/4.0.0/specifications/default/bundler-4.0.20.gemspec") {
		t.Fatal("vendor/bundle", inst)
	}
	// Without BUNDLE_PATH the user and interpreter gem directories are searched.
	plain := rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", "")})
	if inst := discoverRubocopFiles(t, plain, rt, env); inst.State != "" || inst.Mode != "bundler" || inst.Rubocop.Home != filepath.Join(env.Home, ".gem/ruby/4.0.0") || inst.BundlerEnv != "" {
		t.Fatal("no BUNDLE_PATH, no BUNDLED WITH", inst)
	}
	// BUNDLE_DEPLOYMENT implies vendor/bundle.
	deploy := rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "---\nBUNDLE_DEPLOYMENT: \"true\"\n"})
	writeGem(t, filepath.Join(deploy, "vendor/bundle/ruby/4.0.0"), "rubocop", "1.92.2")
	if inst := discoverRubocopFiles(t, deploy, rt, env); inst.State != "" || inst.Rubocop.Home != filepath.Join(deploy, "vendor/bundle/ruby/4.0.0") {
		t.Fatal("deployment", inst)
	}
	// An absolute BUNDLE_PATH, here from the user's ~/.bundle/config.
	shared, _ := filepath.EvalSymlinks(t.TempDir())
	writeGem(t, filepath.Join(shared, "ruby/4.0.0"), "rubocop", "1.92.2")
	writeTree(t, env.Home, map[string]string{".bundle/config": "---\nBUNDLE_PATH: \"" + shared + "\"\n"})
	if inst := discoverRubocopFiles(t, plain, rt, env); inst.State != "" || inst.Rubocop.Home != filepath.Join(shared, "ruby/4.0.0") {
		t.Fatal("user BUNDLE_PATH", inst)
	}
}

func TestFindRubocopBundlerVersions(t *testing.T) {
	rt, env := rubocopHost(t)
	user := filepath.Join(env.Home, ".gem/ruby/4.0.0")
	writeGem(t, user, "rubocop", "1.92.2")
	older := writeGem(t, user, "bundler", "2.6.9")
	for bundled, want := range map[string]struct{ version, env string }{
		"4.0.20": {"4.0.20", ""},
		"2.6.9":  {"2.6.9", "2.6.9"},
		"2.5.0":  {"4.0.20", ""}, // not installed: the newest runs, Bundler warns itself
		"":       {"4.0.20", ""},
	} {
		inst := discoverRubocopFiles(t, rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", bundled)}), rt, env)
		if inst.State != "" || inst.Bundler.Version != want.version || inst.BundlerEnv != want.env {
			t.Errorf("BUNDLED WITH %q: %+v", bundled, inst)
		}
		if want.version == "2.6.9" && inst.Bundler.Entry != older {
			t.Errorf("regular bundler gem entry: %s", inst.Bundler.Entry)
		}
	}
	// No Bundler at all.
	home, env2 := rubyHost(t)
	prefix := filepath.Join(home, ".rbenv/versions/3.3.5")
	writeRuby(t, prefix, "3.3.5")
	rt2, _ := inspectRubyPrefix(prefix)
	writeGem(t, filepath.Join(prefix, "lib/ruby/gems/3.3.0"), "rubocop", "1.92.2")
	if inst := discoverRubocopFiles(t, rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", "")}), rt2, env2); inst.State != "missingBundler" {
		t.Fatal("missing bundler", inst)
	}
}

func TestFindRubocopRejected(t *testing.T) {
	rt, env := rubocopHost(t)
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	writeGem(t, filepath.Join(outside, "ruby/4.0.0"), "rubocop", "1.92.2")
	linked := rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "BUNDLE_PATH: vendor/bundle\n"})
	symlink(t, outside, filepath.Join(linked, "vendor/bundle"))
	for name, c := range map[string]struct {
		ws    string
		state string
	}{
		"relative path leaving": {rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "BUNDLE_PATH: \"../shared\"\n"}), "bundlePathOutside"},
		"vendor link leaving":   {linked, "bundlePathOutside"},
		"bad bundle config":     {rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": "BUNDLE_PATH: *ref\n"}), "invalidBundleConfig"},
		"oversized config":      {rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", ""), ".bundle/config": strings.Repeat("#", maxBundleConfig+1)}), "invalidBundleConfig"},
		"git source":            {rubyProject(t, map[string]string{"Gemfile.lock": "GIT\n  remote: x\n  specs:\n    rubocop (1.92.2)\n"}), "unsupportedSource"},
		"path source":           {rubyProject(t, map[string]string{"Gemfile.lock": "PATH\n  remote: .\n  specs:\n    rubocop (1.92.2)\n"}), "unsupportedSource"},
		"malformed lockfile":    {rubyProject(t, map[string]string{"Gemfile.lock": "GEM\n  specs:\n    rubocop\n"}), "invalidLockfile"},
		"oversized lockfile":    {rubyProject(t, map[string]string{"Gemfile.lock": lockWith("1.92.2", "") + strings.Repeat("#", maxGemfileLock)}), "invalidLockfile"},
	} {
		if inst := discoverRubocopFiles(t, c.ws, rt, env); inst.State != c.state {
			t.Errorf("%s: %+v", name, inst)
		}
	}
}

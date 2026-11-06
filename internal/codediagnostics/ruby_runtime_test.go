package codediagnostics

// SYNTHETIC fixtures: the fake interpreters, rbconfig.rb files and version
// manager trees below prove Ruby discovery only. Nothing is executed. The
// layouts follow the mise install observed on the development host
// (docs/plan/code-diagnostics-ruby.md, "Finding Ruby").

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fakeRubyBinary = "\xcf\xfa\xed\xfe synthetic ruby"

func rbConfig(version string) string {
	v := strings.Split(version, ".")
	return "# encoding: ascii-8bit\nmodule RbConfig\n  CONFIG = {}\n" +
		"  CONFIG[\"MAJOR\"] = \"" + v[0] + "\"\n  CONFIG[\"MINOR\"] = \"" + v[1] + "\"\n  CONFIG[\"TEENY\"] = \"" + v[2] + "\"\n" +
		"  CONFIG[\"ruby_version\"] = \"" + v[0] + "." + v[1] + ".0\"\n" +
		"  CONFIG[\"RUBY_PROGRAM_VERSION\"] = \"$(MAJOR).$(MINOR).$(TEENY)\"\nend\n"
}

// writeRuby lays out a fake (never executed) interpreter at prefix/bin/ruby
// with the rbconfig.rb a source build installs.
func writeRuby(t *testing.T, prefix, version string) string {
	t.Helper()
	abi := strings.Join(strings.Split(version, ".")[:2], ".") + ".0"
	writeTree(t, prefix, map[string]string{
		"bin/ruby": fakeRubyBinary,
		"lib/ruby/" + abi + "/arm64-darwin25/rbconfig.rb": rbConfig(version),
		"lib/ruby/gems/" + abi + "/specifications/.keep":  "",
	})
	bin := filepath.Join(prefix, "bin", "ruby")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func rubyHost(t *testing.T) (string, RubyEnvironment) {
	t.Helper()
	home, _ := filepath.EvalSymlinks(t.TempDir())
	return home, RubyEnvironment{Home: home}
}

func rubyProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	writeTree(t, dir, files)
	return dir
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestRubyVersionManagers(t *testing.T) {
	cases := map[string]struct {
		install, request string // install is relative to HOME
		files            map[string]string
		version          string
	}{
		"rbenv .ruby-version":     {".rbenv/versions/3.3.5", "", map[string]string{".ruby-version": "3.3.5\n"}, "3.3.5"},
		"mise mise.toml":          {".local/share/mise/installs/ruby/3.4.1", "", map[string]string{"mise.toml": "[env]\nruby = \"9.9\"\n[tools] # pinned\nnode = \"22\"\nruby = \"3.4.1\" # comment\n"}, "3.4.1"},
		"mise .mise.toml":         {".local/share/mise/installs/ruby/3.4.1", "", map[string]string{".mise.toml": "[tools]\n'ruby' = '3.4.1'\n"}, "3.4.1"},
		"asdf .tool-versions":     {".asdf/installs/ruby/3.2.6", "", map[string]string{".tool-versions": "# tools\nnodejs 22.1.0\nruby 3.2.6 3.2.5\n"}, "3.2.6"},
		"chruby ruby- prefix":     {".rubies/ruby-3.3.5", "", map[string]string{".ruby-version": "ruby-3.3.5"}, "3.3.5"},
		"rvm patch level":         {".rvm/rubies/ruby-2.7.8", "", map[string]string{".ruby-version": "ruby-2.7.8-p225\n"}, "2.7.8"},
		"lockfile exact":          {".rbenv/versions/3.3.5", "ruby 3.3.5p100", nil, "3.3.5"},
		"lockfile same minor":     {".rbenv/versions/3.3.7", "ruby 3.3.5p100", nil, "3.3.7"},
		"version file over lock":  {".rbenv/versions/3.4.1", "ruby 3.3.5p100", map[string]string{".ruby-version": "3.4.1"}, "3.4.1"},
		"ruby-version over mise":  {".rbenv/versions/3.4.1", "", map[string]string{".ruby-version": "3.4.1", "mise.toml": "[tools]\nruby = \"3.3\"\n"}, "3.4.1"},
		"mise.toml without ruby":  {".asdf/installs/ruby/3.2.6", "", map[string]string{"mise.toml": "[tools]\nnode = \"22\"\n", ".tool-versions": "ruby 3.2.6\n"}, "3.2.6"},
		"partial newest in root":  {".local/share/mise/installs/ruby/3.3.10", "", map[string]string{".ruby-version": "3.3"}, "3.3.10"},
		"preview exact directory": {".rbenv/versions/3.5.0-preview1", "", map[string]string{".ruby-version": "3.5.0-preview1"}, "3.5.0"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			home, env := rubyHost(t)
			if strings.HasSuffix(c.install, "3.3.10") {
				writeRuby(t, filepath.Join(home, ".local/share/mise/installs/ruby/3.3.9"), "3.3.9")
				writeRuby(t, filepath.Join(home, ".local/share/mise/installs/ruby/3.4.1"), "3.4.1")
			}
			bin := writeRuby(t, filepath.Join(home, c.install), c.version)
			project := rubyProject(t, c.files)
			rt := resolveRuby([]string{project}, c.request, nil, env)
			if rt.State != "" || rt.Interpreter != bin || rt.Version != c.version || rt.Prefix != filepath.Join(home, c.install) || rt.ABI != majorMinor(c.version)+".0" {
				t.Fatalf("%+v", rt)
			}
			if rt.RequestFile == "" || rt.Request == "" {
				t.Fatalf("request not recorded: %+v", rt)
			}
		})
	}
}

func TestRubyInstallOrderAndRoots(t *testing.T) {
	home, env := rubyHost(t)
	project := rubyProject(t, map[string]string{".ruby-version": "3.3.5"})
	rbenv := writeRuby(t, filepath.Join(home, ".rbenv/versions/3.3.5"), "3.3.5")
	writeRuby(t, filepath.Join(home, ".local/share/mise/installs/ruby/3.3.5"), "3.3.5")
	if rt := resolveRuby([]string{project}, "", nil, env); rt.Interpreter != rbenv {
		t.Fatal("rbenv comes first", rt)
	}
	// Manager roots from the reviewed environment replace the HOME defaults.
	custom, _ := filepath.EvalSymlinks(t.TempDir())
	mise := writeRuby(t, filepath.Join(custom, "mise/installs/ruby/3.3.5"), "3.3.5")
	env.RbenvRoot, env.MiseDataDir = filepath.Join(custom, "rbenv"), filepath.Join(custom, "mise")
	if rt := resolveRuby([]string{project}, "", nil, env); rt.Interpreter != mise {
		t.Fatal("MISE_DATA_DIR", rt)
	}
	env.MiseDataDir = "relative/mise"
	if rt := resolveRuby([]string{project}, "", nil, env); rt.State != "" || !strings.HasPrefix(rt.Interpreter, filepath.Join(home, ".local")) {
		t.Fatal("relative manager root ignored", rt)
	}
	// chruby's /opt/rubies.
	old := rubySystemRubies
	t.Cleanup(func() { rubySystemRubies = old })
	rubySystemRubies, _ = filepath.EvalSymlinks(t.TempDir())
	opt := writeRuby(t, filepath.Join(rubySystemRubies, "ruby-3.1.6"), "3.1.6")
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "3.1.6"})}, "", nil, RubyEnvironment{Home: home}); rt.Interpreter != opt {
		t.Fatal("/opt/rubies", rt)
	}
}

// The observed mise layout: 4.0.9 plus links 4, 4.0, latest and global.
func TestRubyMiseAliasLinks(t *testing.T) {
	home, env := rubyHost(t)
	installs := filepath.Join(home, ".local/share/mise/installs/ruby")
	bin := writeRuby(t, filepath.Join(installs, "4.0.9"), "4.0.9")
	writeRuby(t, filepath.Join(installs, "4.0.1"), "4.0.1")
	writeRuby(t, filepath.Join(installs, "3.4.8"), "3.4.8")
	for _, alias := range []string{"4", "4.0", "latest", "global"} {
		symlink(t, "./4.0.9", filepath.Join(installs, alias))
	}
	writeTree(t, installs, map[string]string{".mise.backend.toml": "backend = \"core:ruby\"\n"})
	for request, want := range map[string]string{"4": bin, "4.0": bin, "latest": bin, "3": filepath.Join(installs, "3.4.8/bin/ruby"), "4.0.1": filepath.Join(installs, "4.0.1/bin/ruby")} {
		rt := resolveRuby([]string{rubyProject(t, map[string]string{"mise.toml": "[tools]\nruby = \"" + request + "\"\n"})}, "", nil, env)
		if rt.State != "" || rt.Interpreter != want {
			t.Errorf("%s: %+v", request, rt)
		}
	}
	// Without the links, a partial request takes the newest matching install.
	home2, env2 := rubyHost(t)
	installs2 := filepath.Join(home2, ".local/share/mise/installs/ruby")
	writeRuby(t, filepath.Join(installs2, "4.0.1"), "4.0.1")
	newest := writeRuby(t, filepath.Join(installs2, "4.0.10"), "4.0.10")
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "4"})}, "", nil, env2); rt.Interpreter != newest {
		t.Fatal("numeric ordering", rt)
	}
}

func TestRubyRequestNotReplaced(t *testing.T) {
	home, env := rubyHost(t)
	installs := filepath.Join(home, ".local/share/mise/installs/ruby")
	writeRuby(t, filepath.Join(installs, "3.3.6"), "3.3.6")
	elsewhere, _ := filepath.EvalSymlinks(t.TempDir())
	writeRuby(t, filepath.Join(elsewhere, "3.3.5"), "3.3.5")
	writeRuby(t, filepath.Join(elsewhere, "4.0.9"), "4.0.9")
	// An alias link leaving the installs directory is not an install.
	symlink(t, filepath.Join(elsewhere, "4.0.9"), filepath.Join(installs, "4"))
	symlink(t, filepath.Join(elsewhere, "3.3.5"), filepath.Join(installs, "3.3.5"))
	path := []string{filepath.Join(elsewhere, "4.0.9/bin")}
	for name, c := range map[string]struct {
		files map[string]string
		lock  string
		req   string
		file  string
	}{
		"exact missing":       {map[string]string{".ruby-version": "3.3.5"}, "", "3.3.5", ".ruby-version"},
		"alias leaves root":   {map[string]string{"mise.toml": "[tools]\nruby = \"4\"\n"}, "", "4", "mise.toml"},
		"lock minor missing":  {nil, "ruby 3.2.2p53", "3.2.2", "Gemfile.lock"},
		"partial not present": {map[string]string{".tool-versions": "ruby 3.1\n"}, "", "3.1", ".tool-versions"},
	} {
		project := rubyProject(t, c.files)
		rt := resolveRuby([]string{project}, c.lock, path, env)
		if rt.State != "notInstalled" || rt.Interpreter != "" || rt.Request != c.req || rt.RequestFile != filepath.Join(project, c.file) {
			t.Errorf("%s: %+v", name, rt)
		}
	}
	// A directory whose rbconfig.rb names another version does not satisfy the
	// request.
	writeRuby(t, filepath.Join(installs, "3.2.9"), "3.1.0")
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "3.2.9"})}, "", nil, env); rt.State != "notInstalled" {
		t.Fatal("mismatched contents", rt)
	}
}

func TestRubyAncestorRequests(t *testing.T) {
	home, env := rubyHost(t)
	r33 := writeRuby(t, filepath.Join(home, ".rbenv/versions/3.3.5"), "3.3.5")
	r34 := writeRuby(t, filepath.Join(home, ".rbenv/versions/3.4.1"), "3.4.1")
	root := rubyProject(t, map[string]string{".ruby-version": "3.3.5", "svc/Gemfile": "", "api/.ruby-version": "3.4.1"})
	if rt := resolveRuby([]string{filepath.Join(root, "svc"), root}, "", nil, env); rt.Interpreter != r33 || rt.RequestFile != filepath.Join(root, ".ruby-version") {
		t.Fatal("ancestor", rt)
	}
	if rt := resolveRuby([]string{filepath.Join(root, "api"), root}, "", nil, env); rt.Interpreter != r34 {
		t.Fatal("nearest wins", rt)
	}
	// A workspace lockfile does not outrank an ancestor's version file.
	if rt := resolveRuby([]string{filepath.Join(root, "svc"), root}, "ruby 3.4.1", nil, env); rt.Interpreter != r33 {
		t.Fatal("ancestor file over lock", rt)
	}
}

func TestRubyInvalidRequests(t *testing.T) {
	home, env := rubyHost(t)
	writeRuby(t, filepath.Join(home, ".rbenv/versions/3.3.5"), "3.3.5")
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	writeTree(t, outside, map[string]string{"version": "3.3.5"})
	cases := map[string]map[string]string{
		"oversized":          {".ruby-version": "3.3.5\n" + strings.Repeat("#", maxRubyVersionFile)},
		"oversized toml":     {"mise.toml": "[tools]\nruby = \"3.3.5\"\n" + strings.Repeat("#", maxRubyVersionFile)},
		"jruby":              {".ruby-version": "jruby-9.4.8.0"},
		"truffleruby":        {".tool-versions": "ruby truffleruby-24.1.0\n"},
		"garbage":            {".ruby-version": "$(rm -rf ~)"},
		"empty":              {".ruby-version": "\n"},
		"tool-versions bare": {".tool-versions": "ruby\n"},
		"toml array":         {"mise.toml": "[tools]\nruby = [\"3.3\", \"3.2\"]\n"},
		"toml table":         {"mise.toml": "[tools]\nruby = { version = \"3.3\" }\n"},
		"toml unterminated":  {"mise.toml": "[tools]\nruby = \"3.3\n"},
		"toml trailing":      {"mise.toml": "[tools]\nruby = \"3.3\" extra\n"},
		"lockfile garbage":   {},
	}
	for name, files := range cases {
		project := rubyProject(t, files)
		lock := ""
		if name == "lockfile garbage" {
			lock = "ruby x.y"
		}
		if rt := resolveRuby([]string{project}, lock, nil, env); rt.State != "invalidRequest" || rt.RequestFile == "" {
			t.Errorf("%s: %+v", name, rt)
		}
	}
	project := rubyProject(t, nil)
	symlink(t, filepath.Join(outside, "version"), filepath.Join(project, ".ruby-version"))
	if rt := resolveRuby([]string{project}, "", nil, env); rt.State != "invalidRequest" {
		t.Fatal("link leaving the boundary", rt)
	}
	inside := rubyProject(t, map[string]string{"config/ruby-version": "3.3.5"})
	symlink(t, "config/ruby-version", filepath.Join(inside, ".ruby-version"))
	if rt := resolveRuby([]string{inside}, "", nil, env); rt.State != "" {
		t.Fatal("link inside the boundary", rt)
	}
	// "system" defers to PATH, as rbenv does.
	system := writeRuby(t, filepath.Join(outside, "usr"), "3.2.3")
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "system"})}, "", []string{filepath.Dir(system)}, env); rt.Interpreter != system || rt.Request != "" {
		t.Fatal("system", rt)
	}
}

func TestRubyFromPath(t *testing.T) {
	home, env := rubyHost(t)
	project := rubyProject(t, nil)
	// rbenv/asdf shims are scripts; mise shims link to the mise binary.
	rbenvShims := filepath.Join(home, ".rbenv/shims")
	writeTree(t, rbenvShims, map[string]string{"ruby": "#!/usr/bin/env bash\nexec rbenv exec ruby \"$@\"\n"})
	if err := os.Chmod(filepath.Join(rbenvShims, "ruby"), 0o755); err != nil {
		t.Fatal(err)
	}
	miseBin := filepath.Join(home, ".local/bin/mise")
	writeTree(t, home, map[string]string{".local/bin/mise": "\xcf\xfa\xed\xfe synthetic mise"})
	if err := os.Chmod(miseBin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Even beside an rbconfig.rb, the mise binary is not an interpreter.
	writeTree(t, home, map[string]string{".local/lib/ruby/3.3.0/arm64-darwin25/rbconfig.rb": rbConfig("3.3.5")})
	miseShims := filepath.Join(home, ".local/share/mise/shims")
	symlink(t, miseBin, filepath.Join(miseShims, "ruby"))
	brew := writeRuby(t, filepath.Join(home, "homebrew/Cellar/ruby/3.4.1"), "3.4.1")
	brewOpt := filepath.Join(home, "homebrew/opt/ruby")
	symlink(t, filepath.Join(home, "homebrew/Cellar/ruby/3.4.1"), brewOpt)
	shims := []string{rbenvShims, miseShims, filepath.Join(home, "missing")}

	rt := resolveRuby([]string{project}, "", append(shims, filepath.Join(brewOpt, "bin")), env)
	if rt.State != "" || rt.Interpreter != brew || rt.Version != "3.4.1" || rt.ABI != "3.4.0" || !rt.ShimSkipped || rt.Request != "" {
		t.Fatal("homebrew after shims", rt)
	}
	if rt := resolveRuby([]string{project}, "", shims, env); rt.State != "foreign" || !rt.ShimSkipped {
		t.Fatal("only shims", rt)
	}
	if rt := resolveRuby([]string{project}, "", []string{filepath.Join(home, "missing"), "relative/bin"}, env); rt.State != "missing" {
		t.Fatal("none", rt)
	}
	// A mise-activated shell puts the install's bin directly on PATH.
	active := writeRuby(t, filepath.Join(home, ".local/share/mise/installs/ruby/4.0.9"), "4.0.9")
	if rt := resolveRuby([]string{project}, "", []string{filepath.Dir(active), filepath.Join(brewOpt, "bin")}, env); rt.Interpreter != active || rt.ShimSkipped {
		t.Fatal("mise activate", rt)
	}
	// Debian: /usr/bin/ruby -> ruby3.1, rbconfig.rb under lib/<multiarch>/ruby.
	debian := filepath.Join(home, "debian/usr")
	writeTree(t, debian, map[string]string{"bin/ruby3.1": fakeRubyBinary, "lib/x86_64-linux-gnu/ruby/3.1.0/rbconfig.rb": rbConfig("3.1.2")})
	if err := os.Chmod(filepath.Join(debian, "bin/ruby3.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlink(t, "ruby3.1", filepath.Join(debian, "bin/ruby"))
	if rt := resolveRuby([]string{project}, "", []string{filepath.Join(debian, "bin")}, env); rt.State != "" || rt.Interpreter != filepath.Join(debian, "bin/ruby3.1") || rt.ABI != "3.1.0" {
		t.Fatal("debian", rt)
	}
}

func TestRubyTooOld(t *testing.T) {
	home, env := rubyHost(t)
	project := rubyProject(t, nil)
	// macOS's /usr/bin/ruby reads rbconfig.rb through the /usr/lib/ruby link.
	usr := filepath.Join(home, "usr")
	framework := filepath.Join(home, "System/Ruby.framework/Versions/2.6/usr/lib/ruby")
	writeTree(t, usr, map[string]string{"bin/ruby": fakeRubyBinary})
	if err := os.Chmod(filepath.Join(usr, "bin/ruby"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTree(t, framework, map[string]string{"2.6.0/universal-darwin26/rbconfig.rb": rbConfig("2.6.10")})
	symlink(t, framework, filepath.Join(usr, "lib/ruby"))
	rt := resolveRuby([]string{project}, "", []string{filepath.Join(usr, "bin")}, env)
	if rt.State != "tooOld" || rt.Version != "2.6.10" || rt.Interpreter != filepath.Join(usr, "bin/ruby") {
		t.Fatal("system ruby", rt)
	}
	// A Finder-launched app's inherited /usr/bin precedes the login shell's
	// mise directory; the usable Ruby after it wins.
	mise := writeRuby(t, filepath.Join(home, ".local/share/mise/installs/ruby/4.0.9"), "4.0.9")
	if rt := resolveRuby([]string{project}, "", []string{filepath.Join(usr, "bin"), filepath.Dir(mise)}, env); rt.State != "" || rt.Interpreter != mise {
		t.Fatal("too-old ruby skipped", rt)
	}
	writeRuby(t, filepath.Join(home, ".rbenv/versions/2.6.10"), "2.6.10")
	rt = resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "2.6.10"})}, "", nil, env)
	if rt.State != "tooOld" || rt.Request != "2.6.10" {
		t.Fatal("requested old ruby", rt)
	}
	writeRuby(t, filepath.Join(home, ".rbenv/versions/2.7.0"), "2.7.0")
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "2.7.0"})}, "", nil, env); rt.State != "" {
		t.Fatal("2.7 is the minimum", rt)
	}
}

func TestRubyUnidentifiableInstalls(t *testing.T) {
	home, env := rubyHost(t)
	versions := filepath.Join(home, ".rbenv/versions")
	project := rubyProject(t, map[string]string{".ruby-version": "3.3.5"})
	prefix := filepath.Join(versions, "3.3.5")
	rbconfig := filepath.Join(prefix, "lib/ruby/3.3.0/arm64-darwin25/rbconfig.rb")
	writeRuby(t, prefix, "3.3.5")
	for name, body := range map[string]string{
		"no version":      "module RbConfig\n  CONFIG = {}\nend\n",
		"abi mismatch":    strings.Replace(rbConfig("3.3.5"), `"3.3.0"`, `"3.2.0"`, 1),
		"duplicate key":   rbConfig("3.3.5") + "  CONFIG[\"MAJOR\"] = \"9\"\n",
		"non-numeric":     strings.Replace(rbConfig("3.3.5"), `"5"`, `"5a"`, 1),
		"oversized":       rbConfig("3.3.5") + strings.Repeat("#", maxRbConfig),
		"computed values": strings.Replace(rbConfig("3.3.5"), `CONFIG["TEENY"] = "5"`, `CONFIG["TEENY"] = TEENY`, 1),
	} {
		writeTree(t, filepath.Dir(rbconfig), map[string]string{"rbconfig.rb": body})
		if rt := resolveRuby([]string{project}, "", nil, env); rt.State != "notInstalled" || rt.Interpreter != "" {
			t.Errorf("%s: %+v", name, rt)
		}
	}
	if err := os.Remove(rbconfig); err != nil {
		t.Fatal(err)
	}
	if rt := resolveRuby([]string{project}, "", nil, env); rt.State != "notInstalled" {
		t.Fatal("missing rbconfig", rt)
	}
	if rt := resolveRuby([]string{project}, "", []string{filepath.Join(prefix, "bin")}, env); rt.State != "notInstalled" {
		t.Fatal("a request is never replaced by PATH", rt)
	}
	if rt := resolveRuby([]string{rubyProject(t, nil)}, "", []string{filepath.Join(prefix, "bin")}, env); rt.State != "foreign" {
		t.Fatal("unidentifiable PATH ruby", rt)
	}

	// bin/ruby linking out of its prefix is not that install's interpreter.
	other, _ := filepath.EvalSymlinks(t.TempDir())
	outside := writeRuby(t, filepath.Join(other, "3.4.1"), "3.4.1")
	link := filepath.Join(versions, "3.4.1")
	writeTree(t, link, map[string]string{"lib/ruby/3.4.0/arm64-darwin25/rbconfig.rb": rbConfig("3.4.1")})
	symlink(t, outside, filepath.Join(link, "bin/ruby"))
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "3.4.1"})}, "", nil, env); rt.State != "notInstalled" {
		t.Fatal("link leaving the prefix", rt)
	}
	// A script named ruby in an install directory is not an interpreter.
	script := filepath.Join(versions, "3.2.6")
	writeRuby(t, script, "3.2.6")
	writeTree(t, script, map[string]string{"bin/ruby": "#!/bin/sh\nexec /usr/bin/ruby \"$@\"\n"})
	if rt := resolveRuby([]string{rubyProject(t, map[string]string{".ruby-version": "3.2.6"})}, "", nil, env); rt.State != "notInstalled" {
		t.Fatal("script", rt)
	}
}

func TestNormalizeRubyVersion(t *testing.T) {
	for in, want := range map[string]string{"3.3.5": "3.3.5", " ruby-3.3.5\n": "3.3.5", "ruby 3.3.5p100": "3.3.5", "2.7.8-p225": "2.7.8", "4": "4", "latest": "latest", "system": "system", "3.4.0-rc1": "3.4.0-rc1"} {
		if got, ok := normalizeRubyVersion(in); !ok || got != want {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "jruby-9.4", "mruby-3", "3.3.5 extra", "../3.3", "3.3.5.1.2", "v3.3"} {
		if got, ok := normalizeRubyVersion(in); ok {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}

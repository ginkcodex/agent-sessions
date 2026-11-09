package codediagnostics

// SYNTHETIC npm and native-Node fixtures. These tests execute no server and
// establish adapter handling only, not real Intelephense behavior.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func phpWritePackage(t *testing.T, dir string) {
	t.Helper()
	writeTree(t, dir, map[string]string{
		"package.json":           `{"name":"intelephense","version":"1.18.5","bin":{"intelephense":"./lib/intelephense.js"}}`,
		"lib/intelephense.js":    "#!/usr/bin/env node\nthrow new Error('synthetic entry must never execute');\n",
		"lib/stub/Core/Core.php": "<?php // synthetic bundled data",
	})
}

func phpHost(t *testing.T) (reviewed []string, env PHPEnvironment, pkg string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(base, "prefix")
	pkg = filepath.Join(prefix, "lib", "node_modules", phpProviderID)
	phpWritePackage(t, pkg)
	writeNode(t, prefix, "22.23.2")
	if err := os.Symlink("../lib/node_modules/intelephense/lib/intelephense.js", filepath.Join(prefix, "bin", phpProviderID)); err != nil {
		t.Fatal(err)
	}
	return []string{filepath.Join(prefix, "bin")}, PHPEnvironment{Home: base}, pkg
}

func TestPHPLanguageAndProjectDiscovery(t *testing.T) {
	for _, file := range []string{"a.php", "src/A.PHP", "a.phtml", "templates/view.PHTML"} {
		if lang, ok := PHPLanguageID(file); !ok || lang != "php" {
			t.Errorf("%q: %s %v", file, lang, ok)
		}
	}
	for _, file := range []string{"a.php.txt", "a.html", "a.phar", "a.inc", "composer.json", "php"} {
		if _, ok := PHPLanguageID(file); ok {
			t.Errorf("unexpected PHP ownership: %s", file)
		}
	}
	for name, tc := range map[string]struct {
		files             map[string]string
		configs, packages []string
		sources           bool
	}{
		"composer":             {map[string]string{"composer.json": "{}", "composer.lock": "{}", "src/a.php": ""}, []string{"composer.json", "composer.lock"}, []string{""}, true},
		"standalone":           {map[string]string{"templates/a.phtml": ""}, nil, nil, true},
		"nested":               {map[string]string{"one/composer.json": "{}", "two/composer.json": "{}", "one/a.php": ""}, []string{"one/composer.json", "two/composer.json"}, []string{"one", "two"}, true},
		"skipped dependencies": {map[string]string{"vendor/p/composer.json": "{}", "vendor/p/a.php": "", "node_modules/p/a.php": "", "target/a.php": "", ".gradle/a.php": ""}, nil, nil, false},
		"unrelated":            {map[string]string{"a.ts": "", "package.json": "{}"}, nil, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			w, err := discoverProjectDirs(goplsRoot(t, tc.files), phpProjectMarkers)
			if err != nil || w.partial || w.sources != tc.sources || !slices.Equal(w.configs, tc.configs) || !slices.Equal(w.packages, tc.packages) {
				t.Fatalf("%+v %v", w, err)
			}
		})
	}
}

func TestDiscoverPHPAvailable(t *testing.T) {
	root := goplsRoot(t, map[string]string{"composer.json": "{}", "composer.lock": "{}", "src/a.php": "<?php", "nested/composer.json": "{}"})
	reviewed, env, pkg := phpHost(t)
	p, err := DiscoverPHP(root, reviewed, env)
	if err != nil {
		t.Fatal(err)
	}
	if p.Availability() != "available" || p.Version() != "1.18.5" || p.NodeVersion() != "22.23.2" || p.ProjectLocal() {
		t.Fatalf("%+v", p)
	}
	cfg, ok := p.Server()
	if !ok || cfg.ProviderID != phpProviderID || cfg.Executable != filepath.Join(reviewed[0], "node") || !slices.Equal(cfg.Args, []string{filepath.Join(pkg, "lib", "intelephense.js"), "--stdio"}) || !cfg.DeclareDiagnostics {
		t.Fatalf("%+v", cfg)
	}
	if !slices.Equal(cfg.Env, []string{"HOME=" + env.Home, "PATH=" + reviewed[0], "NODE_DISABLE_COMPILE_CACHE=1"}) {
		t.Fatalf("environment %v", cfg.Env)
	}
	if !slices.Equal(cfg.ConfigPaths, []string{"composer.json", "composer.lock", "nested/composer.json"}) {
		t.Fatalf("configs %v", cfg.ConfigPaths)
	}
	if err := validateCommand(root, cfg.Tools(), cfg.command()); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Intelephense struct {
			Files       struct{ Associations []string }
			Diagnostics struct {
				Enable bool
				Run    string
			}
			Format      struct{ Enable bool }
			Telemetry   struct{ Enabled bool }
			Environment struct{ IncludePaths []string }
		}
	}
	if err := json.Unmarshal(cfg.Settings, &settings); err != nil {
		t.Fatal(err)
	}
	s := settings.Intelephense
	if !slices.Equal(s.Files.Associations, []string{"*.php", "*.phtml"}) || !s.Diagnostics.Enable || s.Diagnostics.Run != "onType" || s.Format.Enable || s.Telemetry.Enabled || s.Environment.IncludePaths == nil {
		t.Fatalf("settings %s", cfg.Settings)
	}
	st := p.Provider()
	if st.ProviderID != phpProviderID || st.Availability != "available" || st.Coverage.Kind != "openedDocuments" || st.Coverage.Partial || st.Coverage.ExternalChanges != CoverageExternalChanges || !slices.Equal(st.Coverage.Roots, []string{""}) || !slices.Equal(st.DocumentExtensions, phpDocumentExtensions) {
		t.Fatalf("%+v", st)
	}
	for _, text := range []string{"proprietary", "are free", "premium", "opened documents only", "not a whole-project", "unwatched", "No PHP runtime"} {
		if !strings.Contains(st.Notice, text) {
			t.Errorf("notice lacks %q: %s", text, st.Notice)
		}
	}
	st.DocumentExtensions[0] = ".bad"
	st.Coverage.Roots[0] = "bad"
	projects := p.Projects()
	projects[0] = "bad"
	if p.Provider().DocumentExtensions[0] != ".php" || !p.covers("src/a.php") || p.Projects()[0] != "" {
		t.Fatal("caller mutated plan")
	}
	// Every executable input and contained Composer config changes approval.
	if len(cfg.Toolchain) != 3 {
		t.Fatalf("toolchain %+v", cfg.Toolchain)
	}
	for _, path := range []string{cfg.Toolchain[0].Executable, cfg.Toolchain[1].Executable, cfg.Toolchain[2].Executable, cfg.Executable, filepath.Join(root.Project().Root, "composer.json"), filepath.Join(root.Project().Root, "composer.lock")} {
		before, err := Fingerprint(root, cfg.Tools())
		if err != nil {
			t.Fatal(err)
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.WriteString("\nchanged")
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal(writeErr, closeErr)
		}
		after, err := Fingerprint(root, cfg.Tools())
		if err != nil || before == after {
			t.Fatalf("input not bound: %s: %v", path, err)
		}
	}
}

func TestDiscoverPHPUnavailable(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate        func(string, string)
		state, notice string
	}{
		"missing server": {func(bin, _ string) { _ = os.Remove(filepath.Join(bin, phpProviderID)) }, "missingTool", "npm install -g intelephense"},
		"missing node":   {func(bin, _ string) { _ = os.Remove(filepath.Join(bin, "node")) }, "missingTool", "Node.js, required"},
		"script node": {func(bin, _ string) {
			writeTree(t, bin, map[string]string{"node": "#!/bin/sh\ntouch SHOULD_NOT_EXIST\n"})
		}, "missingTool", "Node.js, required"},
		"old node":              {func(bin, _ string) { writeNode(t, filepath.Dir(bin), "18.20.0") }, "unsupported", "too old"},
		"missing node metadata": {func(bin, _ string) { _ = os.Remove(filepath.Join(filepath.Dir(bin), "include/node/node_version.h")) }, "unsupported", "version could not be verified"},
		"foreign name": {func(_, pkg string) {
			writeTree(t, pkg, map[string]string{"package.json": `{"name":"not-intelephense","version":"1.18.5","bin":"lib/intelephense.js"}`})
		}, "unsupported", "could not be verified"},
		"malformed metadata": {func(_, pkg string) { writeTree(t, pkg, map[string]string{"package.json": `{`}) }, "unsupported", "could not be verified"},
		"hostile version": {func(_, pkg string) {
			writeTree(t, pkg, map[string]string{"package.json": `{"name":"intelephense","version":"1.2\n<script>","bin":"lib/intelephense.js"}`})
		}, "unsupported", "could not be verified"},
		"missing bin": {func(_, pkg string) {
			writeTree(t, pkg, map[string]string{"package.json": `{"name":"intelephense","version":"1.18.5"}`})
		}, "unsupported", "could not be verified"},
		"escaping bin": {func(_, pkg string) {
			writeTree(t, pkg, map[string]string{"package.json": `{"name":"intelephense","version":"1.18.5","bin":{"intelephense":"../../evil.js"}}`})
		}, "unsupported", "could not be verified"},
		"wrapper bin": {func(_, pkg string) {
			writeTree(t, pkg, map[string]string{"package.json": `{"name":"intelephense","version":"1.18.5","bin":{"intelephense":"wrapper.js"}}`})
		}, "unsupported", "could not be verified"},
		"missing stubs": {func(_, pkg string) { _ = os.RemoveAll(filepath.Join(pkg, "lib/stub")) }, "unsupported", "incomplete"},
		"empty entry":   {func(_, pkg string) { writeTree(t, pkg, map[string]string{"lib/intelephense.js": ""}) }, "unsupported", "could not be verified"},
		"metadata symlink": {func(_, pkg string) {
			original := filepath.Join(pkg, "package.json")
			_ = os.Rename(original, original+".real")
			_ = os.Symlink("package.json.real", original)
		}, "unsupported", "could not be verified"},
	} {
		t.Run(name, func(t *testing.T) {
			reviewed, env, pkg := phpHost(t)
			tc.mutate(reviewed[0], pkg)
			p, err := DiscoverPHP(goplsRoot(t, map[string]string{"a.php": "<?php"}), reviewed, env)
			if err != nil {
				t.Fatal(err)
			}
			if p.Availability() != tc.state || !strings.Contains(p.Provider().Notice, tc.notice) {
				t.Fatalf("%s: %s", p.Availability(), p.Provider().Notice)
			}
			if _, ok := p.Server(); ok || !p.Provider().Coverage.Partial {
				t.Fatal("unavailable plan offered server/full coverage")
			}
		})
	}
}

func TestPHPWrappersLocalAndReviewedRuntime(t *testing.T) {
	root := goplsRoot(t, map[string]string{"a.phtml": "<?php"})
	reviewed, env, _ := phpHost(t)
	shim := t.TempDir()
	writeTree(t, shim, map[string]string{phpProviderID: "#!/bin/sh\ntouch SHOULD_NOT_EXIST\n", "node": "#!/bin/sh\ntouch SHOULD_NOT_EXIST\n"})
	_ = os.Chmod(filepath.Join(shim, "node"), 0o755)
	p, err := DiscoverPHP(root, []string{shim}, env)
	if err != nil || p.Availability() != "unsupported" || !strings.Contains(p.Provider().Notice, "version-manager shim") {
		t.Fatalf("%+v %v", p, err)
	}
	p, err = DiscoverPHP(root, append([]string{shim, "relative", "../bin"}, reviewed...), env)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%+v %v", p, err)
	}
	local := filepath.Join(root.Project().Root, "node_modules/intelephense")
	phpWritePackage(t, local)
	if err := os.MkdirAll(filepath.Join(root.Project().Root, "node_modules/.bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	command := filepath.Join(root.Project().Root, "node_modules/.bin/intelephense")
	if err := os.Symlink("../intelephense/lib/intelephense.js", command); err != nil {
		t.Fatal(err)
	}
	p, err = DiscoverPHP(root, reviewed, env)
	if err != nil || p.Availability() != "available" || !p.ProjectLocal() || p.config.Args[0] != filepath.Join(local, "lib/intelephense.js") {
		t.Fatalf("%+v %v", p, err)
	}
	// A project-local link escaping the root is not project authority.
	_, _, external := phpHost(t)
	_ = os.Remove(command)
	if err := os.Symlink(filepath.Join(external, "lib/intelephense.js"), command); err != nil {
		t.Fatal(err)
	}
	p, err = DiscoverPHP(root, nil, env)
	if err != nil || p.Availability() != "unsupported" {
		t.Fatalf("escaping local package: %+v %v", p, err)
	}
	// A global install's adjacent native Node wins over another reviewed Node.
	other := t.TempDir()
	writeNode(t, other, "24.1.0")
	p, err = DiscoverPHP(goplsRoot(t, map[string]string{"a.php": ""}), append([]string{filepath.Join(other, "bin"), shim}, reviewed...), env)
	if err != nil || p.Availability() != "available" || p.config.Executable != filepath.Join(reviewed[0], "node") {
		t.Fatalf("%+v %v", p, err)
	}
	// A separate reviewed native Node is used when the adjacent one is absent.
	if err := os.Remove(filepath.Join(reviewed[0], "node")); err != nil {
		t.Fatal(err)
	}
	p, err = DiscoverPHP(goplsRoot(t, map[string]string{"a.php": ""}), append([]string{shim, filepath.Join(other, "bin")}, reviewed...), env)
	if err != nil || p.Availability() != "available" || p.config.Executable != filepath.Join(other, "bin", "node") || p.NodeVersion() != "24.1.0" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestPHPInputsAndReadOnlyDiscovery(t *testing.T) {
	reviewed, env, _ := phpHost(t)
	for _, files := range []map[string]string{{"a.php": ""}, {"a.phtml": ""}, {"composer.json": "{}"}} {
		root := goplsRoot(t, files)
		before, err := os.ReadDir(root.Project().Root)
		if err != nil {
			t.Fatal(err)
		}
		p, err := DiscoverPHP(root, reviewed, env)
		if err != nil || p.Availability() != "available" {
			t.Fatalf("%+v %v", p, err)
		}
		after, err := os.ReadDir(root.Project().Root)
		if err != nil || len(before) != len(after) {
			t.Fatal("discovery wrote project files", err)
		}
		for file, text := range files {
			got, err := os.ReadFile(filepath.Join(root.Project().Root, file))
			if err != nil || string(got) != text {
				t.Fatalf("discovery modified %s: %v", file, err)
			}
		}
	}
	root := goplsRoot(t, map[string]string{"a.php": ""})
	for _, bad := range []PHPEnvironment{{}, {Home: "relative"}, {Home: "/x/../y"}, {Home: "/x\nBAD=yes"}} {
		if _, err := DiscoverPHP(root, reviewed, bad); errCode(err) != "invalidArgument" {
			t.Fatalf("bad home %+v: %v", bad, err)
		}
	}
	p, err := DiscoverPHP(goplsRoot(t, map[string]string{"a.go": ""}), reviewed, PHPEnvironment{})
	if err != nil || p.Availability() != "unsupported" || !strings.Contains(p.Provider().Notice, "No PHP file") {
		t.Fatalf("%+v %v", p, err)
	}
	// A supplied shared scan is used directly; no second discovery walk.
	p, err = discoverPHPWorkspace(goplsRoot(t, nil), reviewed, env, &tsWorkspace{sources: true, partial: true})
	if err != nil || p.Availability() != "available" || !p.Provider().Coverage.Partial {
		t.Fatalf("%+v %v", p, err)
	}
	// npm's shorthand string bin form and unprefixed standard entry both work.
	for _, bin := range []string{`"lib/intelephense.js"`, `{"intelephense":"lib/intelephense.js"}`} {
		path, e, pkg := phpHost(t)
		writeTree(t, pkg, map[string]string{"package.json": `{"name":"intelephense","version":"1.17.0","bin":` + bin + `}`})
		p, err := DiscoverPHP(root, path, e)
		if err != nil || p.Availability() != "available" || p.Version() != "1.17.0" {
			t.Fatalf("%+v %v", p, err)
		}
	}
}

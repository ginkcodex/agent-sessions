package codediagnostics

// Synthetic metadata only: no Java, wrapper, Maven or Gradle is ever executed.
// These tests establish adapter behavior, not real JDT LS protocol compatibility.

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func javaHost(t *testing.T) ([]string, JavaEnvironment, string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dist := filepath.Join(base, "dist")
	platform := map[string]string{"linux": "linux", "darwin": "mac", "windows": "win"}[runtime.GOOS]
	writeTree(t, dist, map[string]string{
		"bin/jdtls": "#!/bin/sh\ntouch MUST_NOT_RUN\n",
		"plugins/org.eclipse.equinox.launcher_1.7.0.jar":                              "synthetic launcher",
		"plugins/org.eclipse.jdt.ls.core_1.57.0.jar":                                  "synthetic core",
		"plugins/other.jar":                                                           "synthetic bundle",
		"config_" + platform + "/config.ini":                                          "osgi.bundles=reference:file:org.eclipse.jdt.ls.core_1.57.0.jar\n",
		"config_" + platform + "/org.eclipse.equinox.simpleconfigurator/bundles.info": "synthetic bundle list",
	})
	native := "\x7fELF synthetic Java"
	if runtime.GOOS == "darwin" {
		native = "\xcf\xfa\xed\xfe synthetic Java"
	}
	if runtime.GOOS == "windows" {
		native = "MZxx synthetic Java"
	}
	jdk := filepath.Join(base, "jdk")
	javaName, jvm, jli := "bin/java", "lib/server/libjvm.so", "lib/libjli.so"
	if runtime.GOOS == "darwin" {
		jvm, jli = "lib/server/libjvm.dylib", "lib/libjli.dylib"
	}
	if runtime.GOOS == "windows" {
		javaName, jvm, jli = "bin/java.exe", "bin/server/jvm.dll", "bin/jli.dll"
	}
	writeTree(t, jdk, map[string]string{javaName: native, "release": "JAVA_VERSION=\"25.0.1\"\n", jvm: "synthetic JVM", jli: "synthetic JLI"})
	if err := os.Chmod(filepath.Join(jdk, javaName), 0755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dist, "bin", "jdtls"), filepath.Join(bin, "jdtls")); err != nil {
		t.Fatal(err)
	}
	return []string{bin, filepath.Join(jdk, "bin")}, JavaEnvironment{Home: base, Cache: filepath.Join(base, "cache"), JavaHome: jdk}, dist
}

func TestJavaProjectDiscovery(t *testing.T) {
	for name, tc := range map[string]struct {
		files   map[string]string
		sources bool
		configs []string
	}{
		"standalone":        {map[string]string{"src/Main.java": ""}, true, nil},
		"maven modules":     {map[string]string{"pom.xml": "", "module/pom.xml": "", "module/src/Main.java": ""}, true, []string{"module/pom.xml", "pom.xml"}},
		"gradle":            {map[string]string{"build.gradle.kts": "", "settings.gradle.kts": "", "gradle/wrapper/gradle-wrapper.properties": ""}, false, []string{"build.gradle.kts", "gradle/wrapper/gradle-wrapper.properties", "settings.gradle.kts"}},
		"generated skipped": {map[string]string{"target/Main.java": "", "vendor/Main.java": "", "build/Other.java": "", ".gradle/Cache.java": ""}, false, nil},
		"unrelated":         {map[string]string{"app.ts": ""}, false, nil},
	} {
		t.Run(name, func(t *testing.T) {
			w, err := discoverProjectDirs(goplsRoot(t, tc.files), javaProjectMarkers)
			if err != nil || w.sources != tc.sources || !slices.Equal(w.configs, tc.configs) {
				t.Fatalf("%+v %v", w, err)
			}
		})
	}
	for _, file := range []string{"Main.java", "src/Main.JAVA"} {
		if lang, ok := JavaLanguageID(file); !ok || lang != "java" {
			t.Fatal(file)
		}
	}
	for _, file := range []string{"Main.class", "Main.kt", "Main.jav"} {
		if _, ok := JavaLanguageID(file); ok {
			t.Fatal(file)
		}
	}
}

func TestJavaFixedLaunchAndReadOnlyDiscovery(t *testing.T) {
	dirs, env, dist := javaHost(t)
	root := goplsRoot(t, map[string]string{"pom.xml": "<project/>", "src/Main.java": "class Main {}", ".mvn/extensions.xml": "<extensions/>"})
	p, err := DiscoverJava(root, dirs, env)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %+v", err, p)
	}
	cfg, ok := p.Server()
	if !ok || cfg.ProviderID != "jdtls" || cfg.Executable != filepath.Join(env.JavaHome, dirsJavaName()) || p.JavaVersion() != "25.0.1" || p.Version() != "1.57.0" {
		t.Fatalf("%+v %+v", p, cfg)
	}
	launcher := filepath.Join(dist, "plugins", "org.eclipse.equinox.launcher_1.7.0.jar")
	data, conf := javaArg(t, cfg, "-data"), javaArg(t, cfg, "-configuration")
	shared := filepath.Join(dist, "config_"+map[string]string{"linux": "linux", "darwin": "mac", "windows": "win"}[runtime.GOOS])
	want := []string{"-Declipse.application=org.eclipse.jdt.ls.core.id1", "-Dosgi.bundles.defaultStartLevel=4", "-Declipse.product=org.eclipse.jdt.ls.core.product", "-Dosgi.checkConfiguration=true", "-Dosgi.sharedConfiguration.area=" + shared, "-Dosgi.sharedConfiguration.area.readOnly=true", "-Dosgi.configuration.cascaded=true", "-Xmx1G", "--add-modules=ALL-SYSTEM", "--add-opens", "java.base/java.util=ALL-UNNAMED", "--add-opens", "java.base/java.lang=ALL-UNNAMED", "-jar", launcher, "-configuration", conf, "-data", data}
	if !slices.Equal(cfg.Args, want) || !cfg.DeclareDiagnostics || cfg.ValidateStart == nil {
		t.Fatalf("%+v", cfg)
	}
	if !slices.Equal(cfg.Env, []string{"HOME=" + env.Home, "JAVA_HOME=" + env.JavaHome, "PATH=" + filepath.Dir(cfg.Executable) + string(os.PathListSeparator) + "/usr/bin:/bin"}) {
		t.Fatalf("env %v", cfg.Env)
	}
	if !slices.Equal(cfg.ConfigPaths, []string{".mvn/extensions.xml", "pom.xml"}) {
		t.Fatal(cfg.ConfigPaths)
	}
	if len(cfg.Tools()) != 5 {
		t.Fatalf("unexpected tool budget %d", len(cfg.Tools()))
	}
	for _, value := range []string{data, conf} {
		if !filepath.IsAbs(value) || within(root.Project().Root, value) || within(dist, value) {
			t.Fatal(value)
		}
	}
	if err := cfg.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(env.Cache); !os.IsNotExist(err) {
		t.Fatalf("discovery wrote cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dist, "MUST_NOT_RUN")); !os.IsNotExist(err) {
		t.Fatal("wrapper ran")
	}
	state := p.Provider()
	if state.Coverage.Kind != "openedDocuments" || state.Coverage.ExternalChanges != "unwatched" || !p.covers("src/Main.java") || !slices.Equal(state.DocumentExtensions, []string{".java"}) {
		t.Fatalf("%+v", state)
	}
	for _, notice := range []string{"execute Maven/Gradle", "download dependencies", "source-file", "jdt:", "opened documents only"} {
		if !strings.Contains(state.Notice, notice) {
			t.Fatalf("missing %q in %s", notice, state.Notice)
		}
	}
	if !strings.Contains(string(cfg.Settings), `"wrapper":{"enabled":true}`) || !strings.Contains(string(cfg.InitializationOptions), `"classFileContentsSupport":false`) {
		t.Fatal("missing import/navigation options")
	}
	again, err := DiscoverJava(root, dirs, env)
	if err != nil {
		t.Fatal(err)
	}
	ac, _ := again.Server()
	if javaArg(t, ac, "-data") != data || javaArg(t, ac, "-configuration") != conf {
		t.Fatal("nondeterministic cache")
	}
	other := goplsRoot(t, map[string]string{"Main.java": ""})
	op, err := DiscoverJava(other, dirs, env)
	if err != nil {
		t.Fatal(err)
	}
	oc, _ := op.Server()
	if javaArg(t, oc, "-data") == data || javaArg(t, oc, "-configuration") == conf {
		t.Fatal("projects share cache")
	}
	host := root
	host.project.HostID += "-other"
	hp, err := DiscoverJava(host, dirs, env)
	if err != nil {
		t.Fatal(err)
	}
	hc, _ := hp.Server()
	if javaArg(t, hc, "-data") == data {
		t.Fatal("hosts share cache")
	}
}

func dirsJavaName() string {
	if runtime.GOOS == "windows" {
		return filepath.Join("bin", "java.exe")
	}
	return filepath.Join("bin", "java")
}
func javaArg(t *testing.T, cfg ServerConfig, flag string) string {
	t.Helper()
	i := slices.Index(cfg.Args, flag)
	if i < 0 || i+1 >= len(cfg.Args) {
		t.Fatalf("missing %s: %v", flag, cfg.Args)
	}
	return cfg.Args[i+1]
}

func TestJavaUnavailableMetadata(t *testing.T) {
	for _, name := range []string{"no server", "foreign wrapper", "no Java", "script Java", "old Java", "no release", "invalid release", "no JVM", "missing launcher", "ambiguous launcher", "no config", "missing core", "cache in project", "cache in distribution"} {
		t.Run(name, func(t *testing.T) {
			dirs, env, dist := javaHost(t)
			root := goplsRoot(t, map[string]string{"Main.java": ""})
			state := "unsupported"
			switch name {
			case "no server":
				dirs = nil
				state = "missingTool"
			case "foreign wrapper":
				if err := os.Remove(filepath.Join(dirs[0], "jdtls")); err != nil {
					t.Fatal(err)
				}
				writeTree(t, dirs[0], map[string]string{"jdtls": "#!/bin/sh\n"})
			case "no Java":
				env.JavaHome += "-absent"
				state = "missingTool"
			case "script Java":
				if err := os.WriteFile(filepath.Join(env.JavaHome, dirsJavaName()), []byte("#!/bin/sh\n"), 0755); err != nil {
					t.Fatal(err)
				}
				state = "missingTool"
			case "old Java":
				writeTree(t, env.JavaHome, map[string]string{"release": "JAVA_VERSION=\"24.0.2\"\n"})
			case "no release":
				if err := os.Remove(filepath.Join(env.JavaHome, "release")); err != nil {
					t.Fatal(err)
				}
			case "invalid release":
				writeTree(t, env.JavaHome, map[string]string{"release": "JAVA_VERSION=arbitrary shell code\n"})
			case "no JVM":
				if err := os.RemoveAll(filepath.Join(env.JavaHome, "lib", "server")); err != nil {
					t.Fatal(err)
				}
				if runtime.GOOS == "windows" {
					if err := os.RemoveAll(filepath.Join(env.JavaHome, "bin", "server")); err != nil {
						t.Fatal(err)
					}
				}
			case "missing launcher":
				os.Remove(filepath.Join(dist, "plugins", "org.eclipse.equinox.launcher_1.7.0.jar"))
			case "ambiguous launcher":
				writeTree(t, dist, map[string]string{"plugins/org.eclipse.equinox.launcher_1.8.0.jar": ""})
			case "no config":
				if err := os.RemoveAll(filepath.Join(dist, "config_"+map[string]string{"linux": "linux", "darwin": "mac", "windows": "win"}[runtime.GOOS])); err != nil {
					t.Fatal(err)
				}
			case "missing core":
				os.Remove(filepath.Join(dist, "plugins", "org.eclipse.jdt.ls.core_1.57.0.jar"))
			case "cache in project":
				env.Cache = filepath.Join(root.Project().Root, "cache")
			case "cache in distribution":
				env.Cache = filepath.Join(dist, "cache")
			}
			p, err := DiscoverJava(root, dirs, env)
			if err != nil || p.Availability() != state {
				t.Fatalf("%v %+v", err, p)
			}
			if _, ok := p.Server(); ok {
				t.Fatal("unavailable plan exposes server")
			}
		})
	}
	root := goplsRoot(t, map[string]string{"Main.ts": ""})
	p, err := DiscoverJava(root, nil, JavaEnvironment{})
	if err != nil || p.Availability() != "unsupported" {
		t.Fatalf("unrelated language required Java environment: %v %+v", err, p)
	}
	dirs, env, _ := javaHost(t)
	env.Home = "relative"
	if _, err := DiscoverJava(goplsRoot(t, map[string]string{"Main.java": ""}), dirs, env); err == nil {
		t.Fatal("relative home accepted")
	}
}

func TestJavaConventionalLayoutAndRuntimePath(t *testing.T) {
	dirs, env, dist := javaHost(t)
	if err := os.Remove(filepath.Join(dirs[0], "jdtls")); err != nil {
		t.Fatal(err)
	}
	conventional := filepath.Join(filepath.Dir(dirs[0]), "share", "java", "jdtls")
	if err := os.MkdirAll(filepath.Dir(conventional), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dist, conventional); err != nil {
		t.Fatal(err)
	}
	env.JavaHome = ""
	env.Cache = ""
	p, err := DiscoverJava(goplsRoot(t, map[string]string{"pom.xml": ""}), dirs, env)
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %+v", err, p)
	}
	cfg, _ := p.Server()
	if !strings.HasPrefix(javaArg(t, cfg, "-data"), filepath.Join(env.Home, ".cache", "agent-sessions")) {
		t.Fatal(cfg.Args)
	}
}

func TestJavaFingerprintAndRevalidation(t *testing.T) {
	for _, name := range []string{"java", "release", "JVM", "bundle", "bundle added", "bundle removed", "configuration", "config added", "project build", "build added", "wrapper added", "cache link"} {
		t.Run(name, func(t *testing.T) {
			dirs, env, dist := javaHost(t)
			root := goplsRoot(t, map[string]string{"pom.xml": "<project/>", "Main.java": ""})
			p, err := DiscoverJava(root, dirs, env)
			if err != nil || p.Availability() != "available" {
				t.Fatalf("%v %+v", err, p)
			}
			cfg, _ := p.Server()
			before, err := Fingerprint(root, cfg.Tools())
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "java":
				f := filepath.Join(env.JavaHome, dirsJavaName())
				data, err := os.ReadFile(f)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(f, append(data, '!'), 0755); err != nil {
					t.Fatal(err)
				}
			case "release":
				writeTree(t, env.JavaHome, map[string]string{"release": "JAVA_VERSION=\"26.0.1\"\n"})
			case "JVM":
				for _, tool := range cfg.Toolchain {
					if strings.Contains(filepath.Base(tool.Executable), "jvm") {
						if err := os.WriteFile(tool.Executable, []byte("changed JVM"), 0644); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "bundle":
				writeTree(t, dist, map[string]string{"plugins/other.jar": "changed bundle bytes"})
			case "bundle added":
				writeTree(t, dist, map[string]string{"plugins/new.jar": "new bundle"})
			case "bundle removed":
				os.Remove(filepath.Join(dist, "plugins", "other.jar"))
			case "configuration":
				for _, tool := range cfg.Args {
					if shared, ok := strings.CutPrefix(tool, "-Dosgi.sharedConfiguration.area="); ok {
						writeTree(t, shared, map[string]string{"config.ini": "changed config"})
					}
				}
			case "config added":
				for _, tool := range cfg.Args {
					if shared, ok := strings.CutPrefix(tool, "-Dosgi.sharedConfiguration.area="); ok {
						writeTree(t, shared, map[string]string{"new.ini": "new config"})
					}
				}
			case "project build":
				writeTree(t, root.Project().Root, map[string]string{"pom.xml": "<changed-project/>"})
			case "build added":
				writeTree(t, root.Project().Root, map[string]string{"build.gradle": ""})
			case "wrapper added":
				writeTree(t, root.Project().Root, map[string]string{".mvn/wrapper/maven-wrapper.jar": "executable wrapper"})
			case "cache link":
				data := javaArg(t, cfg, "-data")
				if err := os.MkdirAll(filepath.Dir(data), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(root.Project().Root, data); err != nil {
					t.Fatal(err)
				}
			}
			// Project config bytes are checked by the ordinary approval Fingerprint;
			// enumeration and the compact server manifest are checked by ValidateStart.
			after, hashErr := Fingerprint(root, cfg.Tools())
			validation := cfg.ValidateStart()
			if validation == nil && hashErr == nil && before == after {
				t.Fatal("change did not invalidate approval")
			}
			if name != "cache link" {
				fresh, err := DiscoverJava(root, dirs, env)
				if err != nil {
					t.Fatal(err)
				}
				if next, ok := fresh.Server(); ok {
					now, err := Fingerprint(root, next.Tools())
					if err != nil || now == before {
						t.Fatalf("fresh fingerprint unchanged: %v", err)
					}
				}
			}
		})
	}
}

func TestJavaWorkspaceConfigAndCacheSafety(t *testing.T) {
	dirs, env, _ := javaHost(t)
	root := goplsRoot(t, map[string]string{"Main.java": "", "module/pom.xml": "", "module/.mvn/extensions.xml": "<extensions/>"})
	// The shared scan may omit sibling config ownership from a markerless root,
	// but JDT LS can still import it: the launch must bind those configurations.
	p, err := discoverJavaWorkspace(root, dirs, env, &tsWorkspace{sources: true})
	if err != nil || p.Availability() != "available" {
		t.Fatalf("%v %+v", err, p)
	}
	cfg, _ := p.Server()
	if !slices.Equal(cfg.ConfigPaths, []string{"module/.mvn/extensions.xml", "module/pom.xml"}) {
		t.Fatal(cfg.ConfigPaths)
	}
	if err := cfg.ValidateStart(); err != nil {
		t.Fatal(err)
	}
	data := javaArg(t, cfg, "-data")
	if err := os.MkdirAll(filepath.Dir(data), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root.Project().Root, data); err != nil {
		t.Fatal(err)
	}
	p, err = DiscoverJava(root, dirs, env)
	if err != nil || p.Availability() != "unsupported" {
		t.Fatalf("existing cache escape accepted: %v %+v", err, p)
	}
}

func TestJavaIdentityBounds(t *testing.T) {
	dirs, env, dist := javaHost(t)
	root := goplsRoot(t, map[string]string{"Main.java": ""})
	f, err := os.Create(filepath.Join(dist, "plugins", "oversize.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxIdentityFile + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	p, err := DiscoverJava(root, dirs, env)
	if err != nil || p.Availability() != "unsupported" {
		t.Fatalf("oversized bundle accepted: %v %+v", err, p)
	}
}

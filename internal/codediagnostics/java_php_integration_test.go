package codediagnostics

import (
	"slices"
	"testing"
)

func javaPHPEnv(t *testing.T) LocalLiveEnvironment {
	t.Helper()
	javaPath, java, _ := javaHost(t)
	phpPath, php, _ := phpHost(t)
	return LocalLiveEnvironment{Path: append(javaPath, phpPath...), Java: java, PHP: php, Disabled: NewDisabledLanguages([]string{"go", "typescript", "svelte", "python", "rust", "ruby"})}
}

func TestJavaPHPWorkspaceRouting(t *testing.T) {
	root := workspaceTestRoot(t, map[string]string{
		"app/pom.xml": "<project/>", "app/src/Main.java": "", "app/module/pom.xml": "<project/>", "app/module/src/Child.java": "",
		"other/settings.gradle": "", "other/src/Other.java": "", "Loose.java": "",
		"site/composer.json": "{}", "site/index.php": "", "site/sub/composer.json": "{}", "site/sub/view.phtml": "", "loose.php": "",
		"vendor/ignored/composer.json": "{}", "vendor/ignored/bad.php": "", "target/ignored/pom.xml": "<project/>", "target/ignored/Bad.java": "",
	})
	regs, err := DiscoverLocalLive(root, javaPHPEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(regs) != 2 {
		t.Fatalf("registrations %+v", regs)
	}
	wantRoots := map[string][]string{javaProviderID: {"", "app", "other"}, phpProviderID: {"", "site", "site/sub"}}
	for _, r := range regs {
		if r.Availability != "available" || !slices.Equal(r.Live.State.Coverage.Roots, wantRoots[r.ProviderID]) {
			t.Fatalf("%s %+v", r.ProviderID, r.Live.State)
		}
		for _, w := range r.Live.Workspaces {
			if w.Server.ValidateStart != nil {
				if err := w.Server.ValidateStart(); err != nil {
					t.Fatalf("%s %s: %v", r.ProviderID, w.Server.Directory, err)
				}
			}
		}
	}
	docs, err := DiscoveryDocuments(root, regs, nil)
	if err != nil {
		t.Fatal(err)
	}
	for file, dir := range map[string]string{"Loose.java": "", "app/src/Main.java": "app", "app/module/src/Child.java": "app", "other/src/Other.java": "other", "loose.php": "", "site/index.php": "site", "site/sub/view.phtml": "site/sub"} {
		servers := docs.ResolveAll(root, file)
		if len(servers) != 1 || servers[0].Config.Directory != dir {
			t.Errorf("%s want %q: %+v", file, dir, servers)
		}
	}
}

func TestJavaPHPDisabledSkipsInvalidToolEnvironment(t *testing.T) {
	root := workspaceTestRoot(t, map[string]string{"Main.java": "", "index.php": ""})
	env := javaPHPEnv(t)
	env.Disabled = NewDisabledLanguages([]string{"go", "typescript", "svelte", "python", "rust", "ruby", "java", "php"})
	env.Java = JavaEnvironment{Home: "relative", Cache: "relative", JavaHome: "relative"}
	env.PHP = PHPEnvironment{Home: "relative"}
	regs, err := DiscoverLocalLive(root, env)
	if err != nil || len(regs) != 0 {
		t.Fatalf("%v %+v", err, regs)
	}
	disabled := DisabledRegistrations(root, env.Disabled)
	if len(disabled) != 2 || disabled[0].ProviderID != javaProviderID || disabled[1].ProviderID != phpProviderID {
		t.Fatalf("%+v", disabled)
	}
	for _, r := range disabled {
		if r.Availability != "disabled" || r.Live.Server.Executable != "" {
			t.Fatalf("%+v", r)
		}
	}
}

func TestJavaPHPBridgeAndObservedSaves(t *testing.T) {
	for id, file := range map[string]string{javaProviderID: "src/Main.java", phpProviderID: "views/main.phtml"} {
		spec, ok := pushSpecs[id]
		if !ok || !spec.owns(file) || spec.owns("a.rb") {
			t.Errorf("bridge ownership %s", id)
		}
		if !watchedFileFor(id, file) || watchedFileFor(id, "a.rb") {
			t.Errorf("observed saves %s", id)
		}
	}
}

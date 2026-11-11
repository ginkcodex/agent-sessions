//go:build darwin || linux

package managedtools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPyrightInstallAndDiscovery(t *testing.T) {
	for _, hostVersion := range []string{"18.0.0", "17.9.0", ""} {
		t.Run("host-"+hostVersion, func(t *testing.T) {
			i, _ := fixture(t)
			host := filepath.Join(filepath.Dir(i.root), "host-node")
			writeTree(t, host, map[string]string{"bin/node": "node", "lib/node_modules/npm/bin/npm-cli.js": "npm"})
			if err := os.Chmod(filepath.Join(host, "bin/node"), 0o755); err != nil {
				t.Fatal(err)
			}
			i.UseHostNode(func() (string, string) { return host, hostVersion })
			node := tarGz(t, entry{name: "node-x/bin/node", body: "node", mode: 0o755}, entry{name: "node-x/lib/node_modules/npm/bin/npm-cli.js", body: "npm"})
			replace(t, nodeArchives, pin(node))
			serve(i, node)
			p, err := i.Propose("pyright")
			if err != nil {
				t.Fatal(err)
			}
			if p.Version != pyrightVersion || !strings.Contains(p.Source, "pyright "+pyrightVersion) || !strings.Contains(p.Integrity, "install scripts off") {
				t.Fatal(p)
			}
			nodeDir := filepath.Join(i.root, installDir("node", nodeVersion))
			if hostVersion == "18.0.0" {
				nodeDir = host
				if !strings.Contains(p.Note, "not downloaded") {
					t.Fatal(p)
				}
			} else if !strings.Contains(p.Note, "Also downloads Node.js") {
				t.Fatal(p)
			}
			ran := false
			i.command = func(_ context.Context, exe, cwd string, env []string, args ...string) ([]byte, error) {
				ran = true
				if exe != filepath.Join(nodeDir, "bin/node") || !slices.Equal(args, []string{filepath.Join(nodeDir, "lib/node_modules/npm/bin/npm-cli.js"), "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--loglevel=error"}) {
					t.Fatal(exe, args)
				}
				if !slices.Contains(env, "npm_config_ignore_scripts=true") || !slices.Contains(env, "npm_config_registry="+npmRegistry) {
					t.Fatal(env)
				}
				for name, want := range map[string][]byte{"package.json": pyrightPackageJSON, "package-lock.json": pyrightLockfile} {
					if got, err := os.ReadFile(filepath.Join(cwd, name)); err != nil || !bytes.Equal(got, want) {
						t.Fatal(name, err)
					}
				}
				writeTree(t, cwd, map[string]string{pyrightMain: "pyright", "node_modules/pyright/package.json": `{"name":"pyright","version":"` + pyrightVersion + `"}`})
				if err := os.Mkdir(filepath.Join(cwd, "node_modules/.bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				return nil, os.Symlink("../pyright/langserver.index.js", filepath.Join(cwd, "node_modules/.bin/pyright-langserver"))
			}
			path, err := i.Install(context.Background(), i.binding, p.Token)
			if err != nil || !ran || path != filepath.Join(p.Destination, pyrightMain) {
				t.Fatal(path, err, ran)
			}
			if got, err := i.Discover("pyright"); err != nil || got != path {
				t.Fatal(got, err)
			}
			if !slices.Contains(i.Dirs(), filepath.Join(p.Destination, "node_modules/.bin")) {
				t.Fatal(i.Dirs())
			}
			if _, err := os.Stat(filepath.Join(p.Destination, ".work")); !os.IsNotExist(err) {
				t.Fatal("scratch retained", err)
			}
			// Metadata, the command shim, and receipt-bound main bytes all matter.
			metadata := filepath.Join(p.Destination, "node_modules/pyright/package.json")
			if err := os.WriteFile(metadata, []byte(`{"name":"pyright","version":"0.0.0"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := i.Discover("pyright"); err == nil {
				t.Fatal("wrong version accepted")
			}
			if err := os.WriteFile(metadata, []byte(`{"name":"pyright","version":"`+pyrightVersion+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(p.Destination, "node_modules/.bin/pyright-langserver")); err != nil {
				t.Fatal(err)
			}
			if _, err := i.Discover("pyright"); err == nil {
				t.Fatal("missing command accepted")
			}
			if err := os.Symlink("../pyright/langserver.index.js", filepath.Join(p.Destination, "node_modules/.bin/pyright-langserver")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := i.Discover("pyright"); err == nil {
				t.Fatal("changed main accepted")
			}
		})
	}
}

func TestPyrightLockfilePinsOfficialPackages(t *testing.T) {
	var lock struct {
		LockfileVersion int
		Packages        map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(pyrightLockfile, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.LockfileVersion != 3 || len(lock.Packages) != pyrightLockPackages+1 {
		t.Fatal("unexpected lockfile")
	}
	for name, pkg := range lock.Packages {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "node_modules/") || !strings.HasPrefix(pkg.Resolved, npmRegistry) || !strings.HasPrefix(pkg.Integrity, "sha512-") {
			t.Fatal(name, pkg)
		}
	}
	pyright := lock.Packages["node_modules/pyright"]
	if pyright.Version != pyrightVersion || pyright.Integrity != "sha512-FPZZb51jepDX4eP7TEYDeNFtmE3WgwkkEcJpvH3/QmUSsj0EAy3LXu+xB4T/FWDejtsXlCfFr/rbWFjwuwuXww==" {
		t.Fatal(pyright)
	}
}

func TestSvelteLSLockfilePinsOfficialPackages(t *testing.T) {
	var lock struct {
		LockfileVersion int
		Packages        map[string]struct{ Version, Resolved, Integrity string }
	}
	if err := json.Unmarshal(svelteLSLockfile, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.LockfileVersion != 3 || len(lock.Packages) != svelteLSLockPackages+1 {
		t.Fatal("unexpected lockfile")
	}
	for name, pkg := range lock.Packages {
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "node_modules/") || !strings.HasPrefix(pkg.Resolved, npmRegistry) || !strings.HasPrefix(pkg.Integrity, "sha512-") {
			t.Fatal(name, pkg)
		}
	}
	server := lock.Packages["node_modules/svelte-language-server"]
	if server.Version != svelteLSVersion || server.Integrity != "sha512-AD4EGNmC5zt3b/boO5+Ng7rKW/qJFbna6iDdGotQhI4wqoI/3wxn8cWRi3GX8bMOK5d+hsJyOZbOYCJ+NFYqYg==" {
		t.Fatal(server)
	}
	// The versions the install validates are the ones the lockfile installs.
	for pkg, version := range svelteLSPackage().versions {
		if got := lock.Packages["node_modules/"+pkg].Version; got != version {
			t.Fatal(pkg, got, version)
		}
	}
}

func TestACPAdapterLockfilesPinOfficialPackages(t *testing.T) {
	for _, c := range []struct {
		pkg      npmPackage
		count    int
		manifest string
	}{
		{claudeACPPackage(), claudeACPLockPackages, "@agentclientprotocol/claude-agent-acp"},
		{codexACPPackage(), codexACPLockPackages, "@agentclientprotocol/codex-acp"},
	} {
		t.Run(c.pkg.name, func(t *testing.T) {
			var lock struct {
				LockfileVersion int
				Packages        map[string]struct {
					Version, Resolved, Integrity string
					Bin                          map[string]string
				}
			}
			if err := json.Unmarshal(c.pkg.lockfile, &lock); err != nil {
				t.Fatal(err)
			}
			if lock.LockfileVersion != 3 || len(lock.Packages) != c.count+1 {
				t.Fatal("unexpected lockfile", lock.LockfileVersion, len(lock.Packages))
			}
			for name, pkg := range lock.Packages {
				if name == "" {
					continue
				}
				if !strings.HasPrefix(name, "node_modules/") || !strings.HasPrefix(pkg.Resolved, npmRegistry) || !strings.HasPrefix(pkg.Integrity, "sha512-") {
					t.Fatal(name, pkg)
				}
			}
			// The manifest pins the adapter exactly, and the lockfile installs it.
			var manifest struct{ Dependencies map[string]string }
			if err := json.Unmarshal(c.pkg.manifest, &manifest); err != nil || len(manifest.Dependencies) != 1 || manifest.Dependencies[c.manifest] != c.pkg.version {
				t.Fatal(manifest, err)
			}
			for pkg, version := range c.pkg.versions {
				if got := lock.Packages["node_modules/"+pkg].Version; got != version {
					t.Fatal(pkg, got, version)
				}
			}
			// npm links the command to the main script the launch runs.
			adapter := lock.Packages["node_modules/"+c.manifest]
			if "node_modules/"+c.manifest+"/"+adapter.Bin[c.pkg.bin] != c.pkg.main {
				t.Fatal(adapter.Bin, c.pkg.main)
			}
			if !c.pkg.omitOptional || c.pkg.policy() != npmOmitOptionalPolicy || c.pkg.nodeMajor() != 22 {
				t.Fatal(c.pkg)
			}
		})
	}
	// Language servers keep their policy and Node.js minimum.
	if p := pyrightPackage(); p.omitOptional || p.policy() != npmPolicy || p.nodeMajor() != hostNodeMinMajor {
		t.Fatal(p)
	}
}

func TestACPAdapterInstallAndDiscovery(t *testing.T) {
	for _, tool := range []string{"claude-agent-acp", "codex-acp"} {
		// 22 is used; 20 runs language servers but not the adapters; none at all.
		for _, hostVersion := range []string{"22.0.0", "20.11.0", ""} {
			t.Run(tool+"/host-"+hostVersion, func(t *testing.T) {
				pkg := npmPackages[tool]()
				i, _ := fixture(t)
				host := filepath.Join(filepath.Dir(i.root), "host-node")
				writeTree(t, host, map[string]string{"bin/node": "node", "lib/node_modules/npm/bin/npm-cli.js": "npm"})
				if err := os.Chmod(filepath.Join(host, "bin/node"), 0o755); err != nil {
					t.Fatal(err)
				}
				i.UseHostNode(func() (string, string) { return host, hostVersion })
				node := tarGz(t, entry{name: "node-x/bin/node", body: "node", mode: 0o755}, entry{name: "node-x/lib/node_modules/npm/bin/npm-cli.js", body: "npm"})
				replace(t, nodeArchives, pin(node))
				serve(i, node)
				p, err := i.Propose(tool)
				if err != nil {
					t.Fatal(err)
				}
				if p.Version != pkg.version || !strings.Contains(p.Source, pkg.source) || !strings.Contains(p.Integrity, "install scripts off") || !strings.Contains(p.Integrity, "optional packages (platform binaries) left out") {
					t.Fatal(p)
				}
				nodeDir := filepath.Join(i.root, installDir("node", nodeVersion))
				switch hostVersion {
				case "22.0.0":
					nodeDir = host
					if !strings.Contains(p.Note, "Uses Node.js 22.0.0") {
						t.Fatal(p.Note)
					}
				case "20.11.0":
					if !strings.Contains(p.Note, "Node.js 20.11.0 on this host is older than 22. Also downloads Node.js") {
						t.Fatal(p.Note)
					}
				default:
					if !strings.HasPrefix(p.Note, "Also downloads Node.js") {
						t.Fatal(p.Note)
					}
				}
				ran := false
				i.command = func(_ context.Context, exe, cwd string, env []string, args ...string) ([]byte, error) {
					ran = true
					want := []string{filepath.Join(nodeDir, "lib/node_modules/npm/bin/npm-cli.js"), "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--loglevel=error", "--omit=optional"}
					if exe != filepath.Join(nodeDir, "bin/node") || !slices.Equal(args, want) {
						t.Fatal(exe, args)
					}
					if !slices.Contains(env, "npm_config_ignore_scripts=true") {
						t.Fatal(env)
					}
					for name, want := range map[string][]byte{"package.json": pkg.manifest, "package-lock.json": pkg.lockfile} {
						if got, err := os.ReadFile(filepath.Join(cwd, name)); err != nil || !bytes.Equal(got, want) {
							t.Fatal(name, err)
						}
					}
					files := map[string]string{pkg.main: "adapter"}
					for name, version := range pkg.versions {
						files["node_modules/"+name+"/package.json"] = `{"name":"` + name + `","version":"` + version + `"}`
					}
					writeTree(t, cwd, files)
					if err := os.Mkdir(filepath.Join(cwd, "node_modules/.bin"), 0o755); err != nil {
						t.Fatal(err)
					}
					return nil, os.Symlink("../"+strings.TrimPrefix(pkg.main, "node_modules/"), filepath.Join(cwd, "node_modules/.bin", pkg.bin))
				}
				path, err := i.Install(context.Background(), i.binding, p.Token)
				if err != nil || !ran || path != filepath.Join(p.Destination, pkg.main) {
					t.Fatal(path, err, ran)
				}
				if got, err := i.Discover(tool); err != nil || got != path {
					t.Fatal(got, err)
				}
				if hostVersion == "20.11.0" {
					// With the app's Node.js installed, nothing more is downloaded.
					again, err := i.Propose(tool)
					if err != nil || again.Note != "Node.js 20.11.0 on this host is older than 22. Uses the app's Node.js "+nodeVersion+"." {
						t.Fatal(again.Note, err)
					}
					i.Dismiss()
				}
				var r receipt
				if data, err := os.ReadFile(filepath.Join(p.Destination, "receipt.json")); err != nil || json.Unmarshal(data, &r) != nil || r.Policy != npmOmitOptionalPolicy || r.Version != pkg.version {
					t.Fatal(r, err)
				}
				// Code diagnostics does not search the adapters' commands.
				for _, dir := range i.Dirs() {
					if strings.HasPrefix(dir, p.Destination) {
						t.Fatal(i.Dirs())
					}
				}
				// An install under the language servers' policy is not this entry's.
				if err := os.WriteFile(filepath.Join(p.Destination, "receipt.json"), []byte(`{"Version":"`+r.Version+`","Policy":"`+npmPolicy+`","SHA256":"`+r.SHA256+`"}`), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := i.Discover(tool); err == nil {
					t.Fatal("receipt with another policy accepted")
				}
			})
		}
	}
}

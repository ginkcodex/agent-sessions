package managedtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// npmPackage describes a pinned npm server and its runtime metadata checks.
// bin is the command npm links to main; a TypeScript plugin has none.
// omitOptional installs without optional dependencies; minNode is the oldest
// host Node.js major the entry runs on (0: hostNodeMinMajor).
type npmPackage struct {
	name, version, main, bin, source string
	manifest, lockfile               []byte
	versions                         map[string]string
	omitOptional                     bool
	minNode                          int
}

// npmPackages are the catalog's npm installs by tool name.
var npmPackages = map[string]func() npmPackage{
	"vtsls": vtslsPackage, "pyright": pyrightPackage,
	"svelte-language-server": svelteLSPackage, "vue-typescript-plugin": vuePluginPackage,
	"claude-agent-acp": claudeACPPackage, "codex-acp": codexACPPackage,
}

func vtslsPackage() npmPackage {
	return npmPackage{
		name: "vtsls", version: vtslsVersion, main: vtslsMain, bin: "vtsls",
		source:   fmt.Sprintf("@vtsls/language-server %s, TypeScript %s, %d locked packages", vtslsVersion, vtslsTypeScript, vtslsLockPackages),
		manifest: vtslsPackageJSON, lockfile: vtslsLockfile,
		versions: map[string]string{"@vtsls/language-server": vtslsVersion, "typescript": vtslsTypeScript},
	}
}

func pyrightPackage() npmPackage {
	return npmPackage{
		name: "pyright", version: pyrightVersion, main: pyrightMain, bin: "pyright-langserver",
		source:   fmt.Sprintf("pyright %s, %d locked packages", pyrightVersion, pyrightLockPackages),
		manifest: pyrightPackageJSON, lockfile: pyrightLockfile,
		versions: map[string]string{"pyright": pyrightVersion},
	}
}

func svelteLSPackage() npmPackage {
	return npmPackage{
		name: "svelte-language-server", version: svelteLSVersion, main: svelteLSMain, bin: "svelteserver",
		source:   fmt.Sprintf("svelte-language-server %s, Svelte %s, TypeScript %s, %d locked packages", svelteLSVersion, svelteLSSvelte, svelteLSTypeScript, svelteLSLockPackages),
		manifest: svelteLSPackageJSON, lockfile: svelteLSLockfile,
		versions: map[string]string{"svelte-language-server": svelteLSVersion, "svelte-language-server/node_modules/svelte": svelteLSSvelte, "typescript": svelteLSTypeScript},
	}
}

func vuePluginPackage() npmPackage {
	return npmPackage{
		name: "vue-typescript-plugin", version: vuePluginVersion, main: vuePluginMain,
		source:   fmt.Sprintf("@vue/typescript-plugin %s, %d locked packages", vuePluginVersion, vuePluginLockPackages),
		manifest: vuePluginPackageJSON, lockfile: vuePluginLockfile,
		versions: map[string]string{"@vue/typescript-plugin": vuePluginVersion},
	}
}

func claudeACPPackage() npmPackage {
	return npmPackage{
		name: "claude-agent-acp", version: claudeACPVersion, main: claudeACPMain, bin: "claude-agent-acp",
		source:   fmt.Sprintf("@agentclientprotocol/claude-agent-acp %s, Claude Agent SDK %s, %d locked packages", claudeACPVersion, claudeACPSDK, claudeACPLockPackages),
		manifest: claudeACPPackageJSON, lockfile: claudeACPLockfile,
		versions:     map[string]string{"@agentclientprotocol/claude-agent-acp": claudeACPVersion, "@anthropic-ai/claude-agent-sdk": claudeACPSDK},
		omitOptional: true, minNode: adapterMinNodeMajor,
	}
}

func codexACPPackage() npmPackage {
	return npmPackage{
		name: "codex-acp", version: codexACPVersion, main: codexACPMain, bin: "codex-acp",
		source:   fmt.Sprintf("@agentclientprotocol/codex-acp %s, @openai/codex %s, %d locked packages", codexACPVersion, codexACPCodex, codexACPLockPackages),
		manifest: codexACPPackageJSON, lockfile: codexACPLockfile,
		versions:     map[string]string{"@agentclientprotocol/codex-acp": codexACPVersion, "@openai/codex": codexACPCodex},
		omitOptional: true, minNode: adapterMinNodeMajor,
	}
}

// policy is the receipt policy the package installs under.
func (pkg npmPackage) policy() string {
	if pkg.omitOptional {
		return npmOmitOptionalPolicy
	}
	return npmPolicy
}

// nodeMajor is the oldest host Node.js major the package runs on.
func (pkg npmPackage) nodeMajor() int { return max(pkg.minNode, hostNodeMinMajor) }

// installNPM runs `npm ci` of the embedded lockfile in stage with the
// selected Node.js. npm ci installs exactly the locked tree and checks every
// package against its sha512 integrity value; no package script runs. The
// environment is allowlisted: no user npmrc, cache, token or registry
// override, and HOME is a scratch directory.
func installNPM(ctx context.Context, pkg npmPackage, stage, nodeDir string, command func(context.Context, string, string, []string, ...string) ([]byte, error)) error {
	node := filepath.Join(nodeDir, "bin", "node")
	npm := filepath.Join(nodeDir, "lib", "node_modules", "npm", "bin", "npm-cli.js")
	if err := executableFile(node); err != nil {
		return err
	}
	if _, err := fileDigest(npm); err != nil {
		return err
	}
	work := filepath.Join(stage, ".work")
	if err := os.Mkdir(work, 0o700); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(work) }()
	for _, name := range []string{"home", "tmp", "cache"} {
		if err := os.Mkdir(filepath.Join(work, name), 0o700); err != nil {
			return err
		}
	}
	// npm refuses to load one file as both the user and the global config.
	userrc, globalrc := filepath.Join(work, "npmrc"), filepath.Join(work, "global-npmrc")
	for _, rc := range []string{userrc, globalrc} {
		if err := os.WriteFile(rc, nil, 0o600); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(stage, "package.json"), pkg.manifest, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "package-lock.json"), pkg.lockfile, 0o600); err != nil {
		return err
	}
	env := []string{
		"HOME=" + filepath.Join(work, "home"), "TMPDIR=" + filepath.Join(work, "tmp"),
		"PATH=" + filepath.Join(nodeDir, "bin") + ":/usr/bin:/bin",
		"npm_config_userconfig=" + userrc, "npm_config_globalconfig=" + globalrc,
		"npm_config_cache=" + filepath.Join(work, "cache"), "npm_config_registry=" + npmRegistry,
		"npm_config_ignore_scripts=true", "npm_config_update_notifier=false",
		"NODE_OPTIONS=", "NO_UPDATE_NOTIFIER=1",
	}
	args := []string{npm, "ci", "--ignore-scripts", "--no-audit", "--no-fund", "--loglevel=error"}
	if pkg.omitOptional {
		args = append(args, "--omit=optional")
	}
	_, err := command(ctx, node, stage, env, args...)
	return err
}

// validate reads package metadata only; it never runs the server.
func (pkg npmPackage) validate(dir string) error {
	main := filepath.Join(dir, filepath.FromSlash(pkg.main))
	if _, err := fileDigest(main); err != nil {
		return err
	}
	// A key is a path below node_modules, so a nested copy names its parent.
	for pkg, version := range pkg.versions {
		data, err := readBounded(filepath.Join(dir, "node_modules", filepath.FromSlash(pkg), "package.json"), 1<<20)
		if err != nil {
			return err
		}
		_, name, nested := strings.Cut(pkg, "/node_modules/")
		if !nested {
			name = pkg
		}
		var meta struct{ Name, Version string }
		if json.Unmarshal(data, &meta) != nil || meta.Name != name || meta.Version != version {
			return errors.New("unexpected package metadata")
		}
	}
	if pkg.bin == "" {
		return nil
	}
	link, err := filepath.EvalSymlinks(filepath.Join(dir, "node_modules", ".bin", pkg.bin))
	real, rerr := filepath.EvalSymlinks(main)
	if err != nil || rerr != nil || link != real {
		return errors.New(pkg.name + " command missing")
	}
	return nil
}

// validateNode checks the archive layout npm servers' install and launch rely on.
func validateNode(dir string) error {
	if err := executableFile(filepath.Join(dir, "bin", "node")); err != nil {
		return err
	}
	_, err := fileDigest(filepath.Join(dir, "lib", "node_modules", "npm", "bin", "npm-cli.js"))
	return err
}

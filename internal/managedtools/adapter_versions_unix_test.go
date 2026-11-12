//go:build darwin || linux

package managedtools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeAdapter lays out a valid install of tool at version, as the
// installer leaves it, and returns its main script.
func writeAdapter(t *testing.T, i *Installer, tool, version string) string {
	t.Helper()
	pkg := npmPackages[tool]()
	if version != pkg.version {
		pkg = otherVersion(pkg, version)
	}
	dir := filepath.Join(i.root, installDir(pkg.name, version))
	files := map[string]string{pkg.main: "adapter " + version}
	for name, v := range pkg.versions {
		files["node_modules/"+name+"/package.json"] = `{"name":"` + name + `","version":"` + v + `"}`
	}
	writeTree(t, dir, files)
	if err := os.Mkdir(filepath.Join(dir, "node_modules/.bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../"+strings.TrimPrefix(pkg.main, "node_modules/"), filepath.Join(dir, "node_modules/.bin", pkg.bin)); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("adapter " + version))
	r, _ := json.Marshal(receipt{Version: version, Policy: pkg.policy(), SHA256: hex.EncodeToString(sum[:])})
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), r, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{i.root, dir} {
		if err := os.Chmod(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, filepath.FromSlash(pkg.main))
}

func TestAdapterFindsPinnedThenOlderCopies(t *testing.T) {
	i, _ := fixture(t)
	tool := "claude-agent-acp"
	pinned := i.Pinned(tool)
	if pinned != claudeACPVersion || i.Pinned("pyright") != "" {
		t.Fatal(pinned, i.Pinned("pyright"))
	}
	if _, _, err := i.Adapter(tool); !os.IsNotExist(err) {
		t.Fatal("nothing installed:", err)
	}
	if _, _, err := i.Adapter("pyright"); err == nil || os.IsNotExist(err) {
		t.Fatal("a language server is not an adapter:", err)
	}

	// Older copies: the newest valid one wins, by number, not by name.
	writeAdapter(t, i, tool, "0.9.0")
	old := writeAdapter(t, i, tool, "0.10.0")
	if main, v, err := i.Adapter(tool); err != nil || v != "0.10.0" || main != old {
		t.Fatal(main, v, err)
	}
	// Names that are not a plain version of this platform are ignored.
	here := "-" + runtime.GOOS + "-" + runtime.GOARCH
	for _, name := range []string{tool + "-1.0-" + "x", tool + "-99.0.0-plan9-mips", tool + "-9.9.9-beta" + here, "99.0.0" + here, "codex-acp-50.0.0"} {
		if err := os.Mkdir(filepath.Join(i.root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, v, err := i.Adapter(tool); err != nil || v != "0.10.0" {
		t.Fatal(v, err)
	}
	// Each adapter finds its own copies only.
	if _, _, err := i.Adapter("codex-acp"); !os.IsNotExist(err) {
		t.Fatal("codex found claude's copies:", err)
	}
	codex := writeAdapter(t, i, "codex-acp", "2.5.0")
	if main, v, err := i.Adapter("codex-acp"); err != nil || v != "2.5.0" || main != codex {
		t.Fatal(main, v, err)
	}
	// A changed main script or a receipt for another version is skipped.
	if err := os.WriteFile(old, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, v, err := i.Adapter(tool); err != nil || v != "0.9.0" {
		t.Fatal("changed copy used:", v, err)
	}
	r := filepath.Join(i.root, installDir(tool, "0.9.0"), "receipt.json")
	if err := os.WriteFile(r, []byte(`{"Version":"0.8.0","Policy":"`+npmOmitOptionalPolicy+`","SHA256":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Adapter(tool); !os.IsNotExist(err) {
		t.Fatal("wrong receipt used:", err)
	}
	// A copy others can write to is not used.
	writeAdapter(t, i, tool, "0.11.0")
	if err := os.Chmod(filepath.Join(i.root, installDir(tool, "0.11.0")), 0o770); err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Adapter(tool); !os.IsNotExist(err) {
		t.Fatal("unsafe folder used:", err)
	}

	// The pinned copy comes first.
	main := writeAdapter(t, i, tool, pinned)
	if got, v, err := i.Adapter(tool); err != nil || v != pinned || got != main {
		t.Fatal(got, v, err)
	}
	// A damaged pinned copy is reported, not passed over.
	if err := os.WriteFile(main, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := i.Adapter(tool); err == nil || os.IsNotExist(err) {
		t.Fatal("damaged pinned copy:", err)
	}
}

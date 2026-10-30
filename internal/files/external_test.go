package files

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadExternal(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, text string) string {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	store := filepath.Join(dir, "store")
	policy := EditPolicy{ProtectedRoots: []string{store}}

	src := write("mod/fmt/print.go", "package fmt\n")
	got, err := ReadExternal(policy, src)
	if err != nil || got.Text != "package fmt\n" || got.Path != src || got.Revision == "" {
		t.Fatal(got, err)
	}
	if got.Edit != nil || got.Changes != nil {
		t.Fatal("an external file must never be editable", got)
	}

	big := write("mod/big.go", strings.Repeat("x", MaxReadBytes+1))
	if got, err := ReadExternal(policy, big); err != nil || !got.TooLarge || got.Text != "" {
		t.Fatal(got.TooLarge, err)
	}
	bin := write("mod/blob.bin", "a\x00b")
	if got, err := ReadExternal(policy, bin); err != nil || !got.Binary || got.Text != "" {
		t.Fatal(got, err)
	}
	if _, err := ReadExternal(policy, filepath.Join(dir, "mod")); err == nil {
		t.Fatal("read a directory")
	}
	if _, err := ReadExternal(policy, filepath.Join(dir, "mod/missing.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	for _, path := range []string{"mod/fmt/print.go", dir + "/mod/../mod/fmt/print.go"} {
		if _, err := ReadExternal(policy, path); !errors.Is(err, ErrExternalPath) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	for _, path := range []string{
		write(".ssh/config", "Host *\n"),
		write("home/.codex/auth.json", "{}"),
		write("pkg/.credentials.json", "{}"),
		write("store/sessions/a.jsonl", "{}"),
	} {
		if _, err := ReadExternal(policy, path); !errors.Is(err, ErrEditProtected) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	// An open directory inside a store is read; the rest of the store and
	// protected names in it are not.
	tools := filepath.Join(store, "managed-tools")
	stub := write("store/managed-tools/pyright/typeshed/builtins.pyi", "class int: ...\n")
	if got, err := ReadExternal(policy, stub, tools); err != nil || got.Text != "class int: ...\n" {
		t.Fatal(got, err)
	}
	if _, err := ReadExternal(policy, stub); !errors.Is(err, ErrEditProtected) {
		t.Fatal("read the store without opening it", err)
	}
	for _, path := range []string{
		filepath.Join(store, "sessions/a.jsonl"),
		write("store/managed-tools/pkg/.credentials.json", "{}"),
	} {
		if _, err := ReadExternal(policy, path, tools); !errors.Is(err, ErrEditProtected) {
			t.Fatalf("%s: %v", path, err)
		}
	}
	if runtime.GOOS != "windows" {
		// A link out of the open directory is refused by where it leads.
		escape := filepath.Join(tools, "escape.go")
		if err := os.Symlink(filepath.Join(store, "sessions", "a.jsonl"), escape); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadExternal(policy, escape, tools); !errors.Is(err, ErrEditProtected) {
			t.Fatal(err)
		}
		// A link that resolves into a store is refused by where it leads.
		link := filepath.Join(dir, "mod", "link.go")
		if err := os.Symlink(filepath.Join(store, "sessions", "a.jsonl"), link); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadExternal(policy, link); !errors.Is(err, ErrEditProtected) {
			t.Fatal(err)
		}
		// A link to an ordinary file reads it, under the path asked for.
		plain := filepath.Join(dir, "mod", "plain.go")
		if err := os.Symlink(src, plain); err != nil {
			t.Fatal(err)
		}
		if got, err := ReadExternal(policy, plain); err != nil || got.Path != plain || got.Text != "package fmt\n" {
			t.Fatal(got, err)
		}
	}
}

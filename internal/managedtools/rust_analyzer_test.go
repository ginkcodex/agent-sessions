//go:build darwin || linux

package managedtools

import (
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func gz(t *testing.T, parts ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, p := range parts {
		w := gzip.NewWriter(&buf)
		if _, err := w.Write([]byte(p)); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func TestRustAnalyzerCatalogPins(t *testing.T) {
	want := map[string]string{
		"darwin/arm64": "rust-analyzer-aarch64-apple-darwin.gz",
		"darwin/amd64": "rust-analyzer-x86_64-apple-darwin.gz",
		"linux/arm64":  "rust-analyzer-aarch64-unknown-linux-gnu.gz",
		"linux/amd64":  "rust-analyzer-x86_64-unknown-linux-gnu.gz",
	}
	if len(rustAnalyzerArchives) != len(want) || !Proposable("rust-analyzer") {
		t.Fatal(rustAnalyzerArchives)
	}
	for platform, name := range want {
		a := rustAnalyzerArchives[platform]
		if a.URL != "https://github.com/rust-lang/rust-analyzer/releases/download/2026-10-26/"+name || !digest(a.SHA256) {
			t.Errorf("%s: %+v", platform, a)
		}
	}
}

func TestRustAnalyzerInstall(t *testing.T) {
	i, data := fixture(t)
	archive := gz(t, "fake rust-analyzer")
	replace(t, rustAnalyzerArchives, pin(archive))
	p, err := i.Propose("rust-analyzer")
	if err != nil {
		t.Fatal(err)
	}
	a := rustAnalyzerArchives[platform()]
	if p.Tool != "rust-analyzer" || p.Version != rustAnalyzerVersion || p.Source != a.URL || p.Integrity != "sha256:"+a.SHA256 || p.Note != "" ||
		p.Destination != filepath.Join(i.root, installDir("rust-analyzer", rustAnalyzerVersion)) {
		t.Fatalf("%+v", p)
	}
	if entries, _ := os.ReadDir(data); len(entries) != 0 {
		t.Fatal("preconsent IO")
	}
	if _, err := i.Discover("rust-analyzer"); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	urls := serve(i, archive)
	path, err := i.Install(context.Background(), i.binding, p.Token)
	if err != nil || path != filepath.Join(p.Destination, "rust-analyzer") || len(*urls) != 1 || (*urls)[0] != a.URL {
		t.Fatal(path, err, *urls)
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o755 {
		t.Fatal(st, err)
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "fake rust-analyzer" {
		t.Fatal(string(body), err)
	}
	if got, err := i.Discover("rust-analyzer"); err != nil || got != path {
		t.Fatal(got, err)
	}
	if dirs := i.Dirs(); !slices.Equal(dirs, []string{p.Destination}) {
		t.Fatal(dirs)
	}
	receipt, err := os.ReadFile(filepath.Join(p.Destination, "receipt.json"))
	if err != nil || !strings.Contains(string(receipt), rustAnalyzerPolicy+":"+a.SHA256) {
		t.Fatal(string(receipt), err)
	}
	// An installed version is reused without another download.
	p, err = i.Propose("rust-analyzer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Install(context.Background(), i.binding, p.Token); err != nil || len(*urls) != 1 {
		t.Fatal(err, *urls)
	}
}

func TestIsPinnedRustAnalyzer(t *testing.T) {
	i, data := fixture(t)
	archive := gz(t, "fake rust-analyzer")
	replace(t, rustAnalyzerArchives, pin(archive))
	serve(i, archive)
	p, err := i.Propose("rust-analyzer")
	if err != nil {
		t.Fatal(err)
	}
	path, err := i.Install(context.Background(), i.binding, p.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !IsPinnedRustAnalyzer(path) {
		t.Fatal("pinned install rejected")
	}
	// Unrelated analyzers never qualify, wherever they live or however named.
	unrelated := filepath.Join(data, "bin", "rust-analyzer")
	if err := os.MkdirAll(filepath.Dir(unrelated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("fake rust-analyzer"), 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(i.root, installDir("rust-analyzer", "2020-01-01"), "rust-analyzer")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("fake rust-analyzer"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"", "rust-analyzer", unrelated, other, filepath.Join(filepath.Dir(path), "receipt.json"), filepath.Dir(path), path + "/", filepath.Dir(path) + "/../" + filepath.Base(filepath.Dir(path)) + "/rust-analyzer"} {
		if IsPinnedRustAnalyzer(c) {
			t.Errorf("accepted %q", c)
		}
	}
	// A symlink alias to the pinned binary is a different path and is refused.
	alias := filepath.Join(data, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if IsPinnedRustAnalyzer(alias) {
		t.Error("symlink alias accepted")
	}
	// A metadata check does not execute or write anything.
	before, _ := os.ReadDir(filepath.Dir(path))
	_ = IsPinnedRustAnalyzer(path)
	if after, _ := os.ReadDir(filepath.Dir(path)); len(after) != len(before) {
		t.Fatal("check wrote")
	}

	receiptPath := filepath.Join(filepath.Dir(path), "receipt.json")
	good, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	// Receipt for another version or policy, or without the matching hash.
	for name, mutate := range map[string]func() error{
		"changed binary": func() error { return os.WriteFile(path, []byte("evil"), 0o700) },
		"forged receipt": func() error {
			return os.WriteFile(receiptPath, []byte(`{"Version":"`+rustAnalyzerVersion+`","Policy":"`+rustAnalyzerPolicy+`","SHA256":"forged"}`), 0o600)
		},
		"other version": func() error {
			return os.WriteFile(receiptPath, bytes.ReplaceAll(good, []byte(rustAnalyzerVersion), []byte("2020-01-01")), 0o600)
		},
		"other policy": func() error {
			return os.WriteFile(receiptPath, bytes.ReplaceAll(good, []byte(rustAnalyzerPolicy), []byte("other-policy")), 0o600)
		},
		"missing binary":  func() error { return os.Remove(path) },
		"missing receipt": func() error { return os.Remove(receiptPath) },
		"other asset pin": func() error {
			return os.WriteFile(receiptPath, bytes.ReplaceAll(good, []byte(rustAnalyzerArchives[platform()].SHA256), []byte(strings.Repeat("0", 64))), 0o600)
		},
		"receipt link": func() error {
			_ = os.Remove(receiptPath)
			return os.Symlink(path, receiptPath)
		},
		"binary link": func() error {
			if err := os.Remove(path); err != nil {
				return err
			}
			return os.Symlink(unrelated, path)
		},
		"not executable": func() error { return os.Chmod(path, 0o600) },
		"public dir":     func() error { return os.Chmod(filepath.Dir(path), 0o755) },
		"public root":    func() error { return os.Chmod(i.root, 0o755) },
	} {
		func() {
			orig, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			restore := func() {
				_ = os.Chmod(i.root, 0o700)
				_ = os.Chmod(filepath.Dir(path), 0o700)
				_ = os.Remove(receiptPath)
				_ = os.WriteFile(receiptPath, good, 0o600)
				_ = os.Remove(path)
				_ = os.WriteFile(path, orig, 0o700)
			}
			defer restore()
			if err := mutate(); err != nil {
				t.Fatal(name, err)
			}
			if IsPinnedRustAnalyzer(path) {
				t.Errorf("%s accepted", name)
			}
		}()
	}
	if !IsPinnedRustAnalyzer(path) {
		t.Fatal("restored install rejected")
	}
}

func TestIsPinnedRustAnalyzerRejectsSymlinkedAncestors(t *testing.T) {
	i, data := fixture(t)
	archive := gz(t, "fake rust-analyzer")
	replace(t, rustAnalyzerArchives, pin(archive))
	serve(i, archive)
	p, err := i.Propose("rust-analyzer")
	if err != nil {
		t.Fatal(err)
	}
	path, err := i.Install(context.Background(), i.binding, p.Token)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(data), filepath.Base(data)+"-link")
	if err := os.Symlink(data, link); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(link) }()
	if IsPinnedRustAnalyzer(filepath.Join(link, strings.TrimPrefix(path, data))) {
		t.Fatal("symlinked ancestor accepted")
	}
}

func TestGzipFileExtractionIsBounded(t *testing.T) {
	for name, tc := range map[string]struct {
		data  []byte
		limit int64
	}{
		"empty":         {gz(t, ""), 1 << 20},
		"over limit":    {gz(t, strings.Repeat("x", 1025)), 1024},
		"multi member":  {gz(t, "fake", "more"), 1 << 20},
		"trailing data": {append(gz(t, "fake"), []byte("trailing")...), 1 << 20},
		"not gzip":      {[]byte("plain executable"), 1 << 20},
		"truncated":     {gz(t, strings.Repeat("abcdefgh", 512))[:20], 1 << 20},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "a.gz")
			if err := os.WriteFile(file, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := extractGzipFile(context.Background(), file, filepath.Join(dir, "out"), tc.limit); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "a.gz")
	if err := os.WriteFile(file, gz(t, strings.Repeat("x", 1024)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractGzipFile(context.Background(), file, filepath.Join(dir, "out"), 1024); err != nil {
		t.Fatal("exact limit refused:", err)
	}
	// An existing target is never overwritten.
	if err := extractGzipFile(context.Background(), file, filepath.Join(dir, "out"), 1024); err == nil {
		t.Fatal("overwrote an existing file")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := extractGzipFile(ctx, file, filepath.Join(dir, "canceled"), 1024); err == nil {
		t.Fatal("canceled extraction succeeded")
	}
}

func TestRustAnalyzerInstallFailures(t *testing.T) {
	for name, archive := range map[string][]byte{
		"wrong digest": gz(t, "tampered"),
		"multi member": gz(t, "fake", "more"),
		"empty":        gz(t, ""),
	} {
		t.Run(name, func(t *testing.T) {
			i, _ := fixture(t)
			good := gz(t, "fake rust-analyzer")
			pinned := pin(good)
			if name != "wrong digest" {
				pinned = pin(archive)
			}
			replace(t, rustAnalyzerArchives, pinned)
			serve(i, archive)
			p, err := i.Propose("rust-analyzer")
			if err != nil {
				t.Fatal(err)
			}
			_, err = i.Install(context.Background(), i.binding, p.Token)
			if err == nil || strings.Contains(err.Error(), "integrity") {
				t.Fatalf("error: %v", err)
			}
			if _, err := os.Stat(p.Destination); !os.IsNotExist(err) {
				t.Fatal("partial install published")
			}
			entries, _ := os.ReadDir(i.root)
			for _, e := range entries {
				if e.Name() != "install.lock" {
					t.Fatal("staging leaked", e.Name())
				}
			}
		})
	}
}

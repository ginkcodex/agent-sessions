package files

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestListDirectoryReadsOneFolder(t *testing.T) {
	root := t.TempDir()
	write(t, root, "node_modules/pkg/index.js", "x")
	write(t, root, "node_modules/pkg/lib/deep.js", "x")
	write(t, root, "node_modules/pkg/.git/HEAD", "x")
	write(t, root, "node_modules/.bin/tool", "x")
	write(t, root, "node_modules/.package-lock.json", "{}")
	if err := os.Symlink(filepath.Join(root, "node_modules", "pkg"), filepath.Join(root, "node_modules", "alias")); err != nil {
		t.Fatal(err)
	}

	got, err := ListDirectory(context.Background(), root, "node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"node_modules/.bin", "node_modules/alias", "node_modules/pkg"}; !slices.Equal(got.Directories, want) {
		t.Errorf("directories = %v, want %v", got.Directories, want)
	}
	if want := []string{"node_modules/.package-lock.json"}; !slices.Equal(got.Files, want) {
		t.Errorf("files = %v, want %v", got.Files, want)
	}
	nested, err := ListDirectory(context.Background(), root, "node_modules/pkg")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(nested.Directories, []string{"node_modules/pkg/lib"}) || !slices.Equal(nested.Files, []string{"node_modules/pkg/index.js"}) {
		t.Errorf("nested = %+v, want lib and index.js without .git", nested)
	}

	for _, rel := range []string{"", "..", "../x", "node_modules/pkg/index.js", "missing"} {
		if _, err := ListDirectory(context.Background(), root, rel); err == nil {
			t.Errorf("%q: listed", rel)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ListDirectory(context.Background(), root, "escape"); !errors.Is(err, ErrOutsideRoot) {
		t.Errorf("escape: %v", err)
	}
}

func TestListDirectoryBounded(t *testing.T) {
	root := t.TempDir()
	for i := range MaxDirectoryEntries + 3 {
		write(t, root, fmt.Sprintf("big/f%05d", i), "")
	}
	got, err := ListDirectory(context.Background(), root, "big")
	if err != nil || !got.Truncated || len(got.Files) != MaxDirectoryEntries {
		t.Fatalf("truncated=%v files=%d err=%v", got.Truncated, len(got.Files), err)
	}
}

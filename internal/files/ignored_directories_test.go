package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestIgnoredDirectoriesListsFoldersOnly(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	write(t, root, ".gitignore", "dist/\n*.log\nbuild/\n")
	git := nestedRepo(t, root, ".")
	write(t, root, "dist/app.js", "x")
	write(t, root, "web/build/out.js", "x")
	write(t, root, "web/page.ts", "x")
	write(t, root, "debug.log", "x")
	write(t, root, "src/trace.log", "x")
	write(t, root, "src/tracked.go", "package src\n")
	git("add", "src/tracked.go")

	got, ok := IgnoredDirectories(context.Background(), root)
	if !ok || !slices.Equal(got, []string{"dist", "web/build"}) {
		t.Fatalf("IgnoredDirectories = %v, %v; want [dist web/build], true", got, ok)
	}

	// From a subfolder, paths are relative to it.
	got, ok = IgnoredDirectories(context.Background(), filepath.Join(root, "web"))
	if !ok || !slices.Equal(got, []string{"build"}) {
		t.Fatalf("from web: %v, %v; want [build], true", got, ok)
	}
}

func TestIgnoredDirectoriesEmptyRepository(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	nestedRepo(t, root, ".")
	got, ok := IgnoredDirectories(context.Background(), root)
	if !ok || got == nil || len(got) != 0 {
		t.Fatalf("IgnoredDirectories = %#v, %v; want empty, true", got, ok)
	}
}

func TestIgnoredDirectoriesUnknown(t *testing.T) {
	requireGit(t)
	ctx := context.Background()

	if got, ok := IgnoredDirectories(ctx, t.TempDir()); ok {
		t.Fatalf("not a repository: %v, true; want false", got)
	}

	// A dotfiles-style outer repository that ignores everything: the project
	// folder itself is ignored, which is not an answer about its contents.
	outer := t.TempDir()
	write(t, outer, ".gitignore", "*\n!.gitignore\n")
	nestedRepo(t, outer, ".")
	write(t, outer, "proj/node_modules/x.js", "x")
	if got, ok := IgnoredDirectories(ctx, filepath.Join(outer, "proj")); ok {
		t.Fatalf("ignored root: %v, true; want false", got)
	}

	repo := t.TempDir()
	nestedRepo(t, repo, ".")
	t.Setenv("PATH", "")
	if got, ok := IgnoredDirectories(ctx, repo); ok {
		t.Fatalf("no git on PATH: %v, true; want false", got)
	}
}

func TestIgnoredDirectoriesCanceled(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	nestedRepo(t, root, ".")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, ok := IgnoredDirectories(ctx, root); ok {
		t.Fatalf("canceled: %v, true; want false", got)
	}
}

func TestIgnoredDirectoriesDoesNotRunFsmonitor(t *testing.T) {
	requireGit(t)
	if runtime.GOOS == "windows" {
		t.Skip("shell script hook")
	}
	root := t.TempDir()
	git := nestedRepo(t, root, ".")
	marker := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("config", "core.fsmonitor", hook)
	if _, ok := IgnoredDirectories(context.Background(), root); !ok {
		t.Fatal("IgnoredDirectories failed")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the repository's core.fsmonitor program ran")
	}
}

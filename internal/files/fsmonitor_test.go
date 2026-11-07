package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A repository's own core.fsmonitor program must not run when the app reads
// the repository: the file tree, Changes, diffs, blame and History.
func TestGitReadsDoNotRunRepositoryFsmonitor(t *testing.T) {
	requireGit(t)
	if runtime.GOOS == "windows" {
		t.Skip("shell script hook")
	}
	ctx := context.Background()
	root := t.TempDir()
	git := nestedRepo(t, root, ".")
	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "new.txt", "new\n")
	write(t, root, ".gitignore", "dist/\n")
	write(t, root, "dist/a.js", "a\n")
	marker := filepath.Join(t.TempDir(), "ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	git("config", "core.fsmonitor", hook)
	target := GitHistoryTarget{Dir: root, Repo: "."}

	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"List", func() error { _, err := List(ctx, root); return err }},
		{"ListingStatus", func() error {
			l, err := ListPaths(ctx, root)
			if err == nil {
				ListingStatus(ctx, l)
			}
			return err
		}},
		{"ListDirectory", func() error { _, err := ListDirectory(ctx, root, "dist"); return err }},
		{"IgnoredDirectories", func() error { IgnoredDirectories(ctx, root); return nil }},
		{"GitRepoStatus", func() error { _, err := GitRepoStatus(ctx, root); return err }},
		{"ReadGitDiff", func() error { _, err := ReadGitDiff(ctx, EditPolicy{}, root, "main.go", GitDiffIndex); return err }},
		{"BlameFile", func() error { _, err := BlameFile(ctx, root, "main.go", 1); return err }},
		{"GitHistoryRepositories", func() error { _, err := GitHistoryRepositories(ctx, root); return err }},
		{"GitHistoryBranches", func() error { _, err := GitHistoryBranches(ctx, target); return err }},
		{"GitHistoryList", func() error {
			_, err := GitHistoryList(ctx, GitHistoryListRequest{GitHistoryTarget: target, Limit: 10})
			return err
		}},
	} {
		if err := c.run(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Errorf("%s ran the repository's core.fsmonitor program", c.name)
			_ = os.Remove(marker)
		}
	}
}

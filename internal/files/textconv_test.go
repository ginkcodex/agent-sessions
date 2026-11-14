package files

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A repository's own diff.<driver>.textconv program must not run when the
// app reads it: blame, diffs and History, in the workspace's repository, in a
// nested one and through a submodule. .gitattributes is committed content,
// so a cloned repository only needs the config line to pick the program.
func TestGitReadsDoNotRunRepositoryTextconv(t *testing.T) {
	requireGit(t)
	if runtime.GOOS == "windows" {
		t.Skip("shell script textconv")
	}
	ctx := context.Background()
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	textconv := filepath.Join(t.TempDir(), "textconv.sh")
	if err := os.WriteFile(textconv, []byte("#!/bin/sh\ntouch '"+marker+"'\ncat \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A repository whose every file, gitlinks included, uses the driver.
	evilRepo := func(rel string) func(...string) {
		git := nestedRepo(t, root, rel)
		git("config", "diff.evil.textconv", textconv)
		write(t, root, rel+"/.gitattributes", "* diff=evil\n")
		write(t, root, rel+"/data.dat", "one\ntwo\n")
		git("add", ".gitattributes", "data.dat")
		git("commit", "-q", "-m", "data")
		write(t, root, rel+"/data.dat", "one\nthree\n")
		return git
	}
	git := evilRepo(".")
	evilRepo("nested")
	sub := evilRepo("sub")
	git("add", "sub") // recorded as a gitlink
	git("commit", "-q", "-m", "add sub")
	write(t, root, "sub/main.go", "package sub\n")
	sub("commit", "-q", "-am", "move sub")

	for _, c := range []struct {
		name string
		run  func() error
	}{
		{"BlameFile", func() error { _, err := BlameFile(ctx, root, "data.dat", 1); return err }},
		{"BlameFile nested", func() error { _, err := BlameFile(ctx, root, "nested/data.dat", 1); return err }},
		{"BlameFile submodule", func() error { _, err := BlameFile(ctx, root, "sub/data.dat", 1); return err }},
		{"ReadGitDiff", func() error { _, err := ReadGitDiff(ctx, EditPolicy{}, root, "data.dat", GitDiffIndex); return err }},
		{"ReadGitDiff nested", func() error {
			_, err := ReadGitDiff(ctx, EditPolicy{}, root, "nested/data.dat", GitDiffHead)
			return err
		}},
		{"ReadGitDiff submodule", func() error {
			for _, basis := range []GitDiffBasis{GitDiffIndex, GitDiffHead, GitDiffStaged} {
				got, err := ReadGitDiff(ctx, EditPolicy{}, root, "sub", basis)
				if err != nil {
					return err
				}
				if !got.Submodule {
					t.Errorf("%s: not read as a submodule: %+v", basis, got)
				}
			}
			return nil
		}},
		{"GitHistoryList", func() error {
			_, err := GitHistoryList(ctx, GitHistoryListRequest{GitHistoryTarget: GitHistoryTarget{Dir: root, Repo: "."}, Limit: 10})
			return err
		}},
	} {
		if err := c.run(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Errorf("%s ran the repository's textconv program", c.name)
			_ = os.Remove(marker)
		}
	}
}

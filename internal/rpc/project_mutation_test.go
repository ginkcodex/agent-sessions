//go:build linux || darwin

package rpc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/paths"
)

func projectGit(t *testing.T, git, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command(git, append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func waitProjectFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The real engine has no exported loader seam, so a PATH-shadowing Git wrapper
// is the barrier: the armed path enumeration runs real Git first, captures its
// pre-mutation output, then blocks until release and replays the stale bytes.
func TestRPCMutationDuringProjectFlightReconciles(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	global := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(global, []byte("[user]\n\tname = Fixture\n\temail = fixture@example.test\n[commit]\n\tgpgsign = false\n[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := filepath.Join(base, "repo")
	projectGit(t, git, base, "init", "-q", root)
	writeRPCFile(t, root, "kept.txt", "kept\n")
	projectGit(t, git, root, "add", ".")
	projectGit(t, git, root, "commit", "-q", "-m", "base")

	control := filepath.Join(base, "control")
	bin := filepath.Join(base, "bin")
	for _, dir := range []string{control, bin} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" ls-files -z --cached --others \"*)\n" +
		"  if mv \"$PROJECT_BARRIER/armed\" \"$PROJECT_BARRIER/taken\" 2>/dev/null; then\n" +
		"    \"$PROJECT_REAL_GIT\" \"$@\" > \"$PROJECT_BARRIER/out\"; code=$?\n" +
		"    : > \"$PROJECT_BARRIER/entered\"\n" +
		"    while [ ! -e \"$PROJECT_BARRIER/release\" ]; do sleep 0.01; done\n" +
		"    cat \"$PROJECT_BARRIER/out\"; exit $code\n" +
		"  fi;;\n" +
		"esac\n" +
		"exec \"$PROJECT_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROJECT_REAL_GIT", git)
	t.Setenv("PROJECT_BARRIER", control)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	release := func() { _ = os.WriteFile(filepath.Join(control, "release"), nil, 0o600) }
	t.Cleanup(release)

	client := startEditServer(t, root, paths.Roots{})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := os.WriteFile(filepath.Join(control, "armed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	type result struct {
		files []string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		listing, err := client.ListFiles(ctx, root)
		done <- result{listing.Files, err}
	}()
	waitProjectFile(t, filepath.Join(control, "entered"))
	if err := client.CreateFile(ctx, root, "", "new.go"); err != nil {
		t.Fatal(err)
	}
	release()
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if !slices.Contains(got.files, "new.go") || !slices.Contains(got.files, "kept.txt") {
		t.Fatalf("authoritative listing used pre-mutation paths: %v", got.files)
	}
	// The superseded flight must not have published its stale paths either.
	snap, err := client.ProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Paths == nil || !snap.Unverified || !slices.Contains(snap.Paths.Files, "new.go") {
		t.Fatalf("snapshot after reconciliation = %+v", snap)
	}
}

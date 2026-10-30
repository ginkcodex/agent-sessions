package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// Engine's existing Git helpers do not isolate global configuration; the files
// package's listingTree is package-private. Keep all inputs and Git config here
// generated under TempDir, including for benchmarks. Do not run these in parallel:
// Git subprocesses used by files.List inherit this test-local environment.
func projectFixtureEnvironment(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			t.Setenv(key, "")
		}
	}
	config := t.TempDir()
	t.Setenv("HOME", config)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(config, "absent"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_ATTR_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
}

func projectPut(t testing.TB, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func projectGit(t testing.TB, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func projectFixture(t testing.TB, git bool, count int) string {
	t.Helper()
	root := t.TempDir()
	if git {
		projectGit(t, root, "init", "-q", "--template=")
	}
	projectPut(t, root, ".gitignore", "ignored/\n*.skip\n")
	projectPut(t, root, "nested/.gitignore", "*.tmp\n!keep.tmp\n")
	projectPut(t, root, "nested/keep.tmp", "included\n")
	projectPut(t, root, "nested/drop.tmp", "ignored\n")
	projectPut(t, root, "ignored/deep/file", "ignored\n")
	projectPut(t, root, "deleted.txt", "delete me\n")
	for i := range count {
		projectPut(t, root, fmt.Sprintf("src%02d/f%05d.txt", i%16, i), "generated\n")
	}
	if err := os.MkdirAll(filepath.Join(root, "nested", "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	if git {
		projectGit(t, root, "add", ".")
	}
	return root
}

func projectOracle(t *testing.T, p *projectIndex, root string) files.Listing {
	t.Helper()
	want, err := files.List(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.listing(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manager differs from files.List at %s\ngot: %+v\nwant: %+v", root, got, want)
	}
	return got
}

func TestProjectIntegrationExternalReconciliation(t *testing.T) {
	projectFixtureEnvironment(t)
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			root := projectFixture(t, git, 128)
			p := newProjectIndex()
			t.Cleanup(p.stop)
			first := projectOracle(t, p, root)
			initial, generation, ok := p.snapshot(root)
			if !ok || initial.Status != nil {
				t.Fatal("missing path-only snapshot")
			}
			projectPut(t, root, "external/new.txt", "external create\n")
			projectPut(t, root, "nested/new.tmp", "ignore later\n")
			if err := os.Remove(filepath.Join(root, "deleted.txt")); err != nil {
				t.Fatal(err)
			}
			projectPut(t, root, ".gitignore", "ignored/\nexternal/\n*.skip\n")
			// External mutations do not notify the manager. Its snapshot is deliberately
			// unverified and must not be mistaken for the next authoritative listing.
			stale, staleGeneration, ok := p.snapshot(root)
			if !ok || staleGeneration != generation || !reflect.DeepEqual(stale, initial) {
				t.Fatal("snapshot unexpectedly reconciled external changes")
			}
			next := projectOracle(t, p, root)
			if reflect.DeepEqual(first, next) {
				t.Fatal("fixture failed to change listing")
			}
			if slices.Contains(next.Files, "deleted.txt") {
				t.Fatal("deleted file retained")
			}
			if git && (slices.Contains(next.Files, "external/new.txt") || next.Status["deleted.txt"] != files.Deleted) {
				t.Fatal("ignore/deletion status not reconciled")
			}
			projectOracle(t, p, filepath.Join(root, "nested"))
			fresh, _, ok := p.snapshot(root)
			next.Status = nil
			if !ok || !reflect.DeepEqual(fresh, next) {
				t.Fatal("reconciliation did not replace path snapshot")
			}
			p.invalidate(filepath.Join(root, "nested"))
			if _, _, ok := p.snapshot(root); ok {
				t.Fatal("child invalidation retained ancestor")
			}
			if _, _, ok := p.snapshot(filepath.Join(root, "nested")); ok {
				t.Fatal("child snapshot survived invalidation")
			}
			projectOracle(t, p, root)
			// Reconcile each external operation separately, without invalidation.
			projectPut(t, root, "external-visible.txt", "untracked\n")
			created := projectOracle(t, p, root)
			if !slices.Contains(created.Files, "external-visible.txt") {
				t.Fatal("external untracked creation missed")
			}
			if err := os.Remove(filepath.Join(root, "external-visible.txt")); err != nil {
				t.Fatal(err)
			}
			removed := projectOracle(t, p, root)
			if slices.Contains(removed.Files, "external-visible.txt") {
				t.Fatal("external untracked deletion missed")
			}
			projectPut(t, root, "nested/.gitignore", "*.tmp\n!new.tmp\n")
			nested := projectOracle(t, p, filepath.Join(root, "nested"))
			projectOracle(t, p, root)
			if git && (!slices.Contains(nested.Files, "new.tmp") || slices.Contains(nested.Files, "drop.tmp")) {
				t.Fatal("nested ignore change missed")
			}
		})
	}
}

func TestProjectIntegrationNestedRepository(t *testing.T) {
	projectFixtureEnvironment(t)
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("outerGit=%v", git), func(t *testing.T) {
			root := projectFixture(t, git, 16)
			nested := filepath.Join(root, "component")
			projectPut(t, root, "component/tracked.txt", "tracked\n")
			projectGit(t, nested, "init", "-q", "--template=")
			projectGit(t, nested, "add", ".")
			projectPut(t, root, "component/untracked.txt", "untracked\n")
			p := newProjectIndex()
			defer p.stop()
			projectOracle(t, p, root)
			child := projectOracle(t, p, nested)
			if !child.Git || !slices.Contains(child.Files, "untracked.txt") {
				t.Fatal("nested repository fixture not exercised")
			}
			projectPut(t, root, "component/.gitignore", "untracked.txt\n")
			projectOracle(t, p, root)
			projectOracle(t, p, nested)
		})
	}
}

func TestProjectIntegrationSnapshotStatusSeparation(t *testing.T) {
	projectFixtureEnvironment(t)
	root := projectFixture(t, true, 16)
	projectGit(t, root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
	projectPut(t, root, "deleted.txt", "modified body\n")
	p := newProjectIndex()
	defer p.stop()
	first := projectOracle(t, p, root)
	if len(first.Status) == 0 {
		t.Fatal("fixture requires authoritative status")
	}
	paths, generation, ok := p.snapshot(root)
	if !ok || paths.Status != nil {
		t.Fatal("status leaked into snapshot")
	}
	projectPut(t, root, "deleted.txt", "delete me\n") // Restore content; paths unchanged.
	stale, nextGeneration, _ := p.snapshot(root)
	if !reflect.DeepEqual(paths, stale) || generation != nextGeneration {
		t.Fatal("status-only external mutation altered snapshot")
	}
	second := projectOracle(t, p, root)
	if reflect.DeepEqual(first.Status, second.Status) {
		t.Fatal("authoritative status was reused")
	}
	second.Status = nil
	if !reflect.DeepEqual(paths, second) {
		t.Fatal("status-only change changed paths")
	}
	paths.Files[0] = "caller mutation"
	again, _, _ := p.snapshot(root)
	if again.Files[0] == "caller mutation" {
		t.Fatal("snapshot aliases caller memory")
	}
}

func TestProjectIntegrationSubdirectoryCap(t *testing.T) {
	projectFixtureEnvironment(t)
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "sub")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if git {
				projectGit(t, parent, "init", "-q", "--template=")
			}
			for i := range files.MaxFiles + 1 {
				projectPut(t, root, fmt.Sprintf("f%05d", i), "generated\n")
			}
			if git {
				projectGit(t, parent, "add", ".")
			}
			p := newProjectIndex()
			defer p.stop()
			out := projectOracle(t, p, root)
			if !out.Truncated || len(out.Files) != files.MaxFiles {
				t.Fatalf("subdirectory cap: paths=%d truncated=%v", len(out.Files), out.Truncated)
			}
			snap, _, ok := p.snapshot(root)
			if !ok || !snap.Truncated || len(snap.Files) != files.MaxFiles {
				t.Fatal("snapshot lost cap metadata")
			}
		})
	}
}

func TestProjectIntegrationConcurrentDifferentRoots(t *testing.T) {
	projectFixtureEnvironment(t)
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			roots := make([]string, projectActiveLimit)
			wants := make([]files.Listing, len(roots))
			for i := range roots {
				roots[i] = projectFixture(t, git, 128)
				projectPut(t, roots[i], fmt.Sprintf("unique%d", i), "unique\n")
				var err error
				wants[i], err = files.List(context.Background(), roots[i])
				if err != nil {
					t.Fatal(err)
				}
			}
			p := newProjectIndex()
			defer p.stop()
			var calls atomic.Int32
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			p.load = func(ctx context.Context, root string) (files.Listing, error) {
				calls.Add(1)
				select {
				case <-release:
					return files.List(ctx, root)
				case <-ctx.Done():
					return files.Listing{}, ctx.Err()
				}
			}
			type result struct {
				i   int
				out files.Listing
				err error
			}
			done := make(chan result, len(roots))
			for i, root := range roots {
				go func() { out, err := p.listing(context.Background(), root); done <- result{i, out, err} }()
			}
			for _, root := range roots {
				waitProjectWaiters(t, p, root, 1)
			}
			unblock()
			for range roots {
				r := <-done
				if r.err != nil || !reflect.DeepEqual(r.out, wants[r.i]) {
					t.Fatalf("different-root result %d: %v", r.i, r.err)
				}
			}
			if calls.Load() != int32(len(roots)) {
				t.Fatalf("different-root loads: %d", calls.Load())
			}
		})
	}
}

func TestProjectIntegrationConcurrentSameKey(t *testing.T) {
	projectFixtureEnvironment(t)
	for _, git := range []bool{false, true} {
		t.Run(fmt.Sprintf("git=%v", git), func(t *testing.T) {
			root := projectFixture(t, git, 128)
			want, err := files.List(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			p := newProjectIndex()
			defer p.stop()
			var calls atomic.Int32
			release := make(chan struct{})
			// A barrier makes overlap deterministic, but the actual loader remains the
			// real files.List. Always release it before stop, including on test failure.
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			p.load = func(ctx context.Context, root string) (files.Listing, error) {
				calls.Add(1)
				select {
				case <-release:
					return files.List(ctx, root)
				case <-ctx.Done():
					return files.Listing{}, ctx.Err()
				}
			}
			const clients = 12
			type result struct {
				listing files.Listing
				err     error
			}
			results := make(chan result, clients)
			for range clients {
				go func() { out, err := p.listing(context.Background(), root); results <- result{out, err} }()
			}
			waitProjectWaiters(t, p, root, clients)
			unblock()
			var outputs []files.Listing
			for range clients {
				got := <-results
				if got.err != nil || !reflect.DeepEqual(got.listing, want) {
					t.Fatalf("concurrent listing: %v", got.err)
				}
				outputs = append(outputs, got.listing)
			}
			if calls.Load() != 1 {
				t.Fatalf("loads = %d, want 1", calls.Load())
			}
			outputs[0].Files[0] = "mutation"
			for _, out := range outputs[1:] {
				if !reflect.DeepEqual(out, want) {
					t.Fatal("coalesced callers share mutable data")
				}
			}
		})
	}
}

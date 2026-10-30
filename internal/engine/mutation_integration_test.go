package engine

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/model"
)

func indexEpoch(p *projectIndex) uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.epoch
}

func retainStale(p *projectIndex, scopes ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, scope := range scopes {
		p.retainLocked(files.Listing{Root: scope, Files: []string{"stale"}})
	}
}

func retainedScopes(p *projectIndex, scopes ...string) []string {
	var out []string
	for _, scope := range scopes {
		if _, _, ok := p.snapshot(scope); ok {
			out = append(out, scope)
		}
	}
	return out
}

func TestSaveReplaceInvalidation(t *testing.T) {
	const body = "alpha\n"
	type op func(context.Context, *Engine, string) (string, error)
	save := func(rev string) op {
		return func(ctx context.Context, e *Engine, dir string) (string, error) {
			res, err := e.SaveFile(ctx, dir, files.SaveRequest{Path: "file.txt", Text: "beta\n", ExpectedRevision: rev})
			return string(res.Status), err
		}
	}
	replace := func(rev string) op {
		return func(ctx context.Context, e *Engine, dir string) (string, error) {
			res, err := e.ReplaceInFiles(ctx, dir, files.ReplaceRequest{
				Query: files.SearchQuery{Pattern: "alpha"}, Replacement: "beta",
				Files: []files.ReplaceFile{{Path: "file.txt", ExpectedRevision: rev}},
			})
			if len(res.Files) == 1 {
				return string(res.Files[0].Status), err
			}
			return "", err
		}
	}
	wrong := editRevision("other\n")
	for _, tc := range []struct {
		name, outcome, status string
		run                   op
	}{
		{"save", "success", "saved", save(editRevision(body))},
		{"save", "conflict", "conflict", save(wrong)},
		{"save", "canceled", "", save(editRevision(body))},
		{"save", "unauthorized", "", save(editRevision(body))},
		{"replace", "success", "replaced", replace(editRevision(body))},
		{"replace", "conflict", "conflict", replace(wrong)},
		{"replace", "canceled", "", replace(editRevision(body))},
		{"replace", "unauthorized", "", replace(editRevision(body))},
	} {
		t.Run(tc.name+"/"+tc.outcome, func(t *testing.T) {
			parent := t.TempDir()
			root, sibling := filepath.Join(parent, "root"), filepath.Join(parent, "sibling")
			file := writeEditFile(t, root, "file.txt", body)
			e := editEngine(root)
			p := e.svc.projects
			t.Cleanup(p.stop)
			scopes := []string{parent, root, filepath.Join(root, "sub"), sibling}
			retainStale(p, scopes...)
			before := indexEpoch(p)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.outcome == "canceled" {
				cancel()
			}
			dir := root
			if tc.outcome == "unauthorized" {
				dir = sibling
			}
			status, err := tc.run(ctx, e, dir)
			switch tc.outcome {
			case "unauthorized":
				if !errors.Is(err, ErrUnknownDirectory) {
					t.Fatalf("authorization: %v", err)
				}
			case "canceled":
				// Replace reports per-file failures; either form must leave bytes alone.
				if err == nil && status != string(files.ReplaceStatusFailed) {
					t.Fatalf("canceled status %q", status)
				}
			default:
				if err != nil || status != tc.status {
					t.Fatalf("status %q err %v, want %q", status, err, tc.status)
				}
			}
			wantBody, wantDelta, wantRetained := body, uint64(2), []string{sibling}
			if tc.outcome == "success" {
				wantBody = "beta\n"
			}
			if tc.outcome == "unauthorized" {
				wantDelta, wantRetained = 0, scopes
			}
			if got := readEditFile(t, file); got != wantBody {
				t.Fatalf("body %q, want %q", got, wantBody)
			}
			if got := indexEpoch(p) - before; got != wantDelta {
				t.Fatalf("generation bumps = %d, want %d", got, wantDelta)
			}
			if got := retainedScopes(p, scopes...); !slices.Equal(got, wantRetained) {
				t.Fatalf("retained %v, want %v", got, wantRetained)
			}
		})
	}
}

func integrationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Generated bare remote, primary checkout and linked worktree only; global and
// system configuration are isolated so no user hooks, credentials or remotes run.
func TestGitMutationSuccessInvalidatesAllRoots(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	global := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(global, []byte("[user]\n\tname = Fixture\n\temail = fixture@example.test\n[commit]\n\tgpgsign = false\n[init]\n\tdefaultBranch = main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	bare, primary, wt := filepath.Join(base, "remote.git"), filepath.Join(base, "primary"), filepath.Join(base, "wt")
	integrationGit(t, base, "init", "-q", "--bare", "-b", "main", bare)
	integrationGit(t, base, "clone", "-q", bare, primary)
	writeEditFile(t, primary, "sub/base.txt", "base\n")
	integrationGit(t, primary, "add", ".")
	integrationGit(t, primary, "commit", "-q", "-m", "base")
	integrationGit(t, primary, "push", "-q", "-u", "origin", "main")
	integrationGit(t, primary, "worktree", "add", "-q", "-b", "wt", wt)
	integrationGit(t, wt, "push", "-q", "-u", "origin", "wt")
	other := t.TempDir()

	e := editEngine(wt, other)
	sub := filepath.Join(wt, "sub")
	e.svc.Catalog().Reset(append(e.svc.Catalog().All(), model.SessionMeta{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "sub"}, CWD: sub, RepoRoot: wt}))
	p := e.svc.projects
	t.Cleanup(p.stop)
	ctx := context.Background()
	scopes := []string{primary, wt, sub, other}
	step := func(name string, run func() error) {
		t.Helper()
		retainStale(p, scopes...)
		before := indexEpoch(p)
		if err := run(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := indexEpoch(p) - before; got != 2 {
			t.Fatalf("%s generation bumps = %d", name, got)
		}
		if got := retainedScopes(p, scopes...); len(got) != 0 {
			t.Fatalf("%s retained %v", name, got)
		}
		for _, dir := range []string{wt, sub, other} {
			snap, err := e.ProjectSnapshot(ctx, dir)
			if err != nil || snap.Paths != nil {
				t.Fatalf("%s snapshot %s = %+v, %v", name, dir, snap, err)
			}
		}
	}
	status := func(dir, path string) files.Status {
		t.Helper()
		listing, err := e.ListFiles(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(listing.Files, "stale") {
			t.Fatalf("stale path in authoritative %s listing", dir)
		}
		return listing.Status[path]
	}

	writeEditFile(t, wt, "sub/new.txt", "new\n")
	unstaged := status(wt, "sub/new.txt")
	step("stage", func() error { return e.GitStage(ctx, wt, "", []string{"sub/new.txt"}) })
	staged := status(wt, "sub/new.txt")
	if staged == unstaged {
		t.Fatalf("stage status unchanged: %+v", staged)
	}
	step("unstage", func() error { return e.GitUnstage(ctx, wt, "", []string{"sub/new.txt"}) })
	if got := status(wt, "sub/new.txt"); got != unstaged {
		t.Fatalf("unstage status %+v, want %+v", got, unstaged)
	}
	step("commit", func() error {
		return e.GitCommit(ctx, wt, "", files.GitCommitRequest{Message: "add new", StageAll: true})
	})
	if got := status(wt, "sub/new.txt"); got != "" {
		t.Fatalf("committed status %+v", got)
	}
	step("sync-push", func() error {
		_, err := e.GitSync(ctx, wt, "", files.GitSyncRequest{Op: files.GitSyncPush})
		return err
	})
	if got := integrationGit(t, bare, "rev-parse", "wt"); got != integrationGit(t, wt, "rev-parse", "HEAD") {
		t.Fatalf("remote wt %s not pushed", got)
	}
	basePath := filepath.Join(wt, "sub", "base.txt")
	writeEditFile(t, wt, "sub/base.txt", "dirty\n")
	if got := status(wt, "sub/base.txt"); got == "" {
		t.Fatal("dirty tracked file has no status")
	}
	step("discard", func() error {
		return e.GitDiscard(ctx, wt, "", files.GitDiscardRequest{Paths: []string{"sub/base.txt"}})
	})
	if got := readEditFile(t, basePath); got != "base\n" {
		t.Fatalf("discarded body %q", got)
	}
	if got := status(wt, "sub/base.txt"); got != "" {
		t.Fatalf("discarded status %q", got)
	}
	// A peer clone advances the local bare remote; pull fast-forwards the worktree.
	peer := filepath.Join(base, "peer")
	integrationGit(t, base, "clone", "-q", "-b", "wt", bare, peer)
	writeEditFile(t, peer, "sub/pulled.txt", "pulled\n")
	integrationGit(t, peer, "add", ".")
	integrationGit(t, peer, "commit", "-q", "-m", "peer")
	integrationGit(t, peer, "push", "-q", "origin", "wt")
	step("sync-pull", func() error {
		_, err := e.GitSync(ctx, wt, "", files.GitSyncRequest{Op: files.GitSyncPull})
		return err
	})
	if listing, err := e.ListFiles(ctx, wt); err != nil || !slices.Contains(listing.Files, "sub/pulled.txt") {
		t.Fatalf("pulled listing %+v, %v", listing.Files, err)
	}
	step("sync-fetch", func() error {
		_, err := e.GitSync(ctx, wt, "", files.GitSyncRequest{Op: files.GitSyncFetch})
		return err
	})
	step("switch", func() error { return e.GitSwitch(ctx, wt, "", files.GitSwitchRequest{Name: "feature", Create: true}) })
	if got := integrationGit(t, wt, "branch", "--show-current"); got != "feature" {
		t.Fatalf("branch %q", got)
	}
	// Subdirectory sessions authorize only their own scope and still see current paths.
	if listing, err := e.ListFiles(ctx, sub); err != nil || !slices.Contains(listing.Files, "new.txt") && !slices.Contains(listing.Files, "sub/new.txt") {
		t.Fatalf("subdirectory listing %+v, %v", listing, err)
	}
}

func TestForgetRevokesProjectSnapshot(t *testing.T) {
	root := t.TempDir()
	writeEditFile(t, root, "kept.txt", "kept\n")
	e := editEngine(root)
	p := e.svc.projects
	t.Cleanup(p.stop)
	ctx := context.Background()
	if _, err := e.ListFiles(ctx, root); err != nil {
		t.Fatal(err)
	}
	snap, err := e.ProjectSnapshot(ctx, root)
	if err != nil || snap.Paths == nil || !snap.Unverified || !slices.Contains(snap.Paths.Files, "kept.txt") {
		t.Fatalf("published snapshot %+v, %v", snap, err)
	}
	before := indexEpoch(p)
	e.Forget(ctx, []model.SessionRef{e.svc.Catalog().All()[0].Ref})
	if got := indexEpoch(p) - before; got != 2 {
		t.Fatalf("forget generation bumps = %d", got)
	}
	if got := retainedScopes(p, root); len(got) != 0 {
		t.Fatalf("forget retained %v", got)
	}
	if _, err := e.ProjectSnapshot(ctx, root); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("forgotten snapshot: %v", err)
	}
	if _, err := e.ListFiles(ctx, root); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("forgotten listing: %v", err)
	}
}

// Withdrawal by catalog replacement does not invalidate, so authorization on
// every hit is the only guard for an already retained entry.
func TestCatalogWithdrawalGuardsRetainedSnapshot(t *testing.T) {
	root := t.TempDir()
	e := editEngine(root)
	p := e.svc.projects
	t.Cleanup(p.stop)
	retainStale(p, root)
	e.svc.Catalog().Reset(nil)
	if _, err := e.ProjectSnapshot(context.Background(), root); !errors.Is(err, ErrUnknownDirectory) {
		t.Fatalf("withdrawn snapshot: %v", err)
	}
	if got := retainedScopes(p, root); len(got) != 1 {
		t.Fatalf("precondition: retained %v", got)
	}
}

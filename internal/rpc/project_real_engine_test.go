package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/scan"
)

var _ engine.ProjectSnapshotBackend = (*engine.Engine)(nil)

// realSnapshotRepo is a committed Git repository with one modified file, under
// test-local Git configuration.
func realSnapshotRepo(t *testing.T) string {
	t.Helper()
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
	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command(git, append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run(base, "init", "-q", root)
	writeRPCFile(t, root, "kept.txt", "kept\n")
	writeRPCFile(t, root, "src/changed.go", "package src\n")
	run(root, "add", ".")
	run(root, "commit", "-q", "-m", "base")
	writeRPCFile(t, root, "src/changed.go", "package src\n\n// edited\n")
	return root
}

// startRealSnapshotServer serves a real engine whose catalog knows root and
// returns it too, for in-process comparison and Forget.
func startRealSnapshotServer(t *testing.T, root string) (*engine.Engine, *Client, model.SessionRef) {
	t.Helper()
	ref := model.SessionRef{Agent: model.AgentClaude, ID: "s1"}
	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{Ref: ref, CWD: root}})
	eng := engine.NewEngine(engine.WithService(engine.NewService(catalog, nil)), engine.WithCacheEnabled(false))
	t.Cleanup(func() { _ = eng.Close() })
	server, client, cleanup := setupPipePair(nil)
	t.Cleanup(cleanup)
	server.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return eng, Capabilities{SaveFile: true, CreateFile: true, CreateDirectory: true}, paths.Roots{}, nil
	})
	res, err := client.Initialize(t.Context(), InitializeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Capabilities.ProjectSnapshot || !client.Capabilities().ProjectSnapshot {
		t.Fatalf("real engine did not advertise projectSnapshot: %+v", res.Capabilities)
	}
	return eng, client, ref
}

func snapshotJSON(t *testing.T, v files.ListingSnapshot) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRealEngineProjectSnapshotRPC(t *testing.T) {
	root := realSnapshotRepo(t)
	eng, client, ref := startRealSnapshotServer(t, root)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Nothing is retained before an authoritative listing.
	if snap, err := client.ProjectSnapshot(ctx, root); err != nil || snap.Paths != nil || snap.Unverified {
		t.Fatalf("cold snapshot = %+v, %v", snap, err)
	}

	// ListFiles publishes paths; the snapshot carries them without status.
	listing, err := client.ListFiles(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !listing.Git || listing.Status["src/changed.go"] != files.Modified {
		t.Fatalf("authoritative listing = %+v", listing)
	}
	snap, err := client.ProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Paths == nil || !snap.Unverified || snap.Generation == "" || snap.Paths.Status != nil || snap.Paths.Root != root {
		t.Fatalf("published snapshot = %+v", snap)
	}
	if !slices.Equal(snap.Paths.Files, listing.Files) || !slices.Contains(snap.Paths.Files, "kept.txt") {
		t.Fatalf("snapshot paths %v, listing %v", snap.Paths.Files, listing.Files)
	}
	if raw := snapshotJSON(t, snap); strings.Contains(raw, `"status"`) {
		t.Fatalf("status leaked onto the wire: %s", raw)
	}

	// In-process parity: the engine answers the same envelope.
	local, err := eng.ProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if remote := snapshotJSON(t, snap); remote != snapshotJSON(t, local) {
		t.Fatalf("remote %s, local %s", remote, snapshotJSON(t, local))
	}

	// A create invalidates the retained paths: no stale hint survives it, and
	// the next authoritative listing publishes a newer generation.
	if err := client.CreateFile(ctx, root, "", "new.go"); err != nil {
		t.Fatal(err)
	}
	stale, err := client.ProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Paths != nil || stale.Unverified || stale.Generation != "" {
		t.Fatalf("pre-mutation paths served after create: %+v", stale)
	}
	listing, err = client.ListFiles(ctx, root)
	if err != nil || !slices.Contains(listing.Files, "new.go") {
		t.Fatalf("listing after create = %v, %v", listing.Files, err)
	}
	next, err := client.ProjectSnapshot(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if next.Paths == nil || !next.Unverified || !slices.Contains(next.Paths.Files, "new.go") || next.Generation == snap.Generation {
		t.Fatalf("snapshot after relist = %+v (was generation %q)", next, snap.Generation)
	}

	// Forget revokes the root: a retained entry never grants access.
	eng.Forget(ctx, []model.SessionRef{ref})
	gone, err := client.ProjectSnapshot(ctx, root)
	if !errors.Is(err, engine.ErrUnknownDirectory) && (err == nil || !strings.Contains(err.Error(), engine.ErrUnknownDirectory.Error())) {
		t.Fatalf("snapshot after Forget = %+v, %v", gone, err)
	}
	if gone.Paths != nil {
		t.Fatalf("forgotten root returned paths: %+v", gone)
	}
	if _, err := eng.ProjectSnapshot(ctx, root); !errors.Is(err, engine.ErrUnknownDirectory) {
		t.Fatalf("in-process snapshot after Forget = %v", err)
	}
	if _, err := client.ListFiles(ctx, root); err == nil {
		t.Fatal("ListFiles authorized a forgotten root")
	}
}

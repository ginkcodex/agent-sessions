package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"io"
	"reflect"
	"testing"
)

type gitStatusBackend struct {
	*mockBackend
	dir, repo string
}

func (b *gitStatusBackend) GitStatus(ctx context.Context, dir, repo string) (files.RepoStatus, error) {
	b.dir, b.repo = dir, repo
	return files.RepoStatus{OK: true, Branch: "main", Entries: []files.GitChange{{Path: "new", OriginalPath: "old", Index: "R", Worktree: "M"}}}, b.errToReturn
}
func TestGitStatusRPC(t *testing.T) {
	backend := &gitStatusBackend{mockBackend: &mockBackend{}}
	_, client, cleanup := setupPipePair(backend)
	defer cleanup()
	init, err := client.Initialize(context.Background(), InitializeRequest{})
	if err != nil || !init.Capabilities.GitChanges {
		t.Fatalf("init: %+v %v", init, err)
	}
	got, err := client.GitStatus(context.Background(), "/repo/sub", "")
	want, _ := backend.GitStatus(context.Background(), "/repo/sub", "")
	if err != nil || !reflect.DeepEqual(got, want) || backend.dir != "/repo/sub" {
		t.Fatalf("status: %+v %v", got, err)
	}
	backend.errToReturn = engine.ErrUnknownDirectory
	if _, err = client.GitStatus(context.Background(), "/unknown", ""); err == nil {
		t.Fatal("lost error")
	}
}
func TestGitStatusUnsupported(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	rec := &recordingWriter{}
	client := NewClient(r, rec)
	defer client.Close()
	if _, err := client.GitStatus(context.Background(), "/repo", ""); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
	if rec.Len() != 0 {
		t.Fatal("wrote unsupported request")
	}
	raw, _ := json.Marshal(Capabilities{})
	var caps map[string]any
	json.Unmarshal(raw, &caps)
	if _, ok := caps["gitChanges"]; ok {
		t.Fatal("optional capability serialized")
	}
	_, client2, cleanup := setupPipePair(&mockBackend{})
	defer cleanup()
	client2.Initialize(context.Background(), InitializeRequest{})
	if _, err := client2.GitStatus(context.Background(), "/repo", ""); err == nil {
		t.Fatal("backend without optional extension accepted")
	}
}
func TestGitChangesRepositoryRPC(t *testing.T) {
	status := &gitStatusBackend{mockBackend: &mockBackend{}}
	_, client, cleanup := setupPipePair(status)
	defer cleanup()
	init, err := client.Initialize(context.Background(), InitializeRequest{})
	if err != nil || !init.Capabilities.GitChangesRepository {
		t.Fatalf("init: %+v %v", init, err)
	}
	if _, err := client.GitStatus(context.Background(), "/repo", "child"); err != nil || status.repo != "child" {
		t.Fatalf("status: %q %v", status.repo, err)
	}
	diff := &gitDiffMock{mockBackend: &mockBackend{}}
	_, client, cleanup = setupPipePair(diff)
	defer cleanup()
	client.Initialize(context.Background(), InitializeRequest{})
	if _, err := client.ReadGitDiff(context.Background(), "/repo", "child", "child/file", files.GitDiffIndex); err != nil || diff.repo != "child" || diff.path != "child/file" {
		t.Fatalf("diff: %+v %v", diff, err)
	}
}

// A server that would ignore the repository is never asked about one: it would
// run Git in the project itself.
func TestGitChangesRepositoryOldServer(t *testing.T) {
	b := &gitStatusBackend{mockBackend: &mockBackend{}}
	_, c, cleanup := startHistory(t, b, Capabilities{GitChanges: true, GitWrite: true, GitDiff: true, GitSync: true})
	defer cleanup()
	ctx := context.Background()
	calls := map[string]func(repo string) error{
		"status":   func(repo string) error { _, err := c.GitStatus(ctx, "/repo", repo); return err },
		"branches": func(repo string) error { _, err := c.GitBranches(ctx, "/repo", repo); return err },
		"switch":   func(repo string) error { return c.GitSwitch(ctx, "/repo", repo, files.GitSwitchRequest{Name: "main"}) },
		"sync": func(repo string) error {
			_, err := c.GitSync(ctx, "/repo", repo, files.GitSyncRequest{Op: files.GitSyncFetch})
			return err
		},
		"stage":   func(repo string) error { return c.GitStage(ctx, "/repo", repo, nil) },
		"unstage": func(repo string) error { return c.GitUnstage(ctx, "/repo", repo, nil) },
		"discard": func(repo string) error { return c.GitDiscard(ctx, "/repo", repo, files.GitDiscardRequest{}) },
		"commit":  func(repo string) error { return c.GitCommit(ctx, "/repo", repo, files.GitCommitRequest{Message: "m"}) },
		"diff": func(repo string) error {
			_, err := c.ReadGitDiff(ctx, "/repo", repo, "file", files.GitDiffIndex)
			return err
		},
	}
	for name, call := range calls {
		if err := call("child"); !errors.Is(err, engine.ErrUnsupportedAction) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if b.dir != "" {
		t.Fatal("request reached the server")
	}
	if _, err := c.GitStatus(ctx, "/repo", "."); err != nil || b.dir != "/repo" || b.repo != "." {
		t.Fatalf("project's own repository: %q %v", b.repo, err)
	}
}

package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/model"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/scan"
	"reflect"
	"strings"
	"testing"
	"time"
)

type historyMock struct {
	*mockBackend
	args     []any
	block    bool
	entered  chan struct{}
	canceled chan struct{}
}

func (b *historyMock) GitStatus(context.Context, string, string) (files.RepoStatus, error) {
	return files.RepoStatus{OK: true}, nil
}

func (b *historyMock) record(ctx context.Context, p any) error {
	b.args = append(b.args, p)
	if b.block {
		close(b.entered)
		<-ctx.Done()
		close(b.canceled)
		return ctx.Err()
	}
	return b.errToReturn
}
func (b *historyMock) GitHistoryRepositories(ctx context.Context, dir string) (files.GitHistoryRepositoryList, error) {
	return files.GitHistoryRepositoryList{Repositories: []files.GitHistoryRepository{{Path: "child", Name: "child"}}}, b.record(ctx, dir)
}
func (b *historyMock) GitHistoryBranches(ctx context.Context, p files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	return files.GitHistoryRefList{Head: "tip", Refs: []files.GitHistoryRef{{Ref: "refs/heads/main", OID: "tip"}}}, b.record(ctx, p)
}
func (b *historyMock) GitHistoryList(ctx context.Context, p files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	return files.GitHistoryPage{Tip: "tip", Cursor: "next", Commits: []files.GitHistorySummary{{OID: "commit", Subject: "subject"}}}, b.record(ctx, p)
}
func (b *historyMock) GitHistoryCommit(ctx context.Context, p files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	return files.GitHistoryCommitDetail{Message: "message", Changes: []files.GitHistoryChange{{Path: "file", Status: "D"}}}, b.record(ctx, p)
}
func (b *historyMock) ReadGitHistoryDiff(ctx context.Context, p files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	return files.GitHistoryDiff{Commit: "commit", Kind: "text", Content: files.Content{Text: "text", Edit: &files.EditInfo{Reason: files.EditReadOnly}}}, b.record(ctx, p)
}
func startHistory(t *testing.T, b engine.Backend, caps Capabilities) (*Server, *Client, func()) {
	t.Helper()
	s, c, cleanup := setupPipePair(b)
	s.SetStarter(func(context.Context) (engine.Backend, Capabilities, paths.Roots, error) {
		return b, caps, paths.Roots{}, nil
	})
	if _, err := c.Initialize(context.Background(), InitializeRequest{}); err != nil {
		t.Fatal(err)
	}
	return s, c, cleanup
}
func historyRPCCalls(c *Client, ctx context.Context) []func() error {
	target := files.GitHistoryTarget{Dir: "/repo", Repo: "child"}
	commit := files.GitHistoryCommitRequest{GitHistoryTarget: target, Tip: "tip", Commit: "commit"}
	return []func() error{
		func() error { _, e := c.GitHistoryRepositories(ctx, "/repo"); return e },
		func() error { _, e := c.GitHistoryBranches(ctx, target); return e },
		func() error {
			_, e := c.GitHistoryList(ctx, files.GitHistoryListRequest{GitHistoryTarget: target, Ref: "HEAD", Query: "literal[1]", Cursor: "cursor", Limit: 7})
			return e
		},
		func() error { _, e := c.GitHistoryCommit(ctx, commit); return e },
		func() error {
			_, e := c.ReadGitHistoryDiff(ctx, files.GitHistoryDiffRequest{GitHistoryCommitRequest: commit, Path: "file"})
			return e
		},
	}
}
func TestGitHistoryRPCRoundTrip(t *testing.T) {
	b := &historyMock{mockBackend: &mockBackend{}}
	_, c, cleanup := startHistory(t, b, Capabilities{GitHistory: true})
	defer cleanup()
	if !c.caps.GitHistory || c.caps.GitChanges || c.caps.GitWrite || c.caps.GitDiff {
		t.Fatalf("not independent: %+v", c.caps)
	}
	for _, call := range historyRPCCalls(c, context.Background()) {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	target := files.GitHistoryTarget{Dir: "/repo", Repo: "child"}
	commit := files.GitHistoryCommitRequest{GitHistoryTarget: target, Tip: "tip", Commit: "commit"}
	want := []any{"/repo", target, files.GitHistoryListRequest{GitHistoryTarget: target, Ref: "HEAD", Query: "literal[1]", Cursor: "cursor", Limit: 7}, commit, files.GitHistoryDiffRequest{GitHistoryCommitRequest: commit, Path: "file"}}
	if !reflect.DeepEqual(b.args, want) {
		t.Fatalf("requests %+v", b.args)
	}
	{
		got, err := c.GitHistoryRepositories(context.Background(), "/repo")
		want := files.GitHistoryRepositoryList{Repositories: []files.GitHistoryRepository{{Path: "child", Name: "child"}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("GitHistoryRepositories: %+v %v", got, err)
		}
	}
	{
		got, err := c.GitHistoryBranches(context.Background(), target)
		want := files.GitHistoryRefList{Head: "tip", Refs: []files.GitHistoryRef{{Ref: "refs/heads/main", OID: "tip"}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("GitHistoryBranches: %+v %v", got, err)
		}
	}
	{
		got, err := c.GitHistoryList(context.Background(), files.GitHistoryListRequest{GitHistoryTarget: target})
		want := files.GitHistoryPage{Tip: "tip", Cursor: "next", Commits: []files.GitHistorySummary{{OID: "commit", Subject: "subject"}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("GitHistoryList: %+v %v", got, err)
		}
	}
	{
		got, err := c.GitHistoryCommit(context.Background(), commit)
		want := files.GitHistoryCommitDetail{Message: "message", Changes: []files.GitHistoryChange{{Path: "file", Status: "D"}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("GitHistoryCommit: %+v %v", got, err)
		}
	}
	{
		got, err := c.ReadGitHistoryDiff(context.Background(), files.GitHistoryDiffRequest{GitHistoryCommitRequest: commit, Path: "file"})
		want := files.GitHistoryDiff{Commit: "commit", Kind: "text", Content: files.Content{Text: "text", Edit: &files.EditInfo{Reason: files.EditReadOnly}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("ReadGitHistoryDiff: %+v %v", got, err)
		}
	}
	for _, sentinel := range []error{engine.ErrUnknownDirectory, files.ErrOutsideRoot, engine.ErrUnsupportedAction} {
		b.errToReturn = fmt.Errorf("bounded detail: %w", sentinel)
		for _, call := range historyRPCCalls(c, context.Background()) {
			if err := call(); !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "bounded detail") {
				t.Fatalf("lost typed error: %v", err)
			}
		}
	}
	b.errToReturn = errors.New("missing Git history object: pinned commit unavailable")
	for _, call := range historyRPCCalls(c, context.Background()) {
		if err := call(); err == nil || !strings.Contains(err.Error(), "pinned commit unavailable") {
			t.Fatalf("lost missing object: %v", err)
		}
	}
}
func TestGitHistoryCapabilities(t *testing.T) {
	// An old peer's omitted capability prevents any wire request.
	r, w := io.Pipe()
	defer w.Close()
	rec := &recordingWriter{}
	c := NewClient(r, rec)
	defer c.Close()
	for _, call := range historyRPCCalls(c, context.Background()) {
		if err := call(); !errors.Is(err, engine.ErrUnsupportedAction) {
			t.Fatal(err)
		}
	}
	if rec.Len() != 0 {
		t.Fatal("unsupported wrote request")
	}
	for _, support := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			var backend engine.Backend = &gitStatusBackend{mockBackend: &mockBackend{}}
			if support {
				backend = &historyMock{mockBackend: &mockBackend{}}
			}
			_, client, cleanup := startHistory(t, backend, Capabilities{GitHistory: enabled, GitChanges: true, GitDiff: true})
			if client.caps.GitHistory != (support && enabled) {
				t.Fatalf("normalization support=%v enabled=%v", support, enabled)
			}
			if !client.caps.GitHistory {
				client.caps.GitHistory = true
				for _, call := range historyRPCCalls(client, context.Background()) {
					if err := call(); !errors.Is(err, engine.ErrUnsupportedAction) {
						t.Fatal(err)
					}
				}
			}
			if _, err := client.GitStatus(context.Background(), "/repo", ""); err != nil {
				t.Fatalf("existing Git API: %v", err)
			}
			cleanup()
		}
	}
	raw, _ := json.Marshal(Capabilities{})
	if strings.Contains(string(raw), "gitHistory") {
		t.Fatal("off capability present")
	}
}
func TestGitHistoryMalformedParams(t *testing.T) {
	b := &historyMock{mockBackend: &mockBackend{}}
	s, _, cleanup := startHistory(t, b, Capabilities{GitHistory: true})
	defer cleanup()
	for _, method := range []string{"gitHistoryRepositories", "gitHistoryBranches", "gitHistoryList", "gitHistoryCommit", "readGitHistoryDiff"} {
		for _, raw := range []string{"", `null`, `[]`, `{}`, `{"dir":3}`, `{"dir":"/repo","repo":".","unknown":true}`, `{"dir":"/repo","repo":null}`, `{"dir":"/repo","repo":".","limit":"wrong"}`} {
			_, err := s.dispatchGitHistory(context.Background(), Request{Method: method, Params: json.RawMessage(raw)}, b)
			var rpcErr *ResponseError
			if !errors.As(err, &rpcErr) || rpcErr.Code != CodeInvalidParams {
				t.Fatalf("%s %s: %v", method, raw, err)
			}
		}
	}
	if len(b.args) != 0 {
		t.Fatal("malformed reached backend")
	}
}
func TestGitHistoryCancellation(t *testing.T) {
	for index := 0; index < 5; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			b := &historyMock{mockBackend: &mockBackend{}, block: true, entered: make(chan struct{}), canceled: make(chan struct{})}
			_, c, cleanup := startHistory(t, b, Capabilities{GitHistory: true})
			defer cleanup()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- historyRPCCalls(c, ctx)[index]() }()
			select {
			case <-b.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("not entered")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("client not canceled")
			}
			select {
			case <-b.canceled:
			case <-time.After(5 * time.Second):
				t.Fatal("server not canceled")
			}
		})
	}
}

// Exercise real file validation and immutable replies through the framed wire,
// not just the forwarding mock.
func TestGitHistoryEngineRPC(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", child}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
	}
	git("init")
	git("config", "core.autocrlf", "false")
	git("config", "commit.gpgsign", "false")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(child, "file"), []byte("immutable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "file")
	git("commit", "-m", "first")
	catalog := scan.NewCatalog()
	catalog.Reset([]model.SessionMeta{{Ref: model.SessionRef{Agent: model.AgentClaude, ID: "history"}, CWD: root}})
	backend := engine.NewEngine(engine.WithService(engine.NewService(catalog, nil)), engine.WithCacheEnabled(false))
	_, c, cleanup := startHistory(t, backend, Capabilities{GitHistory: true})
	defer cleanup()
	ctx := context.Background()
	target := files.GitHistoryTarget{Dir: root, Repo: "child"}
	repos, err := c.GitHistoryRepositories(ctx, root)
	if err != nil || len(repos.Repositories) != 1 {
		t.Fatalf("%+v %v", repos, err)
	}
	refs, err := c.GitHistoryBranches(ctx, target)
	if err != nil || refs.Head == "" {
		t.Fatalf("%+v %v", refs, err)
	}
	page, err := c.GitHistoryList(ctx, files.GitHistoryListRequest{GitHistoryTarget: target})
	if err != nil || len(page.Commits) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	req := files.GitHistoryCommitRequest{GitHistoryTarget: target, Tip: page.Tip, Commit: page.Commits[0].OID}
	detail, err := c.GitHistoryCommit(ctx, req)
	if err != nil || len(detail.Changes) != 1 {
		t.Fatalf("%+v %v", detail, err)
	}
	diff, err := c.ReadGitHistoryDiff(ctx, files.GitHistoryDiffRequest{GitHistoryCommitRequest: req, Path: detail.Changes[0].Path})
	if err != nil || diff.Content.Text != "immutable\n" || diff.Content.Edit == nil || diff.Content.Edit.Reason != files.EditReadOnly {
		t.Fatalf("%+v %v", diff, err)
	}
	req.Commit = strings.Repeat("0", len(req.Commit))
	if _, err := c.GitHistoryCommit(ctx, req); err == nil {
		t.Fatal("missing commit succeeded")
	}
	if _, err := c.GitHistoryList(ctx, files.GitHistoryListRequest{GitHistoryTarget: target, Cursor: "malformed"}); err == nil {
		t.Fatal("malformed cursor succeeded")
	}
	target.Dir = child
	if _, err := c.GitHistoryBranches(ctx, target); !errors.Is(err, engine.ErrUnknownDirectory) {
		t.Fatalf("authorization: %v", err)
	}
	req.Dir = child
	if _, err := c.ReadGitHistoryDiff(ctx, files.GitHistoryDiffRequest{GitHistoryCommitRequest: req, Path: "file"}); !errors.Is(err, engine.ErrUnknownDirectory) {
		t.Fatalf("diff authorization: %v", err)
	}
}

package app

import (
	"context"
	"errors"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/rpc"
	"reflect"
	"testing"
)

type historyRecorder struct {
	engine.Backend
	calls []string
	args  []any
	hook  func()
	err   error
}

func (b *historyRecorder) GitHistoryRepositories(_ context.Context, dir string) (files.GitHistoryRepositoryList, error) {
	b.calls = append(b.calls, "GitHistoryRepositories")
	b.args = append(b.args, dir)
	if b.hook != nil {
		b.hook()
	}
	return files.GitHistoryRepositoryList{}, b.err
}
func (b *historyRecorder) GitHistoryBranches(_ context.Context, req files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	b.calls = append(b.calls, "GitHistoryBranches")
	b.args = append(b.args, req)
	if b.hook != nil {
		b.hook()
	}
	return files.GitHistoryRefList{}, b.err
}
func (b *historyRecorder) GitHistoryList(_ context.Context, req files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	b.calls = append(b.calls, "GitHistoryList")
	b.args = append(b.args, req)
	if b.hook != nil {
		b.hook()
	}
	return files.GitHistoryPage{}, b.err
}
func (b *historyRecorder) GitHistoryCommit(_ context.Context, req files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	b.calls = append(b.calls, "GitHistoryCommit")
	b.args = append(b.args, req)
	if b.hook != nil {
		b.hook()
	}
	return files.GitHistoryCommitDetail{}, b.err
}
func (b *historyRecorder) ReadGitHistoryDiff(_ context.Context, req files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	b.calls = append(b.calls, "ReadGitHistoryDiff")
	b.args = append(b.args, req)
	if b.hook != nil {
		b.hook()
	}
	return files.GitHistoryDiff{}, b.err
}
func TestGitHistoryRouting(t *testing.T) {
	local, remote := &historyRecorder{}, &historyRecorder{}
	a := NewApp()
	a.backend = local
	a.conn = newConnection(nil)
	target := files.GitHistoryTarget{Dir: "/repo", Repo: "child"}
	list := files.GitHistoryListRequest{GitHistoryTarget: target, Ref: "refs/heads/main", Cursor: "cursor", Query: "query", Limit: 4}
	commit := files.GitHistoryCommitRequest{GitHistoryTarget: target, Tip: "tip", Commit: "commit"}
	diff := files.GitHistoryDiffRequest{GitHistoryCommitRequest: commit, Path: "literal[1]"}
	calls := []func() error{
		func() error { _, e := a.GitHistoryRepositories("/repo"); return e },
		func() error { _, e := a.GitHistoryBranches(target); return e },
		func() error { _, e := a.GitHistoryList(list); return e },
		func() error { _, e := a.GitHistoryCommit(commit); return e },
		func() error { _, e := a.ReadGitHistoryDiff(diff); return e },
	}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	a.conn.wantHost = "box"
	a.conn.localActive = false
	a.conn.client = remote
	a.conn.state = ConnectionState{Phase: ConnConnected, Host: "box", Generation: 1}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(local.calls, remote.calls) || !reflect.DeepEqual(local.args, remote.args) || len(local.calls) != 5 {
		t.Fatalf("routing: %+v %+v", local, remote)
	}
	a.conn.client = nil
	a.conn.state = ConnectionState{Phase: ConnReconnecting, Host: "box", Generation: 1}
	for _, call := range calls {
		if err := call(); !errors.Is(err, rpc.ErrDisconnected) {
			t.Fatalf("offline: %v", err)
		}
	}
	if len(local.calls) != 5 || len(remote.calls) != 5 {
		t.Fatal("offline fallback")
	}
	a.conn.client = &gitBranchesRecorder{}
	a.conn.state.Phase = ConnConnected
	for _, call := range calls {
		if err := call(); !errors.Is(err, engine.ErrUnsupportedAction) {
			t.Fatalf("unsupported: %v", err)
		}
	}
	a.conn.client = remote
	remote.err = errors.New("missing object")
	remote.hook = func() { a.conn.localActive = true }
	if err := calls[4](); !errors.Is(err, remote.err) {
		t.Fatalf("lost snapshotted backend error: %v", err)
	}
	if len(local.calls) != 5 || len(remote.calls) != 6 {
		t.Fatal("host-switch fallback")
	}
}

package app

import (
	"context"
	"errors"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"github.com/ginkcode/agent-sessions/internal/rpc"
	"testing"
	"time"
)

type historyActionRecorder struct {
	engine.Backend
	calls   int
	started chan struct{}
}

func (b *historyActionRecorder) GitHistoryRefAction(ctx context.Context, req files.GitHistoryRefActionRequest) (files.GitHistoryRefActionResult, error) {
	b.calls++
	if b.started != nil {
		close(b.started)
		<-ctx.Done()
		return files.GitHistoryRefActionResult{}, ctx.Err()
	}
	return files.GitHistoryRefActionResult{Action: req.Action}, nil
}
func (b *historyActionRecorder) GitHistoryCompare(context.Context, files.GitHistoryCompareRequest) (files.GitHistoryComparison, error) {
	b.calls++
	return files.GitHistoryComparison{}, nil
}
func (b *historyActionRecorder) ReadGitHistoryCompareDiff(context.Context, files.GitHistoryCompareDiffRequest) (files.GitHistoryCompareDiff, error) {
	b.calls++
	return files.GitHistoryCompareDiff{}, nil
}

func TestHistoryActionsCapturedHost(t *testing.T) {
	local, remote := &historyActionRecorder{}, &historyActionRecorder{}
	a := remoteApp(local, remote)
	if _, err := a.GitHistoryRefAction(files.GitHistoryRefActionRequest{}, "id", ""); !errors.Is(err, ErrSaveHostChanged) {
		t.Fatal(err)
	}
	if _, err := a.GitHistoryCompare(files.GitHistoryCompareRequest{}, ""); !errors.Is(err, ErrSaveHostChanged) {
		t.Fatal(err)
	}
	if _, err := a.ReadGitHistoryCompareDiff(files.GitHistoryCompareDiffRequest{}, ""); !errors.Is(err, ErrSaveHostChanged) {
		t.Fatal(err)
	}
	if _, err := a.GitHistoryRefAction(files.GitHistoryRefActionRequest{}, "id", "box"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GitHistoryCompare(files.GitHistoryCompareRequest{}, "box"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ReadGitHistoryCompareDiff(files.GitHistoryCompareDiffRequest{}, "box"); err != nil {
		t.Fatal(err)
	}
	if local.calls != 0 || remote.calls != 3 {
		t.Fatal("wrong routing")
	}
	a.conn.client = nil
	a.conn.state = ConnectionState{Phase: ConnReconnecting, Host: "box"}
	if _, err := a.GitHistoryRefAction(files.GitHistoryRefActionRequest{}, "id", "box"); !errors.Is(err, rpc.ErrDisconnected) {
		t.Fatal(err)
	}
	old := remoteApp(local, &struct{ engine.Backend }{})
	if _, err := old.GitHistoryRefAction(files.GitHistoryRefActionRequest{}, "id", "box"); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
	if _, err := old.GitHistoryCompare(files.GitHistoryCompareRequest{}, "box"); !errors.Is(err, engine.ErrUnsupportedAction) {
		t.Fatal(err)
	}
}
func TestHistoryActionConnectionCapabilities(t *testing.T) {
	a := NewApp()
	a.backend = &historyActionRecorder{}
	if caps := a.ConnectionState().Capabilities; !caps.GitHistoryActions || !caps.GitHistoryCompare {
		t.Fatal(caps)
	}
	a.backend = &gitBranchesRecorder{}
	if caps := a.ConnectionState().Capabilities; caps.GitHistoryActions || caps.GitHistoryCompare {
		t.Fatal(caps)
	}
	a = remoteApp(&historyActionRecorder{}, &historyActionRecorder{})
	if caps := a.ConnectionState().Capabilities; caps.GitHistoryActions || caps.GitHistoryCompare {
		t.Fatal("local capabilities leaked", caps)
	}
	a.conn.state.Capabilities = rpc.Capabilities{GitHistoryActions: true, GitHistoryCompare: true}
	if caps := a.ConnectionState().Capabilities; !caps.GitHistoryActions || !caps.GitHistoryCompare {
		t.Fatal(caps)
	}
	a.conn.client = nil
	a.conn.state = ConnectionState{Phase: ConnReconnecting, Host: "box"}
	if caps := a.ConnectionState().Capabilities; caps.GitHistoryActions || caps.GitHistoryCompare {
		t.Fatal("local capabilities leaked offline", caps)
	}
}

func TestHistoryActionCancelByID(t *testing.T) {
	remote := &historyActionRecorder{started: make(chan struct{})}
	a := remoteApp(&historyActionRecorder{}, remote)
	done := make(chan error, 1)
	go func() {
		_, err := a.GitHistoryRefAction(files.GitHistoryRefActionRequest{Action: "push"}, "action", "box")
		done <- err
	}()
	<-remote.started
	a.CancelGitSync("action")
	select {
	case err := <-done:
		var syncErr *files.GitSyncError
		if !errors.As(err, &syncErr) || syncErr.Code != files.GitSyncCanceled {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("action not canceled")
	}
}

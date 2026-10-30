package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
	"reflect"
	"testing"
	"time"
)

type historyActionsMock struct{ *historyMock }

func (b *historyActionsMock) GitHistoryRefAction(ctx context.Context, p files.GitHistoryRefActionRequest) (files.GitHistoryRefActionResult, error) {
	return files.GitHistoryRefActionResult{Action: p.Action}, b.record(ctx, p)
}
func (b *historyActionsMock) GitHistoryCompare(ctx context.Context, p files.GitHistoryCompareRequest) (files.GitHistoryComparison, error) {
	return files.GitHistoryComparison{}, b.record(ctx, p)
}
func (b *historyActionsMock) ReadGitHistoryCompareDiff(ctx context.Context, p files.GitHistoryCompareDiffRequest) (files.GitHistoryCompareDiff, error) {
	return files.GitHistoryCompareDiff{}, b.record(ctx, p)
}
func historyActionRequest() files.GitHistoryRefActionRequest {
	return files.GitHistoryRefActionRequest{GitHistoryTarget: files.GitHistoryTarget{Dir: "/original", Repo: "nested"}, Action: "push", Ref: "refs/heads/main", ExpectedOID: "oid", ExpectedHead: files.GitHistoryHeadIdentity{SymbolicRef: "refs/heads/main", OID: "head"}, DraftPaths: []string{"file"}, AcknowledgeWholeRepository: true}
}
func TestHistoryActionRoundTripAndPermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		caps    Capabilities
		support bool
		action  bool
		push    bool
		compare bool
	}{
		{"all", Capabilities{GitHistoryActions: true, GitHistoryCompare: true, GitWrite: true, SaveFile: true, GitSync: true}, true, true, true, true},
		{"no-sync", Capabilities{GitHistoryActions: true, GitHistoryCompare: true, GitWrite: true, SaveFile: true}, true, true, false, true},
		{"read-only", Capabilities{GitHistoryActions: true, GitHistoryCompare: true, GitWrite: true}, true, false, false, true},
		{"old-server", Capabilities{GitHistory: true, GitWrite: true, SaveFile: true, GitSync: true}, true, false, false, false},
		{"old-backend", Capabilities{GitHistoryActions: true, GitHistoryCompare: true, GitWrite: true, SaveFile: true, GitSync: true}, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &historyActionsMock{&historyMock{mockBackend: &mockBackend{}}}
			var backend engine.Backend = mock
			if !tc.support {
				backend = &mockBackend{}
			}
			s, c, cleanup := startHistory(t, backend, tc.caps)
			defer cleanup()
			if c.caps.GitHistoryActions != tc.action || c.caps.GitHistoryCompare != tc.compare {
				t.Fatal(c.caps)
			}
			req := historyActionRequest()
			_, err := c.GitHistoryRefAction(context.Background(), req)
			if tc.push {
				if err != nil || !reflect.DeepEqual(mock.args[0], req) {
					t.Fatal(err, mock.args)
				}
			} else if !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatal(err)
			}
			req.Action = "checkout"
			_, err = c.GitHistoryRefAction(context.Background(), req)
			if tc.action {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatal(err)
			}
			_, err = c.GitHistoryCompare(context.Background(), files.GitHistoryCompareRequest{GitHistoryTarget: req.GitHistoryTarget, Ref: req.Ref, ExpectedOID: req.ExpectedOID})
			if tc.compare {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatal(err)
			}
			diffReq := files.GitHistoryCompareDiffRequest{GitHistoryCompareRequest: files.GitHistoryCompareRequest{GitHistoryTarget: req.GitHistoryTarget, Ref: req.Ref, ExpectedOID: req.ExpectedOID}, Token: "snapshot", Path: "file"}
			_, err = c.ReadGitHistoryCompareDiff(context.Background(), diffReq)
			if tc.compare {
				if err != nil || !reflect.DeepEqual(mock.args[len(mock.args)-1], diffReq) {
					t.Fatal(err, mock.args)
				}
			} else if !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatal(err)
			}
			// A client cannot bypass the server's negotiated permissions.
			raw, _ := json.Marshal(historyActionRequest())
			_, err = s.dispatchGitHistoryRefAction(context.Background(), Request{Params: raw}, backend)
			if !tc.push && !errors.Is(err, engine.ErrUnsupportedAction) {
				t.Fatal(err)
			}
		})
	}
}
func TestHistoryActionStrictDecode(t *testing.T) {
	b := &historyActionsMock{&historyMock{mockBackend: &mockBackend{}}}
	s, _, cleanup := startHistory(t, b, Capabilities{GitHistoryActions: true, SaveFile: true, GitWrite: true, GitSync: true})
	defer cleanup()
	raw, _ := json.Marshal(historyActionRequest())
	var valid map[string]any
	_ = json.Unmarshal(raw, &valid)
	for _, field := range []string{"dir", "repo", "ref", "expectedOID", "expectedHead", "action"} {
		p := map[string]any{}
		for k, v := range valid {
			p[k] = v
		}
		delete(p, field)
		data, _ := json.Marshal(p)
		if _, err := s.dispatchGitHistoryRefAction(context.Background(), Request{Params: data}, b); err == nil {
			t.Fatal("accepted missing", field)
		}
	}
	for _, head := range []any{nil, map[string]any{"oid": "oid"}, map[string]any{"symbolicRef": ""}, map[string]any{"symbolicRef": nil, "oid": ""}} {
		p := map[string]any{}
		for k, v := range valid {
			p[k] = v
		}
		p["expectedHead"] = head
		data, _ := json.Marshal(p)
		if _, err := s.dispatchGitHistoryRefAction(context.Background(), Request{Params: data}, b); err == nil {
			t.Fatal("accepted head", head)
		}
	}
	for _, raw := range []string{`[]`, `null`, `{"extra":true}`, `{"action":42}`} {
		if _, err := s.dispatchGitHistoryRefAction(context.Background(), Request{Params: json.RawMessage(raw)}, b); err == nil {
			t.Fatal(raw)
		}
	}
	if len(b.args) != 0 {
		t.Fatal("invalid requests reached backend")
	}
}
func TestHistoryActionRPCCancellation(t *testing.T) {
	b := &historyActionsMock{&historyMock{mockBackend: &mockBackend{}, block: true, entered: make(chan struct{}), canceled: make(chan struct{})}}
	_, c, cleanup := startHistory(t, b, Capabilities{GitHistoryActions: true, SaveFile: true, GitWrite: true, GitSync: true})
	defer cleanup()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.GitHistoryRefAction(ctx, historyActionRequest()); done <- err }()
	<-b.entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client did not cancel")
	}
	select {
	case <-b.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not cancel")
	}
}

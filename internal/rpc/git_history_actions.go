package rpc

import (
	"context"
	"encoding/json"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

var _ engine.GitHistoryActionsBackend = (*Client)(nil)
var _ engine.GitHistoryCompareBackend = (*Client)(nil)

func (c *Client) GitHistoryRefAction(ctx context.Context, req files.GitHistoryRefActionRequest) (files.GitHistoryRefActionResult, error) {
	if !c.caps.GitHistoryActions || !c.caps.GitWrite || !c.caps.SaveFile || (req.Action == "push" && !c.caps.GitSync) {
		return files.GitHistoryRefActionResult{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryRefActionResult
	err := c.call(ctx, "gitHistoryRefAction", req, &out)
	return out, err
}
func (c *Client) GitHistoryCompare(ctx context.Context, req files.GitHistoryCompareRequest) (files.GitHistoryComparison, error) {
	if !c.caps.GitHistoryCompare {
		return files.GitHistoryComparison{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryComparison
	err := c.call(ctx, "gitHistoryCompare", req, &out)
	return out, err
}
func (c *Client) ReadGitHistoryCompareDiff(ctx context.Context, req files.GitHistoryCompareDiffRequest) (files.GitHistoryCompareDiff, error) {
	if !c.caps.GitHistoryCompare {
		return files.GitHistoryCompareDiff{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryCompareDiff
	err := c.call(ctx, "readGitHistoryCompareDiff", req, &out)
	return out, err
}

func historyRequired() error {
	return &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
}

func (s *Server) dispatchGitHistoryRefAction(ctx context.Context, req Request, backend engine.Backend) (any, error) {
	s.mu.Lock()
	enabled, sync := s.gitHistoryActions && s.gitWrite, s.gitSync
	s.mu.Unlock()
	b, ok := backend.(engine.GitHistoryActionsBackend)
	if !enabled || !ok {
		return nil, engine.ErrUnsupportedAction
	}
	var p files.GitHistoryRefActionRequest
	if err := decodeHistoryParams(req.Params, &p); err != nil {
		return nil, err
	}
	// Require the captured identity object, including symbolicRef (empty for detached HEAD).
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(req.Params, &fields)
	var head map[string]json.RawMessage
	_ = json.Unmarshal(fields["expectedHead"], &head)
	if p.Dir == "" || p.Repo == "" || p.Ref == "" || p.ExpectedOID == "" || head["oid"] == nil || string(head["oid"]) == "null" || head["symbolicRef"] == nil || string(head["symbolicRef"]) == "null" {
		return nil, historyRequired()
	}
	switch p.Action {
	case "checkout", "merge", "delete":
	case "push":
		if !sync {
			return nil, engine.ErrUnsupportedAction
		}
	default:
		return nil, &ResponseError{Code: CodeInvalidParams, Message: "unknown history action"}
	}
	return b.GitHistoryRefAction(ctx, p)
}

func (s *Server) dispatchGitHistoryCompare(ctx context.Context, req Request, backend engine.Backend) (any, error) {
	s.mu.Lock()
	enabled := s.gitHistoryCompare
	s.mu.Unlock()
	b, ok := backend.(engine.GitHistoryCompareBackend)
	if !enabled || !ok {
		return nil, engine.ErrUnsupportedAction
	}
	if req.Method == "gitHistoryCompare" {
		var p files.GitHistoryCompareRequest
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" || p.Repo == "" || p.Ref == "" || p.ExpectedOID == "" {
			return nil, historyRequired()
		}
		return b.GitHistoryCompare(ctx, p)
	}
	var p files.GitHistoryCompareDiffRequest
	if err := decodeHistoryParams(req.Params, &p); err != nil {
		return nil, err
	}
	if p.Dir == "" || p.Repo == "" || p.Ref == "" || p.ExpectedOID == "" || p.Token == "" || p.Path == "" {
		return nil, historyRequired()
	}
	return b.ReadGitHistoryCompareDiff(ctx, p)
}

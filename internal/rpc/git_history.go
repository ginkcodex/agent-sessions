package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

var _ engine.GitHistoryBackend = (*Client)(nil)

func (c *Client) GitHistoryRepositories(ctx context.Context, dir string) (files.GitHistoryRepositoryList, error) {
	if !c.caps.GitHistory {
		return files.GitHistoryRepositoryList{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryRepositoryList
	err := c.call(ctx, "gitHistoryRepositories", GitHistoryRepositoriesParams{Dir: dir}, &out)
	return out, err
}
func (c *Client) GitHistoryBranches(ctx context.Context, target files.GitHistoryTarget) (files.GitHistoryRefList, error) {
	if !c.caps.GitHistory {
		return files.GitHistoryRefList{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryRefList
	err := c.call(ctx, "gitHistoryBranches", target, &out)
	return out, err
}
func (c *Client) GitHistoryList(ctx context.Context, request files.GitHistoryListRequest) (files.GitHistoryPage, error) {
	if !c.caps.GitHistory {
		return files.GitHistoryPage{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryPage
	err := c.call(ctx, "gitHistoryList", request, &out)
	return out, err
}
func (c *Client) GitHistoryCommit(ctx context.Context, request files.GitHistoryCommitRequest) (files.GitHistoryCommitDetail, error) {
	if !c.caps.GitHistory {
		return files.GitHistoryCommitDetail{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryCommitDetail
	err := c.call(ctx, "gitHistoryCommit", request, &out)
	return out, err
}
func (c *Client) ReadGitHistoryDiff(ctx context.Context, request files.GitHistoryDiffRequest) (files.GitHistoryDiff, error) {
	if !c.caps.GitHistory {
		return files.GitHistoryDiff{}, engine.ErrUnsupportedAction
	}
	var out files.GitHistoryDiff
	err := c.call(ctx, "readGitHistoryDiff", request, &out)
	return out, err
}

// decodeHistoryParams rejects non-objects and unknown/mistyped fields. Semantic
// target, revision and path validation remains in files after authorization.
func decodeHistoryParams(raw json.RawMessage, out any) error {
	if !json.Valid(raw) || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' {
		return &ResponseError{Code: CodeInvalidParams, Message: "history params must be an object"}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
	}
	return nil
}
func (s *Server) dispatchGitHistory(ctx context.Context, req Request, backend engine.Backend) (any, error) {
	s.mu.Lock()
	enabled := s.gitHistory
	s.mu.Unlock()
	b, ok := backend.(engine.GitHistoryBackend)
	if !enabled || !ok {
		return nil, engine.ErrUnsupportedAction
	}
	switch req.Method {
	case "gitHistoryRepositories":
		var p GitHistoryRepositoriesParams
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
		}
		return b.GitHistoryRepositories(ctx, p.Dir)
	case "gitHistoryBranches":
		var p files.GitHistoryTarget
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" || p.Repo == "" {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
		}
		return b.GitHistoryBranches(ctx, p)
	case "gitHistoryList":
		var p files.GitHistoryListRequest
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" || p.Repo == "" {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
		}
		return b.GitHistoryList(ctx, p)
	case "gitHistoryCommit":
		var p files.GitHistoryCommitRequest
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" || p.Repo == "" || p.Tip == "" || p.Commit == "" {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
		}
		return b.GitHistoryCommit(ctx, p)
	case "readGitHistoryDiff":
		var p files.GitHistoryDiffRequest
		if err := decodeHistoryParams(req.Params, &p); err != nil {
			return nil, err
		}
		if p.Dir == "" || p.Repo == "" || p.Tip == "" || p.Commit == "" || p.Path == "" {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: "missing required history parameters"}
		}
		return b.ReadGitHistoryDiff(ctx, p)
	default:
		return nil, &ResponseError{Code: CodeMethodNotFound, Message: "unknown history method"}
	}
}

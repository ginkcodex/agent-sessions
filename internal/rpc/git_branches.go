package rpc

import (
	"context"
	"encoding/json"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

type GitSwitchParams struct {
	Dir     string                 `json:"dir"`
	Repo    string                 `json:"repo,omitempty"`
	Request files.GitSwitchRequest `json:"request"`
}

func (c *Client) GitBranches(ctx context.Context, dir, repo string) (files.GitBranchList, error) {
	if !c.caps.GitChanges {
		return files.GitBranchList{}, engine.ErrUnsupportedAction
	}
	if err := c.gitChangesRepository(repo); err != nil {
		return files.GitBranchList{}, err
	}
	var out files.GitBranchList
	err := c.call(ctx, "gitBranches", GitStatusParams{Dir: dir, Repo: repo}, &out)
	return out, err
}
func (c *Client) GitSwitch(ctx context.Context, dir, repo string, req files.GitSwitchRequest) error {
	if err := c.gitWrite(repo); err != nil {
		return err
	}
	return c.call(ctx, "gitSwitch", GitSwitchParams{Dir: dir, Repo: repo, Request: req}, nil)
}
func (s *Server) dispatchGitBranches(ctx context.Context, req Request, backend engine.Backend) (any, error) {
	b, ok := backend.(engine.GitBranchesBackend)
	if !ok {
		return nil, engine.ErrUnsupportedAction
	}
	if req.Method == "gitBranches" {
		var p GitStatusParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
		}
		return b.GitBranches(ctx, p.Dir, p.Repo)
	}
	s.mu.Lock()
	enabled := s.gitWrite
	s.mu.Unlock()
	if !enabled {
		return nil, engine.ErrUnsupportedAction
	}
	var p GitSwitchParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
	}
	return nil, b.GitSwitch(ctx, p.Dir, p.Repo, p.Request)
}

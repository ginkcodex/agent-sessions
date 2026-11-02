package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

var _ engine.WorkspaceBackend = (*Client)(nil)

type WorkspaceParams struct {
	Path string `json:"path"`
}

func (c *Client) workspaceCall(ctx context.Context, method, path string, out any) error {
	if !c.Capabilities().Workspaces {
		return fmt.Errorf("%w: update this host to use workspaces", engine.ErrUnsupportedAction)
	}
	return c.call(ctx, method, WorkspaceParams{Path: path}, out)
}
func (c *Client) ListWorkspaces(ctx context.Context) ([]string, error) {
	out := []string{}
	err := c.workspaceCall(ctx, "listWorkspaces", "", &out)
	return out, err
}
func (c *Client) OpenWorkspace(ctx context.Context, dir string) (string, error) {
	var out string
	err := c.workspaceCall(ctx, "openWorkspace", dir, &out)
	return out, err
}
func (c *Client) RemoveWorkspace(ctx context.Context, dir string) error {
	return c.workspaceCall(ctx, "removeWorkspace", dir, nil)
}
func (c *Client) BrowseWorkspace(ctx context.Context, dir string) (engine.WorkspaceBrowse, error) {
	var out engine.WorkspaceBrowse
	err := c.workspaceCall(ctx, "browseWorkspace", dir, &out)
	return out, err
}
func dispatchWorkspace(ctx context.Context, backend engine.Backend, req Request) (any, error) {
	b, ok := backend.(engine.WorkspaceBackend)
	if !ok {
		return nil, &ResponseError{Code: CodeMethodNotFound, Message: "method not found: " + req.Method}
	}
	if req.Method == "listWorkspaces" {
		return b.ListWorkspaces(ctx)
	}
	var p WorkspaceParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, &ResponseError{Code: CodeInvalidParams, Message: err.Error()}
	}
	switch req.Method {
	case "openWorkspace":
		return b.OpenWorkspace(ctx, p.Path)
	case "removeWorkspace":
		return nil, b.RemoveWorkspace(ctx, p.Path)
	default:
		return b.BrowseWorkspace(ctx, p.Path)
	}
}

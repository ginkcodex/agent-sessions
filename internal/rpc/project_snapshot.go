package rpc

import (
	"context"
	"errors"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

var _ engine.ProjectSnapshotBackend = (*Client)(nil)

// ProjectSnapshot returns an empty hint for peers without the optional method.
func (c *Client) ProjectSnapshot(ctx context.Context, dir string) (files.ListingSnapshot, error) {
	if !c.Capabilities().ProjectSnapshot {
		return files.ListingSnapshot{}, nil
	}
	var out files.ListingSnapshot
	err := c.call(ctx, "projectSnapshot", ListFilesParams{Dir: dir}, &out)
	var rpcErr *ResponseError
	if errors.As(err, &rpcErr) && rpcErr.Code == CodeMethodNotFound {
		return files.ListingSnapshot{}, nil
	}
	return out, err
}

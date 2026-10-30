package rpc

import (
	"context"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

var _ engine.ListDirectoryBackend = (*Client)(nil)

// ListDirectory reads one folder when the remote host advertises support.
func (c *Client) ListDirectory(ctx context.Context, dir, path string) (files.DirectoryEntries, error) {
	if !c.Capabilities().ListDirectory {
		return files.DirectoryEntries{}, fmt.Errorf("%w: this host's agent-sessions cannot open ignored folders; update it", engine.ErrUnsupportedAction)
	}
	var out files.DirectoryEntries
	err := c.call(ctx, "listDirectory", ReadFileParams{Dir: dir, Path: path}, &out)
	return out, err
}

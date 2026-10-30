package rpc

import (
	"context"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

func (c *Client) GitStatus(ctx context.Context, dir, repo string) (files.RepoStatus, error) {
	if !c.caps.GitChanges {
		return files.RepoStatus{}, fmt.Errorf("%w: this host's agent-sessions cannot report Git changes; update it", engine.ErrUnsupportedAction)
	}
	if err := c.gitChangesRepository(repo); err != nil {
		return files.RepoStatus{}, err
	}
	var out files.RepoStatus
	err := c.call(ctx, "gitStatus", GitStatusParams{Dir: dir, Repo: repo}, &out)
	return out, err
}

// gitChangesRepository refuses a repository inside the project on a server
// that would ignore it and run Git in the project itself.
func (c *Client) gitChangesRepository(repo string) error {
	if repo == "" || repo == "." || c.caps.GitChangesRepository {
		return nil
	}
	return fmt.Errorf("%w: this host's agent-sessions cannot show changes of a repository inside the project; update it", engine.ErrUnsupportedAction)
}

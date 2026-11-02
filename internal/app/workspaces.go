package app

import (
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/engine"
)

// ListWorkspaces lists explicitly registered roots on the selected host.
// Like other no-error list APIs it returns an empty array when the host is unavailable.
func (a *App) ListWorkspaces() []string {
	if b, ok := a.activeBackend().(engine.WorkspaceBackend); ok {
		if dirs, err := b.ListWorkspaces(a.appCtx()); err == nil && dirs != nil {
			return dirs
		}
	}
	return []string{}
}

func (a *App) OpenWorkspace(dir string) (string, error) {
	if b, ok := a.activeBackend().(engine.WorkspaceBackend); ok {
		return b.OpenWorkspace(a.appCtx(), dir)
	}
	return "", fmt.Errorf("%w: this host cannot register workspaces", engine.ErrUnsupportedAction)
}

func (a *App) RemoveWorkspace(dir string) error {
	if b, ok := a.activeBackend().(engine.WorkspaceBackend); ok {
		return b.RemoveWorkspace(a.appCtx(), dir)
	}
	return fmt.Errorf("%w: this host cannot register workspaces", engine.ErrUnsupportedAction)
}

func (a *App) BrowseWorkspace(path string) (engine.WorkspaceBrowse, error) {
	if b, ok := a.activeBackend().(engine.WorkspaceBackend); ok {
		return b.BrowseWorkspace(a.appCtx(), path)
	}
	return engine.WorkspaceBrowse{}, fmt.Errorf("%w: this host cannot browse workspaces", engine.ErrUnsupportedAction)
}

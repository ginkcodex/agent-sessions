package app

import (
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

// ListDirectory reads one folder of a session directory on the selected
// host that its listing does not look into, such as an ignored node_modules.
func (a *App) ListDirectory(dir, path string) (files.DirectoryEntries, error) {
	if backend, ok := a.activeBackend().(engine.ListDirectoryBackend); ok {
		return backend.ListDirectory(a.appCtx(), dir, path)
	}
	return files.DirectoryEntries{}, fmt.Errorf("%w: this host cannot open ignored folders", engine.ErrUnsupportedAction)
}

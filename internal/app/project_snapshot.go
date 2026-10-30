package app

import (
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/files"
)

// ProjectSnapshot optionally returns retained paths on the selected host.
// Unsupported backends return an empty envelope; ListFiles remains authoritative.
func (a *App) ProjectSnapshot(dir string) (files.ListingSnapshot, error) {
	if backend, ok := a.activeBackend().(engine.ProjectSnapshotBackend); ok {
		return backend.ProjectSnapshot(a.appCtx(), dir)
	}
	return files.ListingSnapshot{}, nil
}

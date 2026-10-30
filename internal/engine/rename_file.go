package engine

import (
	"context"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// RenameFile changes the basename of a catalog-authorized file or directory.
func (e *Engine) RenameFile(ctx context.Context, dir, path, name string) error {
	if !e.svc.knownDirectory(dir) {
		return fmt.Errorf("%w: %s", ErrUnknownDirectory, dir)
	}
	e.svc.projects.invalidate(dir)
	defer e.svc.projects.invalidate(dir)
	return files.Rename(ctx, e.editPolicy(), dir, path, name)
}

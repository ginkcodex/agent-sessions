package engine

import (
	"context"
	"fmt"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// ListDirectoryBackend optionally reads one folder of a session directory
// that its listing does not look into, such as an ignored node_modules.
type ListDirectoryBackend interface {
	ListDirectory(ctx context.Context, dir, path string) (files.DirectoryEntries, error)
}

var _ ListDirectoryBackend = (*Engine)(nil)

// ListDirectory reads one folder of a session directory.
func (e *Engine) ListDirectory(ctx context.Context, dir, path string) (files.DirectoryEntries, error) {
	return e.svc.ListDirectory(ctx, dir, path)
}

// ListDirectory authorizes the session directory, then reads one folder of it.
func (s *Service) ListDirectory(ctx context.Context, dir, path string) (files.DirectoryEntries, error) {
	if !s.knownDirectory(dir) {
		return files.DirectoryEntries{}, fmt.Errorf("%w: %s", ErrUnknownDirectory, dir)
	}
	return files.ListDirectory(ctx, dir, path)
}

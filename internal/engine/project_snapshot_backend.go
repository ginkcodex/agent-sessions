package engine

import (
	"context"

	"github.com/ginkcode/agent-sessions/internal/files"
)

// ProjectSnapshotBackend optionally exposes retained project paths. Callers must
// still use ListFiles for an authoritative listing and current status.
type ProjectSnapshotBackend interface {
	ProjectSnapshot(ctx context.Context, dir string) (files.ListingSnapshot, error)
}

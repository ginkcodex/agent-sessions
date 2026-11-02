package engine

import (
	"context"
	"fmt"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// RenameSessionBackend is optional for compatibility with older remote hosts.
// Capability means the operation exists; providers/versions can still reject it.
type RenameSessionBackend interface {
	RenameSession(context.Context, model.SessionRef, string) error
}

func (e *Engine) RenameSession(ctx context.Context, ref model.SessionRef, title string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.svc == nil || e.svc.Catalog() == nil {
		return ErrUnknownSession
	}
	if err := e.svc.ValidateRefs([]model.SessionRef{ref}); err != nil {
		return err
	}
	mgr, err := e.ensureManage()
	if err != nil {
		return err
	}
	if err := mgr.RenameSession(ctx, ref, title, e.svc.Catalog().All()); err != nil {
		return err
	}
	// Refresh through the normal scanner/index/event path, not an app-only alias.
	if err := e.Scan(ctx); err != nil {
		return fmt.Errorf("session renamed, but refresh failed: %w", err)
	}
	return nil
}

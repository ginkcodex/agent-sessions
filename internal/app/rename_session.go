package app

import (
	"github.com/ginkcode/agent-sessions/internal/engine"
	"github.com/ginkcode/agent-sessions/internal/model"
)

// RenameSession always targets the captured active host; unsupported remote
// hosts must never fall back to the local filesystem.
func (a *App) RenameSession(ref model.SessionRef, title string) error {
	backend := a.activeBackend()
	renamer, ok := backend.(engine.RenameSessionBackend)
	if !ok {
		return engine.ErrUnsupportedAction
	}
	return renamer.RenameSession(a.appCtx(), ref, title)
}

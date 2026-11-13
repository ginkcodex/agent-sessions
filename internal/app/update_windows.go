package app

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/ginkcode/agent-sessions/internal/update"
	"github.com/ginkcode/agent-sessions/internal/version"
)

func init() {
	canSelfUpdate = func() bool { return windowsInstall() != nil }
	applyUpdate = windowsApply
	prepareUpdate = windowsPrepare
}

var errNotUpdatable = errors.New("this install cannot update itself")

var windowsInstalled struct {
	once sync.Once
	win  *update.Windows
}

// windowsInstall describes the running app when it can replace itself through
// its MSI, else nil. It is worked out once: it asks Windows Installer.
func windowsInstall() *update.Windows {
	windowsInstalled.once.Do(func() {
		if !update.Newer("999999.0.0", version.Current()) {
			return // a development build
		}
		dir, err := update.DefaultDir()
		if err != nil {
			return
		}
		win, err := update.SystemWindows(version.Current(), update.NewStore(dir), update.NewChecker().FeedFor)
		if err != nil {
			log.Printf("agent-sessions: automatic updates are off: %v", err)
			return
		}
		if err := win.Eligible(); err != nil {
			log.Printf("agent-sessions: automatic updates are off: %v", err)
			return
		}
		windowsInstalled.win = win
	})
	return windowsInstalled.win
}

// windowsPrepare checks the downloaded MSI and keeps the installer of the
// running version beside it.
func windowsPrepare(ctx context.Context, r update.Ready) error {
	win := windowsInstall()
	if win == nil {
		return errNotUpdatable
	}
	return win.Prepare(ctx, r)
}

// windowsApply starts the helper that replaces the installed app after this
// process exits.
func windowsApply(r update.Ready, relaunch bool) error {
	win := windowsInstall()
	if win == nil {
		return errNotUpdatable
	}
	return update.StartApply(win, r, relaunch)
}

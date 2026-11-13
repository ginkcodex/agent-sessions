package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ginkcode/agent-sessions/internal/configfile"
	"github.com/ginkcode/agent-sessions/internal/paths"
	"github.com/ginkcode/agent-sessions/internal/update"
	"github.com/ginkcode/agent-sessions/internal/version"
)

// UpdateStateEvent carries the update.Status to the frontend whenever it
// changes.
const UpdateStateEvent = "update:state"

const updateTable = "update"

// canSelfUpdate reports whether this install can replace itself, and
// applyUpdate installs a downloaded update after the app quits. The
// platforms' installers set them (macOS, Windows); everywhere else, and in
// builds that cannot update, they stay as they are, so a newer release only
// shows its page.
var (
	canSelfUpdate = func() bool { return false }
	applyUpdate   func(r update.Ready, relaunch bool) error
)

// newUpdateManager is the process's updater, shared by every window: one
// window's check feeds all of them.
func newUpdateManager() *update.Manager {
	m := &update.Manager{
		Supported: func() bool { return applyUpdate != nil && canSelfUpdate() },
		Auto: func() bool {
			roots, err := paths.Default()
			if err != nil {
				return true
			}
			return autoUpdateAt(roots.Config)
		},
		Apply: func(r update.Ready, relaunch bool) error {
			if applyUpdate == nil {
				return errors.New("this install cannot apply updates")
			}
			return applyUpdate(r, relaunch)
		},
	}
	if dir, err := update.DefaultDir(); err == nil {
		m.Store = update.NewStore(dir)
		m.Resume(version.Current())
	}
	return m
}

// autoUpdateAt reads "Download updates automatically" from the config in
// dir. It is on unless the config says otherwise.
func autoUpdateAt(configDir string) bool {
	data, err := os.ReadFile(filepath.Join(configDir, "config.toml"))
	return err != nil || autoUpdateFrom(data)
}

func autoUpdateFrom(data []byte) bool {
	v, ok := configfile.Value(data, updateTable, "auto")
	if !ok {
		return true
	}
	on, err := strconv.ParseBool(v)
	return err != nil || on
}

// AutoUpdate reports whether new versions download in the background. It is
// on by default.
func (a *App) AutoUpdate() (bool, error) {
	path, err := a.localConfigPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read settings: %w", err)
	}
	return autoUpdateFrom(data), nil
}

// SetAutoUpdate turns background downloads of updates on or off. Turning them
// off stops one in progress; the release page is offered instead.
func (a *App) SetAutoUpdate(on bool) error {
	path, err := a.localConfigPath()
	if err != nil {
		return err
	}
	err = configfile.Update(path, func(data []byte) []byte {
		return configfile.SetValue(data, updateTable, "auto", strconv.FormatBool(on))
	})
	if err != nil {
		return fmt.Errorf("save update setting: %w", err)
	}
	if a.updater != nil {
		a.updater.SettingsChanged()
	}
	return nil
}

// UpdateState says where the update is. It is about this computer's app,
// whichever host is shown.
func (a *App) UpdateState() update.Status {
	if a.updater == nil {
		return update.Status{State: update.StateIdle}
	}
	return a.updater.Status()
}

// RestartToUpdate quits the app, asking every window about its unsaved files
// first, installs the downloaded update and starts the new version. The
// caller must first warn that restarting ends the terminal tabs. Success
// means the restart was requested, not that the windows agreed to it.
func (a *App) RestartToUpdate() error {
	if a.updater == nil {
		return errNoUpdate
	}
	if _, ok := a.updater.Pending(); !ok {
		return errNoUpdate
	}
	return a.RestartApp()
}

var errNoUpdate = errors.New("there is no downloaded update to install")

// watchUpdates forwards the updater's changes to this window's frontend, and
// returns what stops it.
func (a *App) watchUpdates() func() {
	if a.updater == nil {
		return func() {}
	}
	return a.updater.Subscribe(func(s update.Status) {
		if a.desktopEvents != nil {
			a.desktopEvents.Emit(UpdateStateEvent, s)
		}
	})
}

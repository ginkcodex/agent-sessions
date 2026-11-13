package update

import (
	"context"
	"fmt"
	"os"

	"github.com/ginkcode/agent-sessions/internal/winmsi"
	"golang.org/x/sys/windows"
)

// SystemWindows describes the running app, with the real Windows Installer
// behind it. version is the running release version.
func SystemWindows(version string, store *Store, feed func(ctx context.Context, version string) (Release, error)) (*Windows, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	local := os.Getenv("LOCALAPPDATA")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read the current user: %w", err)
	}
	return &Windows{
		Exe: exe, LocalAppData: local, SID: user.User.Sid.String(), Version: version,
		Products: winmsi.InstalledProducts,
		ReadMSI:  winmsi.ReadInfo,
		Store:    store,
		Feed:     feed,
	}, nil
}

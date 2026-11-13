package update

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/winmsi"
)

// installFolderParts is where the MSI installs, below the user's local app
// data folder (INSTALLDIR in packaging/windows/agent-sessions.wxs). The
// folder is not a choice in the installer.
var installFolderParts = []string{"Programs", "Agent Sessions"}

// Windows decides whether the running app can replace itself through its MSI,
// and prepares the two packages an update needs: the new one, and the one of
// the installed version, to put back if the new one fails to install.
type Windows struct {
	// Exe is the running executable.
	Exe string
	// LocalAppData is the user's local app data folder.
	LocalAppData string
	// SID is the current user's SID, as Windows Installer reports it.
	SID string
	// Version is the running release version.
	Version string

	// Products lists the registered Agent Sessions products.
	Products func() ([]winmsi.Product, error)
	// ReadMSI reads the identity of an MSI file.
	ReadMSI func(path string) (winmsi.Info, error)

	// Store keeps the downloads, including the rollback package.
	Store *Store
	// Feed finds a release's signed installer: the installed version's, for
	// the rollback package.
	Feed func(ctx context.Context, version string) (Release, error)
}

// InstallPath is the executable the MSI installs.
func (w *Windows) InstallPath() string {
	return filepath.Join(append([]string{w.LocalAppData}, append(installFolderParts, "agent-sessions.exe")...)...)
}

// Eligible reports why this install cannot update itself, or nil: it is when
// the running executable is the one the MSI installs, and Windows Installer
// has exactly one Agent Sessions product registered for this user, of the
// version that is running. A test build or a portable copy elsewhere fails
// the first check; a second registration or a machine-wide install fails the
// second. Both need the user to sort the install out by hand.
func (w *Windows) Eligible() error {
	if w.LocalAppData == "" {
		return errors.New("the local app data folder is unknown")
	}
	if !strings.EqualFold(filepath.Clean(w.Exe), filepath.Clean(w.InstallPath())) {
		return fmt.Errorf("%s is not the installed copy (%s)", w.Exe, w.InstallPath())
	}
	products, err := w.Products()
	if err != nil {
		return fmt.Errorf("read the installed products: %w", err)
	}
	if len(products) != 1 {
		return fmt.Errorf("Windows Installer lists %d Agent Sessions installs, expected one", len(products))
	}
	p := products[0]
	if p.Context != winmsi.UserUnmanaged || p.SID != w.SID {
		return errors.New("the installed Agent Sessions is machine-wide, managed, or another user's")
	}
	if !winmsi.ValidGUID(p.Code) {
		return fmt.Errorf("the installed product has an invalid code %q", p.Code)
	}
	installed, err := winmsi.ParseVersion(p.Version)
	if err != nil {
		return err
	}
	running, err := winmsi.ParseVersion(winmsi.ForRelease(w.Version))
	if err != nil {
		return err
	}
	if installed != running {
		return fmt.Errorf("the installed version is %s but this is %s", p.Version, w.Version)
	}
	return nil
}

// Product is the registered product, for the installer to remove.
func (w *Windows) Product() (winmsi.Product, error) {
	if err := w.Eligible(); err != nil {
		return winmsi.Product{}, err
	}
	products, err := w.Products()
	if err != nil || len(products) != 1 {
		return winmsi.Product{}, fmt.Errorf("the installed product changed: %v", err)
	}
	return products[0], nil
}

// Prepare checks the downloaded MSI r and makes the rollback package ready.
// Both must be Agent Sessions per-user packages of the versions they claim.
// An error means this version is not installed by itself.
func (w *Windows) Prepare(ctx context.Context, r Ready) error {
	if err := w.Eligible(); err != nil {
		return err
	}
	if err := w.checkMSI(r, r.Version); err != nil {
		return fmt.Errorf("the new installer: %w", err)
	}
	installed, _ := winmsi.ParseVersion(winmsi.ForRelease(w.Version))
	incoming, _ := winmsi.ParseVersion(winmsi.ForRelease(r.Version))
	if installed.NewerThan(incoming) {
		return fmt.Errorf("version %s is older than the installed %s", r.Version, w.Version)
	}

	rollback, err := w.rollback(ctx, r.Version)
	if err != nil {
		return fmt.Errorf("the rollback installer: %w", err)
	}
	if err := w.checkMSI(rollback, w.Version); err != nil {
		return fmt.Errorf("the rollback installer: %w", err)
	}
	return nil
}

// Rollback returns the installer of the running version that Prepare kept
// beside r.
func (w *Windows) Rollback(r Ready) (Ready, bool) {
	rb, ok := ReadReady(w.Store.RollbackDir(r.Version))
	if !ok || rb.Version != strings.TrimPrefix(w.Version, "v") {
		return Ready{}, false
	}
	return rb, true
}

func (w *Windows) rollback(ctx context.Context, newVersion string) (Ready, error) {
	// Already downloaded and checked by an earlier run, which works offline.
	if rb, ok := w.Rollback(Ready{Version: newVersion}); ok && rb.Verify() == nil {
		return rb, nil
	}
	rel, err := w.Feed(ctx, w.Version)
	if err != nil {
		return Ready{}, err
	}
	return w.Store.FetchRollback(ctx, newVersion, rel)
}

func (w *Windows) checkMSI(r Ready, version string) error {
	info, err := w.ReadMSI(r.Path())
	if err != nil {
		return err
	}
	return info.Check(winmsi.ForRelease(version))
}

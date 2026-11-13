package update

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/winmsi"
)

// MSIApply installs a downloaded MSI over the installed one after the app has
// quit. Windows Installer cannot schedule an in-place major upgrade for our
// package, so the installed product is removed first and the new one
// installed second, as the setup wrapper does. If the second step fails, the
// installed version's MSI is installed again.
type MSIApply struct {
	Win *Windows
	// New and Rollback are the installers Windows.Prepare checked and kept.
	New, Rollback Ready
	// Relaunch starts the installed app at the end.
	Relaunch bool

	// WaitIdle returns when the app has quit and nothing runs from the
	// install folder any more, or an error if that did not happen in time.
	WaitIdle func(ctx context.Context) error
	// Msiexec runs msiexec with args and returns its exit status.
	Msiexec func(ctx context.Context, args []string) (uint32, error)
	// LogPath is where msiexec writes the verbose log called name.
	LogPath func(name string) string
	// Launch starts the app at exe.
	Launch func(exe string) error
	// Lock takes the updates folder's lock. Nil means none.
	Lock func(ctx context.Context) (unlock func(), err error)
	// Log records what happened. Nil means nothing.
	Log func(format string, args ...any)
}

func (a *MSIApply) logf(format string, args ...any) {
	if a.Log != nil {
		a.Log(format, args...)
	}
}

// Run installs the new version. The result is nil once it is installed.
// ErrStillRunning means nothing was changed because the app was still in use.
// Any other error means the update failed; the installed app is then the
// previous version put back, unless the error says otherwise.
func (a *MSIApply) Run(ctx context.Context) error {
	if err := a.WaitIdle(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrStillRunning, err)
	}
	// The app has quit. Until the removal starts, a failure leaves the
	// installed version as it was, and a restart starts it again (stopped).
	if a.Lock != nil {
		unlock, err := a.Lock(ctx)
		if err != nil {
			return a.stopped(fmt.Errorf("lock the updates folder: %w", err))
		}
		defer unlock()
	}

	// The installers sat in a folder the user can write to: check them again.
	newInfo, err := a.check(a.New, a.New.Version)
	if err != nil {
		return a.stopped(fmt.Errorf("the new installer: %w", err))
	}
	rbInfo, err := a.check(a.Rollback, a.Rollback.Version)
	if err != nil {
		return a.stopped(fmt.Errorf("the rollback installer: %w", err))
	}
	product, err := a.Win.Product()
	if err != nil {
		return a.stopped(err)
	}

	a.logf("removing %s %s", product.Code, product.Version)
	status, err := a.Msiexec(ctx, winmsi.InstallerArgs("/x", product.Code, a.LogPath("uninstall.log")))
	switch {
	case err != nil:
		return a.recover(ctx, rbInfo, fmt.Errorf("remove the installed version: %w", err))
	case status == 3010 || status == 1641:
		return a.recover(ctx, rbInfo, fmt.Errorf("removing the installed version needs a restart of Windows (status %d)", status))
	case status != 0 && status != 1605 && status != 1614:
		return a.recover(ctx, rbInfo, fmt.Errorf("remove the installed version: Windows Installer status %d", status))
	}
	if products, err := a.Win.Products(); err == nil {
		for _, p := range products {
			if strings.EqualFold(p.Code, product.Code) {
				return a.recover(ctx, rbInfo, errors.New("the installed version is still registered after its removal"))
			}
		}
	}

	a.logf("installing %s", a.New.Version)
	status, err = a.Msiexec(ctx, winmsi.InstallerArgs("/i", a.New.Path(), a.LogPath("install.log")))
	switch {
	case err != nil:
		return a.recover(ctx, rbInfo, fmt.Errorf("install the new version: %w", err))
	case status != 0 && status != 3010 && status != 1641:
		return a.recover(ctx, rbInfo, fmt.Errorf("install the new version: Windows Installer status %d", status))
	}
	if err := a.registered(newInfo); err != nil {
		return a.recover(ctx, rbInfo, fmt.Errorf("the new version did not register: %w", err))
	}
	a.logf("installed %s", a.New.Version)
	a.start()
	return nil
}

// check hashes an installer and reads its identity.
func (a *MSIApply) check(r Ready, version string) (winmsi.Info, error) {
	if err := r.Verify(); err != nil {
		return winmsi.Info{}, err
	}
	info, err := a.Win.ReadMSI(r.Path())
	if err != nil {
		return winmsi.Info{}, err
	}
	return info, info.Check(winmsi.ForRelease(version))
}

// registered checks that Windows Installer lists exactly the product that
// info describes, for this user.
func (a *MSIApply) registered(info winmsi.Info) error {
	products, err := a.Win.Products()
	if err != nil {
		return err
	}
	if len(products) != 1 {
		return fmt.Errorf("Windows Installer lists %d installs", len(products))
	}
	p := products[0]
	if !strings.EqualFold(p.Code, info.ProductCode) || p.Context != winmsi.UserUnmanaged || p.SID != a.Win.SID {
		return fmt.Errorf("the registered product %s is not the one installed", p.Code)
	}
	if p.Version != info.ProductVersion {
		return fmt.Errorf("the registered version is %s, expected %s", p.Version, info.ProductVersion)
	}
	return nil
}

func (a *MSIApply) start() {
	if !a.Relaunch {
		return
	}
	if err := a.Launch(a.Win.InstallPath()); err != nil {
		a.logf("start the app: %v", err)
	}
}

// stopped, in a restart, starts the installed version that a failed update
// did not touch, then returns cause.
func (a *MSIApply) stopped(cause error) error {
	a.start()
	return cause
}

// recover makes sure an app is installed after cause stopped the update. If
// the installed version is still registered nothing was removed and it is
// opened. If not, the rollback installer puts the previous version back; and
// if that fails too, the error names the kept installers.
func (a *MSIApply) recover(ctx context.Context, rb winmsi.Info, cause error) error {
	a.logf("%v", cause)
	if products, err := a.Win.Products(); err == nil && len(products) > 0 {
		a.start()
		return cause
	}
	a.logf("installing the previous version %s again", a.Rollback.Version)
	status, err := a.Msiexec(ctx, winmsi.InstallerArgs("/i", a.Rollback.Path(), a.LogPath("rollback.log")))
	if err == nil && status != 0 && status != 3010 && status != 1641 {
		err = fmt.Errorf("Windows Installer status %d", status)
	}
	if err == nil {
		err = a.registered(rb)
	}
	if err != nil {
		a.logf("could not put the previous version back: %v", err)
		return errors.Join(cause, fmt.Errorf("Agent Sessions is not installed. To install it by hand, run %s (this version) or %s (the previous one)", a.New.Path(), a.Rollback.Path()))
	}
	a.logf("the previous version is installed again")
	a.start()
	return cause
}

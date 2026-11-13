package update

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/winmsi"
)

const (
	codeOld = "{AAAAAAAA-0000-0000-0000-000000000001}"
	codeNew = "{BBBBBBBB-0000-0000-0000-000000000002}"
)

// msiEnv is a fake Windows Installer: msiexec edits the product list.
type msiEnv struct {
	*winEnv
	Apply    *MSIApply
	Rollback Ready
	Calls    []string // "x CODE", "i FILE"
	Launched []string
	Logs     []string

	uninstallStatus uint32
	installStatus   uint32 // for the new MSI
	rollbackStatus  uint32
	// installRegisters: what a successful install of the new MSI leaves.
	newRegisters bool
	waitErr      error
}

func newMSIEnv(t *testing.T, relaunch bool) *msiEnv {
	t.Helper()
	e := &msiEnv{winEnv: newWinEnv(t), newRegisters: true}
	// The new MSI has its own product code; the old one is the rollback's.
	oldInfo, newInfo := msiInfo("1.5.5"), msiInfo("1.6.0")
	oldInfo.ProductCode, newInfo.ProductCode = codeOld, codeNew
	e.Infos[winOldFile], e.Infos[winNewFile] = oldInfo, newInfo

	if err := e.W.Prepare(context.Background(), e.New); err != nil {
		t.Fatal(err)
	}
	rb, ok := e.W.Rollback(e.New)
	if !ok {
		t.Fatal("no rollback installer")
	}
	e.Rollback = rb
	e.Apply = &MSIApply{
		Win: e.W, New: e.New, Rollback: rb, Relaunch: relaunch,
		WaitIdle: func(context.Context) error { return e.waitErr },
		LogPath:  func(name string) string { return filepath.Join("logs", name) },
		Launch:   func(exe string) error { e.Launched = append(e.Launched, exe); return nil },
		Log:      func(format string, args ...any) { e.Logs = append(e.Logs, format) },
		Msiexec:  e.msiexec,
	}
	return e
}

func (e *msiEnv) msiexec(_ context.Context, args []string) (uint32, error) {
	op, target := args[0], args[1]
	switch op {
	case "/x":
		e.Calls = append(e.Calls, "x "+target)
		if e.uninstallStatus != 0 && e.uninstallStatus != 1605 && e.uninstallStatus != 1614 {
			return e.uninstallStatus, nil // a failed removal changes nothing
		}
		e.Products = nil
		return e.uninstallStatus, nil
	case "/i":
		file := filepath.Base(target)
		e.Calls = append(e.Calls, "i "+file)
		switch file {
		case winNewFile:
			if e.installStatus != 0 && e.installStatus != 3010 {
				return e.installStatus, nil // MSI rolls its own transaction back
			}
			if e.newRegisters {
				e.Products = []winmsi.Product{{Code: codeNew, Version: "1.6.0", SID: winUser, Context: winmsi.UserUnmanaged}}
			}
			return e.installStatus, nil
		case winOldFile:
			if e.rollbackStatus != 0 {
				return e.rollbackStatus, nil
			}
			e.Products = []winmsi.Product{{Code: codeOld, Version: "1.5.5", SID: winUser, Context: winmsi.UserUnmanaged}}
			return 0, nil
		}
	}
	return 0, errors.New("unexpected msiexec " + strings.Join(args, " "))
}

func (e *msiEnv) installed() string {
	if len(e.Products) != 1 {
		return ""
	}
	return e.Products[0].Version
}

func TestMSIApplyRemovesThenInstallsTheNewVersion(t *testing.T) {
	e := newMSIEnv(t, true)
	if err := e.Apply.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.Calls, ","); got != "x "+codeOld+",i "+winNewFile {
		t.Fatalf("msiexec calls = %s", got)
	}
	if e.installed() != "1.6.0" {
		t.Fatalf("installed %v", e.Products)
	}
	if len(e.Launched) != 1 || e.Launched[0] != e.W.InstallPath() {
		t.Fatalf("started %v", e.Launched)
	}
}

func TestMSIApplyOnQuitDoesNotStartTheApp(t *testing.T) {
	e := newMSIEnv(t, false)
	if err := e.Apply.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.installed() != "1.6.0" || len(e.Launched) != 0 {
		t.Fatalf("installed %v, started %v", e.Products, e.Launched)
	}
}

func TestMSIApplyAcceptsARestartRequestAfterTheInstall(t *testing.T) {
	e := newMSIEnv(t, true)
	e.installStatus = 3010
	if err := e.Apply.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.installed() != "1.6.0" {
		t.Fatalf("installed %v", e.Products)
	}
}

func TestMSIApplyInstallFailureInstallsThePreviousVersionAgain(t *testing.T) {
	for _, status := range []uint32{1603, 1618, 1602} {
		e := newMSIEnv(t, true)
		e.installStatus = status
		err := e.Apply.Run(context.Background())
		if err == nil || errors.Is(err, ErrStillRunning) {
			t.Fatalf("status %d: Run = %v", status, err)
		}
		if got := strings.Join(e.Calls, ","); got != "x "+codeOld+",i "+winNewFile+",i "+winOldFile {
			t.Fatalf("status %d: msiexec calls = %s", status, got)
		}
		if e.installed() != "1.5.5" {
			t.Fatalf("status %d: installed %v, want the previous version", status, e.Products)
		}
		// A restart still opens an app: the previous one.
		if len(e.Launched) != 1 {
			t.Fatalf("status %d: started %v", status, e.Launched)
		}
	}
}

func TestMSIApplyNewVersionThatDoesNotRegisterIsRolledBack(t *testing.T) {
	e := newMSIEnv(t, true)
	e.newRegisters = false
	if err := e.Apply.Run(context.Background()); err == nil {
		t.Fatal("Run = nil")
	}
	if e.installed() != "1.5.5" {
		t.Fatalf("installed %v", e.Products)
	}
}

func TestMSIApplyRollbackFailureKeepsBothInstallersAndNamesThem(t *testing.T) {
	e := newMSIEnv(t, true)
	e.installStatus = 1603
	e.rollbackStatus = 1603
	err := e.Apply.Run(context.Background())
	if err == nil {
		t.Fatal("Run = nil")
	}
	for _, want := range []string{e.New.Path(), e.Rollback.Path(), "not installed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
	if len(e.Launched) != 0 {
		t.Fatalf("started %v with nothing installed", e.Launched)
	}
	if e.Apply.New.Verify() != nil || e.Apply.Rollback.Verify() != nil {
		t.Fatal("an installer was lost")
	}
}

func TestMSIApplyRemovalFailureLeavesTheInstalledVersion(t *testing.T) {
	for name, status := range map[string]uint32{"in use": 1618, "failed": 1603, "restart needed": 3010, "restart started": 1641} {
		t.Run(name, func(t *testing.T) {
			e := newMSIEnv(t, true)
			e.uninstallStatus = status
			if err := e.Apply.Run(context.Background()); err == nil {
				t.Fatal("Run = nil")
			}
			if got := strings.Join(e.Calls, ","); got != "x "+codeOld {
				t.Fatalf("msiexec calls = %s; nothing may be installed after a failed removal", got)
			}
			if e.installed() != "1.5.5" || len(e.Launched) != 1 {
				t.Fatalf("installed %v, started %v", e.Products, e.Launched)
			}
		})
	}
}

func TestMSIApplyAnAlreadyRemovedProductIsFine(t *testing.T) {
	e := newMSIEnv(t, false)
	e.uninstallStatus = 1605
	if err := e.Apply.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.installed() != "1.6.0" {
		t.Fatalf("installed %v", e.Products)
	}
}

func TestMSIApplyAppThatNeverQuitsChangesNothing(t *testing.T) {
	e := newMSIEnv(t, true)
	e.waitErr = context.DeadlineExceeded
	err := e.Apply.Run(context.Background())
	if !errors.Is(err, ErrStillRunning) {
		t.Fatalf("Run = %v, want ErrStillRunning", err)
	}
	if len(e.Calls) != 0 || e.installed() != "1.5.5" || len(e.Launched) != 0 {
		t.Fatalf("calls %v, installed %v, started %v", e.Calls, e.Products, e.Launched)
	}
}

func TestMSIApplyRefusesInstallersThatChangedOnDisk(t *testing.T) {
	for name, which := range map[string]func(e *msiEnv) Ready{
		"the new installer":      func(e *msiEnv) Ready { return e.Apply.New },
		"the rollback installer": func(e *msiEnv) Ready { return e.Apply.Rollback },
	} {
		t.Run(name, func(t *testing.T) {
			e := newMSIEnv(t, true)
			r := which(e)
			if err := osWrite(r.Path(), strings.Repeat("x", int(r.Size))); err != nil {
				t.Fatal(err)
			}
			if err := e.Apply.Run(context.Background()); err == nil {
				t.Fatal("Run = nil")
			}
			if len(e.Calls) != 0 || e.installed() != "1.5.5" {
				t.Fatalf("calls %v, installed %v; nothing may be removed first", e.Calls, e.Products)
			}
		})
	}
}

func TestMSIApplyStopsWhenTheInstallIsNoLongerOurs(t *testing.T) {
	e := newMSIEnv(t, true)
	e.Products = append(e.Products, e.Products[0]) // a second registration appeared
	if err := e.Apply.Run(context.Background()); err == nil || len(e.Calls) != 0 {
		t.Fatalf("Run = %v, calls %v", err, e.Calls)
	}
}

func TestMSIApplyLockIsHeldAndReleased(t *testing.T) {
	e := newMSIEnv(t, false)
	held := false
	e.Apply.Lock = func(context.Context) (func(), error) { held = true; return func() { held = false }, nil }
	e.Apply.Msiexec = func(ctx context.Context, args []string) (uint32, error) {
		if !held {
			t.Error("msiexec ran without the lock")
		}
		return e.msiexec(ctx, args)
	}
	if err := e.Apply.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("the lock was not released")
	}
	e = newMSIEnv(t, false)
	e.Apply.Lock = func(context.Context) (func(), error) { return nil, ErrBusy }
	if err := e.Apply.Run(context.Background()); !errors.Is(err, ErrBusy) || len(e.Calls) != 0 {
		t.Fatalf("Run = %v, calls %v", err, e.Calls)
	}
}

func osWrite(path, data string) error { return os.WriteFile(path, []byte(data), 0o600) }

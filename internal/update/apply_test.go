package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type swapEnv struct {
	*macEnv
	Swap     *Swap
	launched []string
	logs     []string
}

// newSwapEnv is an installed "old" app with the "new" one staged beside it.
func newSwapEnv(t *testing.T, relaunch bool) *swapEnv {
	t.Helper()
	e := &swapEnv{macEnv: newMacEnv(t)}
	if _, err := e.Mac.Stage(context.Background(), e.Ready); err != nil {
		t.Fatal(err)
	}
	e.Swap = &Swap{
		Mac: e.Mac, Version: "1.6.0", Relaunch: relaunch,
		WaitExit: func(context.Context) error { return nil },
		Launch: func(_ context.Context, bundle string) error {
			data, _ := os.ReadFile(filepath.Join(bundle, "Contents", "marker"))
			e.launched = append(e.launched, string(data))
			return nil
		},
		Log: func(format string, args ...any) { e.logs = append(e.logs, fmt.Sprintf(format, args...)) },
	}
	return e
}

func (e *swapEnv) marker(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.Mac.Bundle, "Contents", "marker"))
	if err != nil {
		t.Fatalf("no app at %s: %v", e.Mac.Bundle, err)
	}
	return string(data)
}

func TestSwapInstallsTheStagedApp(t *testing.T) {
	e := newSwapEnv(t, false)
	if err := e.Swap.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.marker(t); got != "new" {
		t.Fatalf("installed app = %q, want the new one", got)
	}
	if got := e.siblings(); len(got) != 1 {
		t.Fatalf("leftovers next to the app: %v", got)
	}
	if len(e.launched) != 0 {
		t.Fatalf("opened the app after a plain quit: %v", e.launched)
	}
}

func TestSwapRestartOpensTheNewApp(t *testing.T) {
	e := newSwapEnv(t, true)
	if err := e.Swap.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.launched) != 1 || e.launched[0] != "new" {
		t.Fatalf("opened %v, want the new app once", e.launched)
	}
	if got := e.siblings(); len(got) != 1 {
		t.Fatalf("leftovers next to the app: %v", got)
	}
}

func TestSwapFailingAfterTheFirstRenamePutsTheOldAppBack(t *testing.T) {
	e := newSwapEnv(t, true)
	e.Swap.rename = func(from, to string) error {
		if from == e.Mac.StagedPath("1.6.0") {
			return errors.New("disk trouble")
		}
		return os.Rename(from, to)
	}
	err := e.Swap.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "disk trouble") {
		t.Fatalf("Run = %v", err)
	}
	if got := e.marker(t); got != "old" {
		t.Fatalf("installed app = %q, want the old one back", got)
	}
	// A restart still opens an app: the old one.
	if len(e.launched) != 1 || e.launched[0] != "old" {
		t.Fatalf("opened %v, want the old app", e.launched)
	}
	if exists(e.Swap.BackupPath()) {
		t.Fatal("the backup stayed beside the restored app")
	}
}

func TestSwapWhoseNewAppWillNotOpenGoesBack(t *testing.T) {
	e := newSwapEnv(t, true)
	open := e.Swap.Launch
	e.Swap.Launch = func(ctx context.Context, bundle string) error {
		if e.marker(t) == "new" {
			return errors.New("The application cannot be opened")
		}
		return open(ctx, bundle)
	}
	if err := e.Swap.Run(context.Background()); err == nil {
		t.Fatal("Run = nil")
	}
	if got := e.marker(t); got != "old" {
		t.Fatalf("installed app = %q, want the old one back", got)
	}
	if len(e.launched) != 1 || e.launched[0] != "old" {
		t.Fatalf("opened %v, want the old app", e.launched)
	}
}

func TestSwapThatCannotMoveTheOldAppChangesNothing(t *testing.T) {
	for _, relaunch := range []bool{false, true} {
		e := newSwapEnv(t, relaunch)
		e.Swap.rename = func(from, to string) error { return errors.New("permission denied") }
		if err := e.Swap.Run(context.Background()); err == nil {
			t.Fatal("Run = nil")
		}
		if got := e.marker(t); got != "old" {
			t.Fatalf("installed app = %q", got)
		}
		if want := map[bool]int{false: 0, true: 1}[relaunch]; len(e.launched) != want || (want == 1 && e.launched[0] != "old") {
			t.Fatalf("relaunch %v: opened %v", relaunch, e.launched)
		}
	}
}

func TestSwapThatCannotLockReopensTheAppInARestart(t *testing.T) {
	e := newSwapEnv(t, true)
	e.Swap.Lock = func(context.Context) (func(), error) { return nil, ErrBusy }
	if err := e.Swap.Run(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatalf("Run = %v", err)
	}
	if got := e.marker(t); got != "old" || len(e.launched) != 1 || e.launched[0] != "old" {
		t.Fatalf("installed %q, opened %v", got, e.launched)
	}
}

func TestSwapWaitsForTheAppAndGivesUpWhenItNeverQuits(t *testing.T) {
	e := newSwapEnv(t, true)
	e.Swap.WaitExit = func(context.Context) error { return context.DeadlineExceeded }
	err := e.Swap.Run(context.Background())
	if !errors.Is(err, ErrStillRunning) {
		t.Fatalf("Run = %v, want ErrStillRunning", err)
	}
	if got := e.marker(t); got != "old" {
		t.Fatalf("installed app = %q", got)
	}
	if len(e.launched) != 0 {
		t.Fatalf("opened %v while the app was still running", e.launched)
	}
	if !exists(e.Mac.StagedPath("1.6.0")) {
		t.Fatal("the staged app was removed, so the next quit cannot install it")
	}
}

func TestSwapChecksTheStagedAppAgainBeforeUsingIt(t *testing.T) {
	// The staged copy sat in a folder the user can write to.
	for name, change := range map[string]func(f *fakeMac){
		"a bad signature": func(f *fakeMac) { f.badSignature = true },
		"another team":    func(f *fakeMac) { f.newTeam = "OTHER99999" },
		"Gatekeeper":      func(f *fakeMac) { f.gatekeeperNo = true },
	} {
		t.Run(name, func(t *testing.T) {
			e := newSwapEnv(t, true)
			change(e.Fake)
			if err := e.Swap.Run(context.Background()); err == nil || errors.Is(err, ErrStillRunning) {
				t.Fatalf("Run = %v", err)
			}
			if got := e.marker(t); got != "old" {
				t.Fatalf("installed app = %q", got)
			}
			// The app had quit for a restart: the user gets the old one back.
			if len(e.launched) != 1 || e.launched[0] != "old" {
				t.Fatalf("opened %v, want the old app", e.launched)
			}
		})
	}
}

func TestSwapPicksUpAfterACutOffSwap(t *testing.T) {
	// The earlier helper moved the app aside and died before the next rename.
	e := newSwapEnv(t, false)
	if err := os.Rename(e.Mac.Bundle, e.Swap.BackupPath()); err != nil {
		t.Fatal(err)
	}
	if err := e.Swap.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := e.marker(t); got != "new" {
		t.Fatalf("installed app = %q", got)
	}
	if got := e.siblings(); len(got) != 1 {
		t.Fatalf("leftovers next to the app: %v", got)
	}
}

func TestSwapLogsWhatItDid(t *testing.T) {
	e := newSwapEnv(t, false)
	if err := e.Swap.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(e.logs) == 0 || !strings.Contains(e.logs[0], "1.6.0") {
		t.Fatalf("log = %v", e.logs)
	}
}

func TestAFailedInstallSendsTheUserToTheReleasePage(t *testing.T) {
	e := newMgr(t, nil)
	e.M.Offer(e.info("1.6.0"))
	waitFor(t, e.M, StateReady)

	// The helper could not install it.
	if err := e.M.Store.GiveUp(context.Background(), "1.6.0"); err != nil {
		t.Fatal(err)
	}
	// The next launch does not offer to install it again...
	m := &Manager{Store: e.M.Store, Supported: e.Supported.Load, Apply: e.M.Apply}
	m.Resume("1.5.5")
	if s := m.Status(); s.State != StateManual || s.Version != "1.6.0" || s.URL == "" {
		t.Fatalf("after Resume: %+v", s)
	}
	// ...and neither does the daily check.
	m.Offer(e.info("1.6.0"))
	if s := m.Status(); s.State != StateManual {
		t.Fatalf("after Offer: %+v", s)
	}
	if _, ok := m.Pending(); ok {
		t.Fatal("a failed install is still pending")
	}
	if e.Srv.Hits.Load() != 1 {
		t.Fatalf("downloaded again (%d downloads)", e.Srv.Hits.Load())
	}
	// A newer release starts over.
	m.Offer(e.info("1.6.1"))
	waitFor(t, m, StateReady)
}

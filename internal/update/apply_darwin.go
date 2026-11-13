package update

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	applyFlag    = "--apply-update"
	applyLogName = "apply.log"
	// applyTimeout is how long the helper waits for the app to quit. The app
	// starts the helper as it exits, so this only matters when it hangs.
	applyTimeout = 2 * time.Minute
	helperPrefix = "agent-sessions-apply-"
)

// StartApply installs r after this process exits. It copies the running
// executable to a temporary folder, since the installed bundle is about to be
// replaced, and starts the copy detached with --apply-update. The copy waits
// for this process to end, swaps the staged app in and, when relaunch is set,
// opens it. r must have been staged by m.Stage.
func StartApply(m *Mac, r Ready, relaunch bool) error {
	if !exists(m.StagedPath(r.Version)) {
		return fmt.Errorf("version %s is not staged", r.Version)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", helperPrefix)
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, "agent-sessions-apply")
	if err := copyExecutable(exe, helper); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	args := []string{applyFlag, r.Dir, "--bundle", m.Bundle, "--pid", strconv.Itoa(os.Getpid())}
	if relaunch {
		args = append(args, "--relaunch")
	}
	cmd := exec.Command(helper, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return cmd.Process.Release()
}

func copyExecutable(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ApplyMain runs the helper when the program was started with --apply-update.
// ok is false for a normal start.
func ApplyMain(args []string) (code int, ok bool) {
	if len(args) < 2 || args[0] != applyFlag {
		return 0, false
	}
	readyDir := args[1]
	fs := flag.NewFlagSet("apply-update", flag.ContinueOnError)
	bundle := fs.String("bundle", "", "installed app")
	pid := fs.Int("pid", 0, "process to wait for")
	relaunch := fs.Bool("relaunch", false, "open the new app afterwards")
	if err := fs.Parse(args[2:]); err != nil || *bundle == "" || *pid <= 0 {
		fmt.Fprintln(os.Stderr, "usage: --apply-update <ready folder> --bundle <app> --pid <n> [--relaunch]")
		return 2, true
	}
	// The helper is its own copy in a temporary folder; remove it on the way out.
	if exe, err := os.Executable(); err == nil && strings.HasPrefix(filepath.Base(filepath.Dir(exe)), helperPrefix) {
		defer os.RemoveAll(filepath.Dir(exe))
	}
	if err := runApply(readyDir, *bundle, *pid, *relaunch); err != nil {
		return 1, true
	}
	return 0, true
}

func runApply(readyDir, bundle string, pid int, relaunch bool) error {
	version := filepath.Base(readyDir)
	store := &Store{Dir: filepath.Dir(readyDir)}
	logger := openApplyLog(store.Dir)
	swap := &Swap{
		Mac:      &Mac{Bundle: bundle},
		Version:  version,
		Relaunch: relaunch,
		WaitExit: func(ctx context.Context) error { return waitExit(ctx, pid, 100*time.Millisecond) },
		Launch: func(ctx context.Context, bundle string) error {
			out, err := exec.CommandContext(ctx, "open", bundle).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%w: %s", err, trim(out))
			}
			return nil
		},
		Lock: store.Exclusive,
		Log:  func(format string, args ...any) { logger.Printf(format, args...) },
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	err := swap.Run(ctx)
	switch {
	case err == nil:
		logger.Printf("update to %s done", version)
	case errors.Is(err, ErrStillRunning):
		logger.Printf("update to %s not installed: %v", version, err)
	default:
		logger.Printf("update to %s failed: %v", version, err)
		// Trying again would fail the same way; the next launch shows the
		// release page for this version.
		if gerr := store.GiveUp(context.Background(), version); gerr != nil {
			logger.Printf("record the failure: %v", gerr)
		}
	}
	return err
}

// openApplyLog appends to apply.log in the updates folder; if that cannot be
// opened, the log goes nowhere.
func openApplyLog(dir string) *log.Logger {
	if err := os.MkdirAll(dir, 0o700); err == nil {
		if f, err := os.OpenFile(filepath.Join(dir, applyLogName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			return log.New(f, "", log.LstdFlags)
		}
	}
	return log.New(io.Discard, "", 0)
}

// waitExit returns when the process pid no longer exists, or when ctx ends.
func waitExit(ctx context.Context, pid int, poll time.Duration) error {
	for {
		// Signal 0 only checks that the process exists. EPERM means it does.
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(poll):
		}
	}
}

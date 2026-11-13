package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	applyFlag    = "--apply-update"
	applyLogName = "apply.log"
	// applyTimeout is how long the helper waits for the app to quit. The app
	// starts the helper as it exits, so this only matters when it hangs.
	applyTimeout = 2 * time.Minute
	helperPrefix = "agent-sessions-apply-"
	// helperDirName is the folder in the updates folder that holds the
	// helper's copy of the app. It is not a version, so the store's cleanup
	// leaves it alone.
	helperDirName = "helper"
	// helperStartTimeout is how long StartApply waits for the helper to say
	// it is running before it gives up and reports that no installer took
	// over.
	helperStartTimeout = 20 * time.Second
	// helperReady is what the helper writes to its notify pipe once it runs.
	helperReady = "ok\n"
)

// awaitHelper waits up to timeout for the started helper cmd to write
// helperReady to pr, the read end of its notify pipe, and then lets the
// process go. A helper that exits, closes the pipe or stays silent is killed,
// reaped and reported: the app must not quit counting on it.
func awaitHelper(cmd *exec.Cmd, pr *os.File, timeout time.Duration) error {
	got := make(chan error, 1)
	go func() {
		buf := make([]byte, len(helperReady))
		_, err := io.ReadFull(pr, buf)
		if err == nil && string(buf) != helperReady {
			err = fmt.Errorf("unexpected report %q", buf)
		}
		got <- err
	}()
	var readErr error
	select {
	case readErr = <-got:
		if readErr == nil {
			return cmd.Process.Release()
		}
	case <-time.After(timeout):
		readErr = os.ErrDeadlineExceeded
	}
	_ = cmd.Process.Kill()
	waitErr := cmd.Wait()
	switch {
	case errors.Is(readErr, os.ErrDeadlineExceeded):
		return fmt.Errorf("the update helper did not start within %s", timeout)
	case waitErr != nil:
		return fmt.Errorf("the update helper did not start: %w", waitErr)
	default:
		return errors.New("the update helper did not start")
	}
}

// reportStarted tells the app that started this helper that it runs, on the
// inherited pipe fd, and closes it.
func reportStarted(fd uintptr) {
	if f := os.NewFile(fd, "notify"); f != nil {
		_, _ = f.WriteString(helperReady)
		_ = f.Close()
	}
}

// ErrStillRunning means the app did not quit in time, so the helper left the
// installed app alone. It is not a failed update: the next quit tries again.
var ErrStillRunning = errors.New("the app did not quit")

// Swap replaces the installed bundle with the staged one, after the app has
// quit. Anything that goes wrong puts the old bundle back, so the user always
// has an app to open.
type Swap struct {
	Mac     *Mac
	Version string
	// Relaunch opens the new app once it is in place.
	Relaunch bool
	// WaitExit returns when the app has quit, or an error if it did not.
	WaitExit func(ctx context.Context) error
	// Launch opens the app at bundle.
	Launch func(ctx context.Context, bundle string) error
	// Lock takes the updates folder's lock for the swap. Nil means none.
	Lock func(ctx context.Context) (unlock func(), err error)
	// Log records what happened. Nil means nothing.
	Log func(format string, args ...any)

	rename func(oldpath, newpath string) error // os.Rename unless a test replaces it
}

func (s *Swap) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

func (s *Swap) move(from, to string) error {
	if s.rename != nil {
		return s.rename(from, to)
	}
	return os.Rename(from, to)
}

// BackupPath is where the old bundle waits during the swap, beside it.
func (s *Swap) BackupPath() string {
	return filepath.Join(filepath.Dir(s.Mac.Bundle), "."+filepath.Base(s.Mac.Bundle)+".old-"+s.Version)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Run waits for the app to quit and installs the staged version.
func (s *Swap) Run(ctx context.Context) error {
	if err := s.WaitExit(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrStillRunning, err)
	}
	// The app has quit. Until the installed bundle is moved aside, a failure
	// leaves it as it was, and a restart opens it again (reopen).
	if s.Lock != nil {
		unlock, err := s.Lock(ctx)
		if err != nil {
			return s.reopen(ctx, fmt.Errorf("lock the updates folder: %w", err))
		}
		defer unlock()
	}
	bundle, backup, staged := s.Mac.Bundle, s.BackupPath(), s.Mac.StagedPath(s.Version)

	// A swap that was cut off (power loss, a killed helper) may have left the
	// old app aside and nothing in place.
	if !exists(bundle) && exists(backup) {
		s.logf("the app was missing; putting back %s", backup)
		if err := s.move(backup, bundle); err != nil {
			return fmt.Errorf("restore the earlier app: %w", err)
		}
	}
	if err := s.Mac.Eligible(ctx); err != nil {
		return s.reopen(ctx, err)
	}
	if err := s.Mac.verify(ctx, staged, s.Version, true); err != nil {
		return s.reopen(ctx, err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return s.reopen(ctx, fmt.Errorf("clear an old backup: %w", err))
	}

	if err := s.move(bundle, backup); err != nil {
		return s.reopen(ctx, fmt.Errorf("move the installed app aside: %w", err))
	}
	if err := s.move(staged, bundle); err != nil {
		err = fmt.Errorf("put the new app in place: %w", err)
		return s.rollback(ctx, err)
	}
	s.logf("installed %s", s.Version)
	if s.Relaunch {
		if err := s.Launch(ctx, bundle); err != nil {
			return s.rollback(ctx, fmt.Errorf("open the new app: %w", err))
		}
	}
	if err := os.RemoveAll(backup); err != nil {
		s.logf("remove the old app %s: %v", backup, err) // the update itself worked
	}
	return nil
}

// reopen, in a restart, opens the installed app that a failed update left
// in place, then returns cause. The user asked for the app back and gets it.
func (s *Swap) reopen(ctx context.Context, cause error) error {
	if s.Relaunch && exists(s.Mac.Bundle) {
		if err := s.Launch(ctx, s.Mac.Bundle); err != nil {
			s.logf("open the app: %v", err)
		}
	}
	return cause
}

// rollback puts the old app back and, in a restart, opens it, then returns
// cause (joined with whatever the rollback itself could not do).
func (s *Swap) rollback(ctx context.Context, cause error) error {
	bundle, backup, staged := s.Mac.Bundle, s.BackupPath(), s.Mac.StagedPath(s.Version)
	s.logf("%v; putting the old app back", cause)
	if exists(bundle) {
		// The new app goes back where it was staged.
		if err := s.move(bundle, staged); err != nil {
			s.logf("move the new app out of the way: %v", err)
			if err := os.RemoveAll(bundle); err != nil {
				return errors.Join(cause, fmt.Errorf("the old app is at %s and could not be put back: %w", backup, err))
			}
		}
	}
	if err := s.move(backup, bundle); err != nil {
		return errors.Join(cause, fmt.Errorf("the old app is at %s and could not be put back: %w", backup, err))
	}
	if s.Relaunch {
		if err := s.Launch(ctx, bundle); err != nil {
			s.logf("open the old app: %v", err)
		}
	}
	return cause
}

// Exclusive takes the updates folder's lock, waiting for whoever has it until
// ctx ends. Call the result to release it.
func (s *Store) Exclusive(ctx context.Context) (unlock func(), err error) {
	l, err := s.lock(ctx, true)
	if err != nil {
		return nil, err
	}
	return func() { l.Unlock() }, nil
}

// GiveUp makes the app stop trying to install version by itself: it shows the
// release page from now on. It is for an install that failed, which would
// fail again.
func (s *Store) GiveUp(ctx context.Context, version string) error {
	unlock, err := s.Exclusive(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	failures := s.readFailures()
	failures[version] = s.maxFailures()
	return s.writeFailures(failures)
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

// recordApply logs how the helper ended. A failed install is marked, so the
// next launch shows the release page for that version rather than trying
// again. An app that never quit is not a failure.
func recordApply(store *Store, version string, logf func(string, ...any), err error) {
	switch {
	case err == nil:
		logf("update to %s done", version)
	case errors.Is(err, ErrStillRunning):
		logf("update to %s not installed: %v", version, err)
	default:
		logf("update to %s failed: %v", version, err)
		if gerr := store.GiveUp(context.Background(), version); gerr != nil {
			logf("record the failure: %v", gerr)
		}
	}
}

package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

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
	if s.Lock != nil {
		unlock, err := s.Lock(ctx)
		if err != nil {
			return fmt.Errorf("lock the updates folder: %w", err)
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
		return err
	}
	if err := s.Mac.verify(ctx, staged, s.Version, true); err != nil {
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("clear an old backup: %w", err)
	}

	if err := s.move(bundle, backup); err != nil {
		return fmt.Errorf("move the installed app aside: %w", err)
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

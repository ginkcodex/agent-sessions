package update

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// StartApply installs r after this process exits. It copies the running app
// bundle to the updates folder, since the installed bundle is about to be
// replaced, and starts the copy's executable detached with --apply-update.
// The copy waits for this process to end, swaps the staged app in and, when
// relaunch is set, opens it. r must have been staged by m.Stage.
//
// The whole bundle is copied, not just the executable: a Developer ID
// executable with the hardened runtime is bound to its bundle's Info.plist,
// and macOS kills a copy that runs without it. StartApply returns nil only
// once the helper has reported in. Otherwise the error tells the caller that
// no installer took over, so a restart starts the old app again rather than
// leaving the user with none.
func StartApply(m *Mac, r Ready, relaunch bool) error {
	if !exists(m.StagedPath(r.Version)) {
		return fmt.Errorf("version %s is not staged", r.Version)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	running := MacBundle(exe)
	if running == "" {
		return fmt.Errorf("%s is not inside an app bundle", exe)
	}
	rel, err := filepath.Rel(running, exe)
	if err != nil {
		return err
	}
	// A helper from an earlier update removes its copy as it ends; one that
	// was killed leaves it behind. Remove that folder now; a helper that is
	// somehow still running keeps working from its open files.
	parent := filepath.Join(filepath.Dir(r.Dir), helperDirName)
	_ = os.RemoveAll(parent)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, helperPrefix)
	if err != nil {
		return err
	}
	copied := filepath.Join(dir, filepath.Base(running))
	if err := copyBundle(running, copied); err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("copy the app for the update helper: %w", err)
	}
	args := []string{applyFlag, r.Dir, "--bundle", m.Bundle, "--pid", strconv.Itoa(os.Getpid()), "--notify-fd", "3"}
	if relaunch {
		args = append(args, "--relaunch")
	}
	cmd := exec.Command(filepath.Join(copied, rel), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := startHelper(cmd, helperStartTimeout); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return nil
}

// startHelper starts cmd with a pipe as file descriptor 3 and waits up to
// timeout for it to report in there (awaitHelper).
func startHelper(cmd *exec.Cmd, timeout time.Duration) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	cmd.ExtraFiles = []*os.File{pw}
	err = cmd.Start()
	pw.Close()
	if err != nil {
		return fmt.Errorf("start the update helper: %w", err)
	}
	return awaitHelper(cmd, pr, timeout)
}

// copyBundle copies the app bundle from to the new path to. It clones it
// where the volume allows, which takes no time or space, and copies it
// otherwise.
func copyBundle(from, to string) error {
	if err := unix.Clonefile(from, to, unix.CLONE_NOFOLLOW); err == nil {
		return nil
	}
	_ = os.RemoveAll(to)
	return copyTree(from, to)
}

// copyTree copies the folder from to the new path to: folders, regular files
// with their permissions, and symbolic links as links.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch mode := info.Mode(); {
		case mode.IsDir():
			return os.Mkdir(target, mode.Perm()|0o700)
		case mode&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case mode.IsRegular():
			return copyFile(path, target, mode.Perm())
		default:
			return fmt.Errorf("%s is not a file, folder or link", path)
		}
	})
}

func copyFile(from, to string, perm fs.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
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
	flags := flag.NewFlagSet("apply-update", flag.ContinueOnError)
	bundle := flags.String("bundle", "", "installed app")
	pid := flags.Int("pid", 0, "process to wait for")
	relaunch := flags.Bool("relaunch", false, "open the new app afterwards")
	notify := flags.Int("notify-fd", 0, "file descriptor to report on once running")
	if err := flags.Parse(args[2:]); err != nil || *bundle == "" || *pid <= 0 {
		fmt.Fprintln(os.Stderr, "usage: --apply-update <ready folder> --bundle <app> --pid <n> [--notify-fd <n>] [--relaunch]")
		return 2, true
	}
	if *notify > 2 {
		reportStarted(uintptr(*notify))
	}
	// The helper runs from its own copy of the bundle in the updates folder;
	// remove that copy on the way out.
	if exe, err := os.Executable(); err == nil {
		if copied := MacBundle(exe); copied != "" && strings.HasPrefix(filepath.Base(filepath.Dir(copied)), helperPrefix) {
			defer os.RemoveAll(filepath.Dir(copied))
		}
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
	recordApply(store, version, logger.Printf, err)
	return err
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

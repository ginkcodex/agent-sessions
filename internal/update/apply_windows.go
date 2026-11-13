package update

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/ginkcode/agent-sessions/internal/winmsi"
	"golang.org/x/sys/windows"
)

const (
	// idleTimeout is how long the helper waits for the app to quit and for
	// every process in the install folder to go away.
	idleTimeout = time.Minute
	// msiTimeout bounds each msiexec run.
	msiTimeout = 10 * time.Minute
)

// StartApply installs r after this process exits. It copies the running
// executable to the updates folder, since msiexec replaces the installed one,
// and starts the copy detached with --apply-update. The copy waits for this
// process to end, runs the removal and installation of the MSIs and, when
// relaunch is set, starts the new app. r must have been prepared by w.Prepare.
//
// StartApply returns nil only once the helper has reported in. Otherwise the
// error tells the caller that no installer took over, so a restart starts the
// old app again rather than leaving the user with none.
func StartApply(w *Windows, r Ready, relaunch bool) error {
	if _, ok := w.Rollback(r); !ok {
		return fmt.Errorf("version %s has no rollback installer", r.Version)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// A helper from an earlier update cannot delete itself while it runs.
	// Remove that folder now; if it is somehow still running, the new copy
	// goes beside it.
	parent := filepath.Join(w.Store.Dir, helperDirName)
	_ = os.RemoveAll(parent)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, helperPrefix)
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, "agent-sessions-apply.exe")
	if err := copyExecutable(exe, helper); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	args := []string{applyFlag, r.Dir, "--pid", strconv.Itoa(os.Getpid()), "--from", w.Version}
	if relaunch {
		args = append(args, "--relaunch")
	}
	if err := startDetached(helper, args, helperStartTimeout); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return nil
}

// startDetached starts exe with args so that it outlives this process, and
// waits up to timeout for it to report in (startHelper). A job object around
// the app may kill its children when it closes, so it first asks to leave the
// job, and starts it without that where the job forbids it. A helper that
// started and then failed is not started again.
func startDetached(exe string, args []string, timeout time.Duration) error {
	const detached = windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP
	var err error
	for _, flags := range []uint32{detached | windows.CREATE_BREAKAWAY_FROM_JOB, detached} {
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags, HideWindow: true}
		if err = startHelper(cmd, timeout); err == nil || cmd.Process != nil {
			return err
		}
	}
	return err
}

// startHelper starts cmd with an inheritable pipe, whose handle it adds to the
// arguments as --notify-handle, and waits up to timeout for the helper to
// report in there (awaitHelper).
func startHelper(cmd *exec.Cmd, timeout time.Duration) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pr.Close()
	h := windows.Handle(pw.Fd())
	if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		pw.Close()
		return fmt.Errorf("start the update helper: %w", err)
	}
	cmd.Args = append(cmd.Args, "--notify-handle", strconv.FormatUint(uint64(h), 10))
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Only this handle and the standard ones are inherited.
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(h))
	err = cmd.Start()
	pw.Close()
	if err != nil {
		return fmt.Errorf("start the update helper: %w", err)
	}
	return awaitHelper(cmd, pr, timeout)
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
	pid := fs.Int("pid", 0, "process to wait for")
	from := fs.String("from", "", "version being replaced")
	relaunch := fs.Bool("relaunch", false, "start the new app afterwards")
	notify := fs.Uint64("notify-handle", 0, "pipe handle to report on once running")
	if err := fs.Parse(args[2:]); err != nil || *pid <= 0 || *from == "" {
		fmt.Fprintln(os.Stderr, "usage: --apply-update <ready folder> --pid <n> --from <version> [--relaunch] [--notify-handle <n>]")
		return 2, true
	}
	if *notify != 0 {
		reportStarted(uintptr(*notify))
	}
	if err := runApply(readyDir, *pid, *from, *relaunch); err != nil {
		return 1, true
	}
	return 0, true
}

func runApply(readyDir string, pid int, from string, relaunch bool) error {
	version := filepath.Base(readyDir)
	store := &Store{Dir: filepath.Dir(readyDir)}
	logger := openApplyLog(store.Dir)
	fail := func(err error) error {
		recordApply(store, version, logger.Printf, err)
		return err
	}

	local := os.Getenv("LOCALAPPDATA")
	installed := (&Windows{LocalAppData: local}).InstallPath()
	// Failures before MSIApply runs change nothing. In a restart the user
	// still gets the app back: the installed one, once this one has quit.
	early := func(err error) error {
		if relaunch {
			ctx, cancel := context.WithTimeout(context.Background(), idleTimeout)
			defer cancel()
			if werr := waitProcess(ctx, pid); werr != nil {
				logger.Printf("not starting the app again: %v", werr)
			} else if lerr := launchDetached(installed); lerr != nil {
				logger.Printf("start the app: %v", lerr)
			}
		}
		return fail(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return early(fmt.Errorf("read the current user: %w", err))
	}
	// The helper runs from a copy; the app it replaces is the one the MSI
	// installs.
	win := &Windows{
		LocalAppData: local, SID: user.User.Sid.String(), Version: from,
		Products: winmsi.InstalledProducts, ReadMSI: winmsi.ReadInfo, Store: store,
	}
	win.Exe = win.InstallPath()

	newReady, ok := ReadReady(readyDir)
	if !ok {
		return early(fmt.Errorf("%s holds no downloaded update", readyDir))
	}
	rollback, ok := win.Rollback(newReady)
	if !ok {
		return early(fmt.Errorf("version %s has no rollback installer", version))
	}
	msiexec, err := msiexecPath()
	if err != nil {
		return early(err)
	}

	a := &MSIApply{
		Win: win, New: newReady, Rollback: rollback, Relaunch: relaunch,
		WaitIdle: func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, idleTimeout)
			defer cancel()
			return waitIdle(ctx, pid, filepath.Dir(win.InstallPath()), 200*time.Millisecond)
		},
		Msiexec: func(ctx context.Context, args []string) (uint32, error) {
			ctx, cancel := context.WithTimeout(ctx, msiTimeout)
			defer cancel()
			return runMsiexec(ctx, msiexec, args)
		},
		LogPath: func(name string) string { return filepath.Join(store.Dir, "msi-"+name) },
		Launch:  launchDetached,
		Lock:    store.Exclusive,
		Log:     func(format string, args ...any) { logger.Printf(format, args...) },
	}
	return fail(a.Run(context.Background()))
}

// msiexecPath is msiexec.exe in the system folder, not found by searching.
func msiexecPath() (string, error) {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return "", fmt.Errorf("find the system folder: %w", err)
	}
	return filepath.Join(dir, "msiexec.exe"), nil
}

// runMsiexec runs msiexec and returns its exit status. A status other than 0
// is not an error here; the caller decides what each one means.
func runMsiexec(ctx context.Context, msiexec string, args []string) (uint32, error) {
	cmd := exec.CommandContext(ctx, msiexec, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return uint32(exit.ExitCode()), nil
	}
	return 0, err
}

// launchDetached starts exe so that it does not depend on this process.
func launchDetached(exe string) error {
	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// waitIdle returns once the process pid has ended and no process runs from a
// file in dir, or an error when ctx ends first.
func waitIdle(ctx context.Context, pid int, dir string, poll time.Duration) error {
	if err := waitProcess(ctx, pid); err != nil {
		return err
	}
	for {
		busy, err := runningIn(dir)
		if err != nil {
			return err
		}
		if !busy {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("a program from %s is still running", dir)
		case <-time.After(poll):
		}
	}
}

// waitProcess returns when the process pid has ended. A process that cannot
// be opened because it does not exist has ended.
func waitProcess(ctx context.Context, pid int) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return fmt.Errorf("wait for process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	for {
		ev, err := windows.WaitForSingleObject(h, 200)
		if err != nil {
			return fmt.Errorf("wait for process %d: %w", pid, err)
		}
		if ev == windows.WAIT_OBJECT_0 {
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("process %d is still running", pid)
		}
	}
}

// runningIn reports whether any process runs a file under dir. Processes that
// cannot be inspected (other users', protected ones) are skipped: they cannot
// be ours.
func runningIn(dir string) (bool, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, fmt.Errorf("list processes: %w", err)
	}
	defer windows.CloseHandle(snap)
	prefix := strings.ToLower(filepath.Clean(dir)) + `\`
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == 0 || entry.ProcessID == uint32(os.Getpid()) {
			continue
		}
		if strings.HasPrefix(strings.ToLower(processImage(entry.ProcessID)), prefix) {
			return true, nil
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, fmt.Errorf("list processes: %w", err)
	}
	return false, nil
}

// processImage is the path of the program a process runs, or "".
func processImage(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

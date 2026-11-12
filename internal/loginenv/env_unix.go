//go:build !windows

package loginenv

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// passwdFile holds the accounts' shells; tests replace it.
var passwdFile = "/etc/passwd"

// Shell is the user's shell: $SHELL, else the account's shell from
// /etc/passwd, else the platform default. wsl.exe --exec, which starts the
// app's server on WSL, sets no $SHELL.
func Shell() string {
	if s := os.Getenv("SHELL"); filepath.IsAbs(s) {
		return s
	}
	if f, err := os.Open(passwdFile); err == nil {
		defer func() { _ = f.Close() }()
		uid := strconv.Itoa(os.Getuid())
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			fields := strings.Split(sc.Text(), ":")
			if len(fields) == 7 && fields[2] == uid && filepath.IsAbs(fields[6]) {
				return fields[6]
			}
		}
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	return "/bin/sh"
}

// read runs shell as an interactive login shell in dir that prints its
// environment, or returns nil. The shell runs in its own session, so it
// cannot take over the terminal the app was started from, and its whole
// process group is killed when it ends or times out: startup files may
// leave background jobs behind that hold its output open.
func read(shell, dir string, timeout time.Duration) []string {
	if shell == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", "echo "+marker+"; env -0")
	if filepath.IsAbs(dir) {
		cmd.Dir = dir
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	var out boundedBuffer
	cmd.Stdout = &out
	err := cmd.Run()
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// ErrWaitDelay: the shell succeeded, but a background job kept its
	// output open; what it printed before exiting is complete.
	if err != nil && !errors.Is(err, exec.ErrWaitDelay) {
		return nil
	}
	return parse(out.buf.Bytes())
}

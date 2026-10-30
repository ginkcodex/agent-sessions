//go:build !windows

package term

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/aymanbagabas/go-pty"
)

// ptyConsole is a console in a Unix pseudo-terminal.
type ptyConsole struct {
	pty.Pty
	cmd *pty.Cmd
}

// startConsole starts path, with argv as its arguments including the
// command, in a new pseudo-terminal of cols by rows.
func startConsole(path string, argv []string, dir string, env []string, cols, rows int) (console, error) {
	p, err := pty.New()
	if err != nil {
		return nil, fmt.Errorf("could not open a terminal: %w", err)
	}
	_ = p.Resize(cols, rows)
	cmd := p.Command(path, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Dir = dir
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		_ = p.Close()
		return nil, err
	}
	// Close the parent's copy of the terminal side, so reading ends once
	// the process and everything it started let go of it.
	if u, ok := p.(pty.UnixPty); ok {
		_ = u.Slave().Close()
	}
	return &ptyConsole{Pty: p, cmd: cmd}, nil
}

func (c *ptyConsole) Process() *os.Process { return c.cmd.Process }

func (c *ptyConsole) Wait() (*os.ProcessState, error) {
	err := c.cmd.Wait()
	return c.cmd.ProcessState, err
}

// terminate hangs up the process group (the process leads its own session)
// and kills what is left of it after killGrace. The kill does not wait on
// the process itself: a child that ignores SIGHUP outlives it, and on macOS
// the terminal ends as soon as the process does. Killing a group that is
// already empty does nothing.
func (s *session) terminate() {
	s.killOnce.Do(func() {
		s.unpause()
		pgid := s.con.Process().Pid
		_ = syscall.Kill(-pgid, syscall.SIGHUP)
		time.AfterFunc(s.killGrace, func() { _ = syscall.Kill(-pgid, syscall.SIGKILL) })
	})
}

// stateCode is the exit status, or 128 plus the signal that ended the
// process, as shells report it.
func stateCode(state *os.ProcessState) int {
	if ws, ok := state.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return state.ExitCode()
}

// detachShell runs cmd in its own session, without a controlling terminal,
// so an interactive shell cannot take over the one the app was started
// from.
func detachShell(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

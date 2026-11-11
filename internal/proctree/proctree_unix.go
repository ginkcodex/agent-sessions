//go:build darwin || linux

package proctree

import (
	"errors"
	"os/exec"
	"syscall"
)

// Tree is a started command and its descendants.
type Tree struct{ cmd *exec.Cmd }

// Start starts cmd as the leader of a new process group. It replaces
// cmd.SysProcAttr.
func Start(cmd *exec.Cmd) (*Tree, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Tree{cmd}, nil
}

// Interrupt asks the whole tree to stop (SIGTERM).
func (t *Tree) Interrupt() error { return t.signal(syscall.SIGTERM) }

// Kill stops the whole tree at once (SIGKILL). A tree that is already gone
// is not an error.
func (t *Tree) Kill() error { return t.signal(syscall.SIGKILL) }

func (t *Tree) signal(sig syscall.Signal) error {
	err := syscall.Kill(-t.cmd.Process.Pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// Release frees what Start allocated. Call it once the tree is stopped.
func (t *Tree) Release() error { return nil }

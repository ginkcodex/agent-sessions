//go:build !darwin && !linux && !windows

package proctree

import "os/exec"

// Tree is a started command and its descendants.
type Tree struct{}

// Start fails: this host has no process-tree ownership.
func Start(*exec.Cmd) (*Tree, error) { return nil, ErrUnsupported }

func (t *Tree) Interrupt() error { return ErrUnsupported }
func (t *Tree) Kill() error      { return ErrUnsupported }
func (t *Tree) Release() error   { return nil }

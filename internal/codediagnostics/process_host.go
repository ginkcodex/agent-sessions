//go:build darwin || linux || windows

package codediagnostics

import (
	"errors"
	"os/exec"

	"github.com/ginkcode/agent-sessions/internal/proctree"
)

type hostProcess struct {
	cmd  *exec.Cmd
	tree *proctree.Tree
}

func startProcess(in Invocation) (Process, error) {
	cmd := exec.Command(in.Executable, in.Args...)
	cmd.Dir, cmd.Env = in.Directory, in.Env
	cmd.Stdout, cmd.Stderr = in.Stdout, in.Stderr
	// Bounds pipe draining even when an uncooperative descendant keeps a pipe open.
	cmd.WaitDelay = in.CleanupTimeout
	tree, err := proctree.Start(cmd)
	if err != nil {
		return nil, err
	}
	return &hostProcess{cmd, tree}, nil
}

func (p *hostProcess) Wait() (int, error) {
	err := p.cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

func (p *hostProcess) Terminate() error { return p.tree.Kill() }
func (p *hostProcess) Close() error     { return p.tree.Release() }

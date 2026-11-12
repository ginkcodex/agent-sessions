//go:build !unix

package codexbridge

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// runReal runs the real codex on this process's stdio and returns its exit
// code.
func runReal(real string, args []string) int {
	cmd := exec.Command(real, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	}
	_, _ = fmt.Fprintf(os.Stderr, "agent-sessions: cannot run %s: %v\n", real, err)
	return 127
}

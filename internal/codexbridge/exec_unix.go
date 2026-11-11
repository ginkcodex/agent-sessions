//go:build unix

package codexbridge

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// runReal becomes the real codex, so codex-acp's signals and pipes reach it
// as before. It returns only when that fails.
func runReal(real string, args []string) int {
	// CODEX_PATH may name a program on PATH; Exec does not search.
	if path, err := exec.LookPath(real); err == nil {
		real = path
	}
	err := syscall.Exec(real, append([]string{real}, args...), os.Environ())
	_, _ = fmt.Fprintf(os.Stderr, "agent-sessions: cannot run %s: %v\n", real, err)
	return 127
}

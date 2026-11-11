//go:build unix

package codexbridge

import (
	"fmt"
	"os"
	"syscall"
)

// runReal becomes the real codex, so codex-acp's signals and pipes reach it
// as before. It returns only when that fails.
func runReal(real string, args []string) int {
	err := syscall.Exec(real, append([]string{real}, args...), os.Environ())
	_, _ = fmt.Fprintf(os.Stderr, "agent-sessions: cannot run %s: %v\n", real, err)
	return 127
}

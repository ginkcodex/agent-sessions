package rpc

import (
	"os"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/acp/acptest"
)

// The test binary also runs as the fake ACP agent of the chat tests.
func TestMain(m *testing.M) {
	acptest.Main()
	os.Exit(m.Run())
}

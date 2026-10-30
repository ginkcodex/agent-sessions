package engine

import (
	"os"
	"testing"

	"github.com/ginkcode/agent-sessions/internal/paths"
)

// Tests never run the developer's login shell; the ones that need a login
// PATH set loginShellPath themselves.
//
// Engines without a configured data directory log failed code checks to a
// temporary one, never the developer's.
func TestMain(m *testing.M) {
	loginShellPath = func(string) []string { return nil }
	logs, err := os.MkdirTemp("", "engine-check-logs-")
	if err != nil {
		panic(err)
	}
	real, _ := paths.Default()
	checkLogData = func(e *Engine) (string, error) {
		// NewEngine without session roots takes all the defaults, Data too.
		if e.roots.Data != "" && e.roots.Data != real.Data {
			return e.roots.Data, nil
		}
		return logs, nil
	}
	code := m.Run()
	_ = os.RemoveAll(logs)
	os.Exit(code)
}

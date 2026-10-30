//go:build unix

package fdlimit

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func childLimit(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("/bin/sh", "-c", "ulimit -n").Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// TestInheritPassesRaisedLimitToChildren re-runs itself under a soft limit of
// 256, as an app opened from Finder gets, so it holds even when go test runs
// from a terminal with a high limit.
func TestInheritPassesRaisedLimitToChildren(t *testing.T) {
	mode := os.Getenv("FDLIMIT_TEST_MODE")
	if mode == "" {
		for _, m := range []string{"plain", "inherit"} {
			cmd := exec.Command("/bin/sh", "-c", `ulimit -S -n 256 && exec "$0" -test.run='^TestInheritPassesRaisedLimitToChildren$' -test.v`, os.Args[0])
			cmd.Env = append(os.Environ(), "FDLIMIT_TEST_MODE="+m)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", m, err, out)
			}
		}
		return
	}
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		t.Fatal(err)
	}
	if lim.Cur <= 256 {
		t.Skipf("Go did not raise the limit (soft %d, hard %d)", lim.Cur, lim.Max)
	}
	if mode == "plain" {
		if got := childLimit(t); got != "256" {
			t.Fatalf("without Inherit the child got %s; Go no longer restores the original", got)
		}
		return
	}
	if err := Inherit(); err != nil {
		t.Fatal(err)
	}
	if got := childLimit(t); got != strconv.FormatUint(uint64(lim.Cur), 10) {
		t.Fatalf("child limit %s, want this process's %d", got, lim.Cur)
	}
}

//go:build !windows

package update

import (
	"os"
	"syscall"
)

// ownedByUser reports whether path belongs to the user running the app. A
// bundle another user owns can be renamed in a shared folder but not deleted.
func ownedByUser(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

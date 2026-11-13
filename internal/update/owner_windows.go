//go:build windows

package update

import "os"

// ownedByUser: Windows installs are not replaced through this check.
func ownedByUser(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

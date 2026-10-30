//go:build unix

// Package fdlimit lets child processes keep the open-file limit Go raised for
// this process.
//
// Apps opened from Finder or the Dock start with a soft limit of 256. Go raises
// its own limit at startup but sets 256 back for every child it starts, so
// shells, git, checkers and language servers run from the app get fewer open
// files than the same commands in a terminal. Calling Setrlimit explicitly
// tells Go to stop restoring it.
package fdlimit

import "syscall"

// Inherit makes processes started from now on get this process's current
// open-file limit (on macOS at most kern.maxfilesperproc, as in a terminal).
func Inherit() error {
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return err
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim)
}

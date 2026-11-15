//go:build unix

package files

import "golang.org/x/sys/unix"

const regularReadFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW

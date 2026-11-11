// Package proctree starts a command so that it and every process it spawns
// can be stopped together: in its own process group on Unix, and in a
// kill-on-close job object on Windows.
package proctree

import "errors"

// ErrUnsupported is returned on hosts without process-tree ownership.
var ErrUnsupported = errors.New("proctree: process-tree ownership is unavailable on this host")

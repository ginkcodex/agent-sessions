//go:build !desktop

package main

import (
	"fmt"
	"os"
)

// run is the fallback entrypoint compiled without the desktop build tag. It keeps plain `go build ./...` and `go test -race ./...` green on
// machines without GTK/WebKitGTK development headers.
func run() {
	fmt.Fprintln(os.Stderr, "agent-sessions: the desktop GUI requires a build with -tags desktop; see cmd/agent-sessions/main.go")
	os.Exit(1)
}

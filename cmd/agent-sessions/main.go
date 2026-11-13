// Command agent-sessions is the Wails v3 desktop application entrypoint.
//
// The GUI build is gated behind the `desktop` build tag so that plain
// `go build ./...` and `go test -race ./...` never require the GTK or
// WebKitGTK development headers:
//
//	make gui-build    # or: go build -tags desktop,production ./cmd/agent-sessions
//	make dev          # Vite plus a -tags desktop build (devtools on)
//
// System build dependencies on Linux: libgtk-4-dev, libwebkitgtk-6.0-dev
// (verify with: pkg-config --exists gtk4 webkitgtk-6.0).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ginkcode/agent-sessions/internal/codexbridge"
	"github.com/ginkcode/agent-sessions/internal/fdlimit"
	"github.com/ginkcode/agent-sessions/internal/remote"
	"github.com/ginkcode/agent-sessions/internal/update"
)

// WebKitGTK's DMA-BUF renderer stays on: Wails turns it off itself where
// WebKit renders on NVIDIA (isNVIDIAGPU in wails/v3/pkg/application), and an
// exported WEBKIT_DISABLE_DMABUF_RENDERER wins over both. Turning it off
// everywhere made the GTK main thread copy and composite every frame on the
// CPU, which pinned it and stuttered on fractional scales
// (docs/plan/linux-gpu-rendering.md).

func main() {
	// A Codex chat's adapter runs this program as its codex.
	if code, ok := codexbridge.Main(os.Args[1:]); ok {
		os.Exit(code)
	}
	// The update helper: a copy of this program that swaps in the new app
	// once this one has quit.
	if code, ok := update.ApplyMain(os.Args[1:]); ok {
		os.Exit(code)
	}
	if sock := os.Getenv("AGENT_SESSIONS_ASKPASS_SOCK"); sock != "" {
		prompt := ""
		if len(os.Args) > 1 {
			prompt = os.Args[1]
		}
		token := os.Getenv("AGENT_SESSIONS_ASKPASS_TOKEN")
		if err := remote.RunAskpassHelper(context.Background(), sock, token, prompt, os.Stdout, os.Stderr); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "agent-sessions askpass: %v\n", err)
			os.Exit(1)
		}
		return
	}
	// Language servers, terminals and checkers get a terminal's open-file
	// limit, not the 256 an app opened from Finder starts with.
	_ = fdlimit.Inherit()
	run()
}

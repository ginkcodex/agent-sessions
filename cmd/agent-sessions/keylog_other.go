//go:build !darwin && desktop

package main

import (
	"encoding/json"

	"github.com/ginkcode/agent-sessions/internal/keylog"
)

// hookKeyboardLog records native key events on macOS only; elsewhere the
// log holds the page's events alone.
func hookKeyboardLog(*keylog.Recorder) {}

func keyboardStateSnapshot() json.RawMessage { return nil }

package frontend

import _ "embed"

// AppIcon is the Icon Composer tile scaled to 824x824 and centered on a
// transparent 1024x1024 canvas (100px margins) to match macOS Dock sizing.
// Use the same artwork at runtime and for the bundle's icns.
//
//go:embed assets/Icon-macOS.png
var AppIcon []byte

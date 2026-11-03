package frontend

import _ "embed"

// AppIcon is the macOS tile exported from Icon Composer. Use the same artwork
// at runtime and for the bundle's icns so Wails preserves its background.
//
//go:embed assets/Icon-iOS-Default-1024x1024@1x.png
var AppIcon []byte

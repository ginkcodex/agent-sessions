//go:build !darwin

package frontend

import _ "embed"

// AppIcon is the transparent application icon used on Windows and Linux.
//
//go:embed assets/Icon-universal.png
var AppIcon []byte

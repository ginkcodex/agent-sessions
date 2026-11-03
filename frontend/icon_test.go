package frontend

import (
	"bytes"
	"image/png"
	"runtime"
	"testing"
)

func TestAppIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(AppIcon))
	if err != nil {
		t.Fatalf("decode embedded icon: %v", err)
	}
	if got := img.Bounds().Size(); got.X != 1024 || got.Y != 1024 {
		t.Fatalf("icon dimensions = %dx%d, want 1024x1024", got.X, got.Y)
	}
	if runtime.GOOS == "darwin" {
		// Keep the entire 100px margin transparent, not just rounded corners.
		for y := range 1024 {
			for x := range 1024 {
				if x >= 100 && x < 924 && y >= 100 && y < 924 {
					continue
				}
				if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
					t.Fatalf("macOS icon margin at (%d,%d) alpha = %#04x, want transparent", x, y, a)
				}
			}
		}
		// The tile's white background must remain opaque outside the artwork.
		r, g, b, alpha := img.At(245, 245).RGBA()
		if r < 0xffff*9/10 || g < 0xffff*9/10 || b < 0xffff*9/10 || alpha != 0xffff {
			t.Fatalf("macOS tile background = (%#04x, %#04x, %#04x, %#04x), want opaque near-white", r, g, b, alpha)
		}
	}
	// Rounded tile corners and the universal icon both show the desktop through.
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha > 0xffff/16 {
		t.Fatalf("icon corner alpha = %#04x, want near transparent", alpha)
	}
}

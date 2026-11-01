package app

import (
	"context"
	"errors"
	"runtime"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// fullscreenOperations is the small part of the desktop runtime needed to
// toggle fullscreen. Keeping it explicit lets the controller run without Wails.
type fullscreenOperations struct {
	isFullscreen func(context.Context) bool
	isMaximised  func(context.Context) bool
	fullscreen   func(context.Context)
	unfullscreen func(context.Context)
	unmaximise   func(context.Context)
	maximise     func(context.Context)
}

type fullscreenController struct {
	mu               sync.Mutex
	restoreMaximised bool
}

// ToggleFullscreen toggles this window's fullscreen state. On Windows, leaving
// fullscreen restores a window that was maximised before entering fullscreen.
func (a *App) ToggleFullscreen() error {
	return a.fullscreen.toggle(a.ctx, runtime.GOOS, fullscreenOperations{
		isFullscreen: wruntime.WindowIsFullscreen,
		isMaximised:  wruntime.WindowIsMaximised,
		fullscreen:   wruntime.WindowFullscreen,
		unfullscreen: wruntime.WindowUnfullscreen,
		unmaximise:   wruntime.WindowUnmaximise,
		maximise:     wruntime.WindowMaximise,
	})
}

func (c *fullscreenController) toggle(ctx context.Context, goos string, ops fullscreenOperations) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Do not use appCtx's background fallback: Wails needs the startup context.
	if ctx == nil {
		return errors.New("fullscreen: the window is not ready")
	}
	if !ops.isFullscreen(ctx) {
		c.restoreMaximised = goos == "windows" && ops.isMaximised(ctx)
		ops.fullscreen(ctx)
		return nil
	}

	ops.unfullscreen(ctx)
	if goos == "windows" && c.restoreMaximised {
		// Wails may still consider the window maximised after unfullscreen.
		// Clear that state first so maximising actually restores its bounds.
		ops.unmaximise(ctx)
		ops.maximise(ctx)
	}
	c.restoreMaximised = false
	return nil
}

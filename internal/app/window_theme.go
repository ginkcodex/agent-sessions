package app

import "errors"

// SetWindowTheme applies this window's native appearance without changing the
// system theme. Unsupported desktop platforms leave their chrome unchanged.
func (a *App) SetWindowTheme(mode string) error {
	switch mode {
	case "system", "light", "dark":
	default:
		return errors.New("window theme: mode must be system, light or dark")
	}
	if fn := desktopFrom(a.ctx).SetWindowTheme; fn != nil {
		return fn(a.ctx, mode)
	}
	return nil
}

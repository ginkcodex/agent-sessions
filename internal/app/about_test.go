package app

import (
	"context"
	"errors"
	"testing"
)

func TestShowAboutOpensTheShellsAboutWindow(t *testing.T) {
	if err := (&App{}).ShowAbout(); !errors.Is(err, ErrDesktopUnavailable) {
		t.Fatalf("show about without a desktop: %v", err)
	}

	shown := 0
	a := &App{ctx: withDesktop(context.Background(), Desktop{ShowAbout: func() { shown++ }})}
	if err := a.ShowAbout(); err != nil || shown != 1 {
		t.Fatalf("ShowAbout = %v, shown %d times", err, shown)
	}
}

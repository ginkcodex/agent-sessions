package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	primary := WindowState{X: 10, Y: 20, Width: 1400, Height: 900, Maximised: true}
	extra := WindowState{X: 60, Y: 70, Width: 1100, Height: 700}

	if err := SaveWindowState(path, RolePrimary, primary); err != nil {
		t.Fatalf("save primary: %v", err)
	}
	if err := SaveWindowState(path, RoleExtra, extra); err != nil {
		t.Fatalf("save extra: %v", err)
	}

	states := LoadWindowStates(path)
	if got := states[RolePrimary]; got != primary {
		t.Errorf("primary = %+v, want %+v", got, primary)
	}
	if got := states[RoleExtra]; got != extra {
		t.Errorf("extra = %+v, want %+v", got, extra)
	}

	// Re-saving one role keeps the other.
	primary.Width = 1234
	if err := SaveWindowState(path, RolePrimary, primary); err != nil {
		t.Fatalf("resave primary: %v", err)
	}
	states = LoadWindowStates(path)
	if states[RolePrimary].Width != 1234 {
		t.Errorf("primary width = %d, want 1234", states[RolePrimary].Width)
	}
	if states[RoleExtra] != extra {
		t.Errorf("extra = %+v, want %+v", states[RoleExtra], extra)
	}
}

func TestWindowStateBadFileAndRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "window-state.json")
	if states := LoadWindowStates(path); len(states) != 0 {
		t.Errorf("missing file gave %v, want none", states)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if states := LoadWindowStates(path); len(states) != 0 {
		t.Errorf("malformed file gave %v, want none", states)
	}
	// Impossible sizes are ignored; valid entries survive.
	if err := os.WriteFile(path, []byte(`{"primary":{"width":-5,"height":100},"extra":{"x":1,"y":2,"width":800,"height":600}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	states := LoadWindowStates(path)
	if _, ok := states[RolePrimary]; ok {
		t.Errorf("negative width was kept: %+v", states[RolePrimary])
	}
	if states[RoleExtra].Width != 800 {
		t.Errorf("extra = %+v, want width 800", states[RoleExtra])
	}
	if err := SaveWindowState("", RolePrimary, WindowState{}); err == nil {
		t.Error("empty path saved without error")
	}
	if err := SaveWindowState(path, "bogus", WindowState{}); err == nil {
		t.Error("unknown role saved without error")
	}
}

func TestDefaultWindowState(t *testing.T) {
	work := Area{X: 0, Y: 0, Width: 2560, Height: 1400}
	got := DefaultWindowState(work)
	if got.Width != DefaultWidth || got.Height != DefaultHeight {
		t.Errorf("big screen got %+v, want %dx%d", got, DefaultWidth, DefaultHeight)
	}
	if got.X != (work.Width-DefaultWidth)/2 || got.Y != (work.Height-DefaultHeight)/2 {
		t.Errorf("big screen not centered: %+v", got)
	}

	small := Area{X: 0, Y: 0, Width: 1600, Height: 1000}
	got = DefaultWindowState(small)
	if got.Width != 1600*9/10 || got.Height != 1000*9/10 {
		t.Errorf("small screen got %+v, want 90%% of the work area", got)
	}

	// A secondary screen's origin shifts the centered window with it.
	shifted := Area{X: 1920, Y: 100, Width: 2560, Height: 1400}
	got = DefaultWindowState(shifted)
	if got.X != 1920+(2560-DefaultWidth)/2 || got.Y != 100+(1400-DefaultHeight)/2 {
		t.Errorf("shifted work area got %+v, want centered inside it", got)
	}

	// No screen information: the plain default at the origin.
	if got := DefaultWindowState(Area{}); got.X != 0 || got.Y != 0 || got.Width != DefaultWidth || got.Height != DefaultHeight {
		t.Errorf("empty work area got %+v, want %dx%d at the origin", got, DefaultWidth, DefaultHeight)
	}
}

func TestCascade(t *testing.T) {
	st := WindowState{X: 100, Y: 100, Width: 1200, Height: 800}
	if got := Cascade(st, 0); got != st {
		t.Errorf("step 0 = %+v, want unchanged", got)
	}
	got := Cascade(st, 2)
	if got.X != 100+2*CascadeOffset || got.Y != 100+2*CascadeOffset {
		t.Errorf("step 2 = %+v, want shifted by 2 offsets", got)
	}
	if got.Width != st.Width || got.Height != st.Height {
		t.Errorf("cascade changed the size: %+v", got)
	}
}

func TestClampToWorkArea(t *testing.T) {
	work := Area{X: 0, Y: 0, Width: 1920, Height: 1080}
	const minW, minH = 960, 600

	// A fitting window keeps its geometry.
	got := ClampToWorkArea(WindowState{X: 100, Y: 100, Width: 1200, Height: 800}, work, minW, minH)
	if got.X != 100 || got.Y != 100 || got.Width != 1200 || got.Height != 800 {
		t.Errorf("fitting window moved: %+v", got)
	}

	// A window on a removed screen is centered in the work area.
	got = ClampToWorkArea(WindowState{X: 3000, Y: 50, Width: 1200, Height: 800}, work, minW, minH)
	if got.X != (1920-1200)/2 || got.Y != (1080-800)/2 {
		t.Errorf("off-screen window got %+v, want centered", got)
	}

	// A window hanging off the edge shifts back in by the least amount.
	got = ClampToWorkArea(WindowState{X: 1500, Y: 900, Width: 1200, Height: 800}, work, minW, minH)
	if got.X != 1920-1200 || got.Y != 1080-800 {
		t.Errorf("edge window got %+v, want shifted into view", got)
	}

	// A remembered size below the minimum grows to it; a huge one shrinks to
	// the work area; the maximised flag survives.
	got = ClampToWorkArea(WindowState{Width: 500, Height: 300, Maximised: true}, work, minW, minH)
	if got.Width != minW || got.Height != minH || !got.Maximised {
		t.Errorf("small window got %+v, want %dx%d maximised", got, minW, minH)
	}
	got = ClampToWorkArea(WindowState{Width: 4000, Height: 3000}, work, minW, minH)
	if got.Width != work.Width || got.Height != work.Height {
		t.Errorf("huge window got %+v, want the work area size", got)
	}
}

func TestVisibleEnough(t *testing.T) {
	work := Area{X: 0, Y: 0, Width: 1920, Height: 1080}
	if !VisibleEnough(WindowState{X: 100, Y: 100, Width: 1200, Height: 800}, work) {
		t.Error("window inside the work area reported invisible")
	}
	// Far away on a removed screen.
	if VisibleEnough(WindowState{X: 3000, Y: 100, Width: 1200, Height: 800}, work) {
		t.Error("removed-screen window reported visible")
	}
	// Title bar above the work area: not grabbable.
	if VisibleEnough(WindowState{X: 100, Y: -100, Width: 1200, Height: 800}, work) {
		t.Error("window with its title bar off-screen reported visible")
	}
	// A sliver at the edge is not usable.
	if VisibleEnough(WindowState{X: 1850, Y: 100, Width: 1200, Height: 800}, work) {
		t.Error("edge sliver reported visible")
	}
	// A secondary screen's work area makes the same window visible again.
	second := Area{X: 1920, Y: 0, Width: 1920, Height: 1080}
	if !VisibleEnough(WindowState{X: 3000, Y: 100, Width: 1200, Height: 800}, second) {
		t.Error("window on the second screen reported invisible")
	}
	if VisibleEnough(WindowState{}, work) {
		t.Error("zero-sized window reported visible")
	}
}

func TestMaximiseBeforeShow(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want bool
	}{
		{name: "windows", goos: "windows", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}},
		{name: "macos", goos: "darwin"},
		{name: "wayland session", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, want: true},
		{name: "x11 session", goos: "linux", env: map[string]string{"DISPLAY": ":0"}},
		{name: "forced x11 on wayland", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "x11"}},
		{name: "x11 first in list", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "x11,wayland"}},
		{name: "wayland first in list", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": " wayland ,x11"}, want: true},
		{name: "any backend", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "*"}, want: true},
		{name: "any backend without wayland", goos: "linux", env: map[string]string{"GDK_BACKEND": "*", "DISPLAY": ":0"}},
		{name: "broadway", goos: "linux", env: map[string]string{"WAYLAND_DISPLAY": "wayland-0", "GDK_BACKEND": "broadway"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string { return tt.env[k] }
			if got := MaximiseBeforeShow(tt.goos, getenv); got != tt.want {
				t.Errorf("MaximiseBeforeShow(%q, %v) = %v, want %v", tt.goos, tt.env, got, tt.want)
			}
		})
	}
}

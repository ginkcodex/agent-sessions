package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ginkcode/agent-sessions/internal/configfile"
	"github.com/ginkcode/agent-sessions/internal/paths"
)

// WindowRole distinguishes the windows whose geometry is remembered. Window
// names are per-run, so geometry is remembered by role: the first window of a
// run is the primary window, windows opened later are extra windows.
type WindowRole string

const (
	RolePrimary WindowRole = "primary"
	RoleExtra   WindowRole = "extra"
)

// WindowState is a window's remembered geometry: its normal (un-maximised)
// bounds in CSS pixels and whether it was maximised. Fullscreen is not
// remembered; the window comes back windowed or maximised.
type WindowState struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised,omitempty"`
}

// Area is a rectangle in CSS pixels, the frame of a screen work area.
type Area struct {
	X      int
	Y      int
	Width  int
	Height int
}

// First-run window size: the smallest window where the sidebar, session list
// and transcript all show their content, held to 90% of a smaller screen.
const (
	DefaultWidth  = 1500
	DefaultHeight = 920
	// CascadeOffset separates two windows' title bars when New Window has no
	// remembered geometry of its own.
	CascadeOffset = 32
)

// savedExtent bounds a remembered size or position; anything beyond is
// treated as a corrupt file rather than a screen far away.
const savedExtent = 100000

// WindowStatePath is where geometry is remembered: window-state.json in the
// user's config dir, beside config.toml.
func WindowStatePath() (string, error) {
	r, err := paths.Default()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(r.Config, "window-state.json"), nil
}

// windowStateFile is the on-disk shape: one remembered geometry per role.
type windowStateFile struct {
	Primary *WindowState `json:"primary,omitempty"`
	Extra   *WindowState `json:"extra,omitempty"`
}

// LoadWindowStates reads the remembered geometries in path. A missing,
// unreadable or malformed file and entries with impossible sizes all yield no
// state for that role: the window then opens at the first-run default.
func LoadWindowStates(path string) map[WindowRole]WindowState {
	out := map[WindowRole]WindowState{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var f windowStateFile
	if err := json.Unmarshal(data, &f); err != nil {
		return out
	}
	if f.Primary != nil && validState(*f.Primary) {
		out[RolePrimary] = *f.Primary
	}
	if f.Extra != nil && validState(*f.Extra) {
		out[RoleExtra] = *f.Extra
	}
	return out
}

// SaveWindowState remembers st for role in path and keeps the other role's
// state. Windows close concurrently, so the write runs under the config lock.
func SaveWindowState(path string, role WindowRole, st WindowState) error {
	if path == "" {
		return fmt.Errorf("empty window state path")
	}
	if role != RolePrimary && role != RoleExtra {
		return fmt.Errorf("unknown window role %q", role)
	}
	return configfile.Update(path, func(data []byte) []byte {
		var f windowStateFile
		_ = json.Unmarshal(data, &f)
		saved := st
		switch role {
		case RolePrimary:
			f.Primary = &saved
		case RoleExtra:
			f.Extra = &saved
		}
		out, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			return data
		}
		return append(out, '\n')
	})
}

// MaximiseBeforeShow reports whether a window closed maximised should be
// created maximised (Wails' StartState) rather than maximised once shown.
// Only GTK on Wayland takes it: the compositor maps the window maximised from
// its first frame and un-maximises to the saved size. Elsewhere the early
// maximise breaks the window:
//
//   - Windows and macOS: Wails maximises before it moves the new window to
//     its position, which shifts the maximised window off its screen.
//   - GTK on X11: the window maps at its minimum size and un-maximising
//     returns to that instead of the saved size.
//
// macOS and Windows maximise just before the first show instead, once the
// window is at its position; X11 after the first show, a brief normal frame
// first. getenv reads the environment (os.Getenv).
func MaximiseBeforeShow(goos string, getenv func(string) string) bool {
	if goos != "linux" {
		return false
	}
	return gdkUsesWayland(getenv)
}

// gdkUsesWayland reports whether GDK picks its Wayland backend: the first
// entry of GDK_BACKEND when that names one, otherwise whether a Wayland
// display is there to connect to, the backend GDK tries first.
func gdkUsesWayland(getenv func(string) string) bool {
	if backend := getenv("GDK_BACKEND"); backend != "" {
		first := strings.TrimSpace(strings.SplitN(backend, ",", 2)[0])
		switch first {
		case "wayland":
			return true
		case "*", "":
		default:
			return false
		}
	}
	return getenv("WAYLAND_DISPLAY") != ""
}

// DefaultWindowState sizes the first window: DefaultWidth by DefaultHeight,
// capped at 90% of work and centered in it. With no work area known it falls
// back to the plain default size at the origin.
func DefaultWindowState(work Area) WindowState {
	st := WindowState{Width: DefaultWidth, Height: DefaultHeight}
	if work.Width <= 0 || work.Height <= 0 {
		return st
	}
	if cap := work.Width * 9 / 10; cap < st.Width {
		st.Width = cap
	}
	if cap := work.Height * 9 / 10; cap < st.Height {
		st.Height = cap
	}
	if st.Width < 1 {
		st.Width = 1
	}
	if st.Height < 1 {
		st.Height = 1
	}
	st.X = work.X + (work.Width-st.Width)/2
	st.Y = work.Y + (work.Height-st.Height)/2
	return st
}

// Cascade shifts st by n steps down and right, so a New Window without a
// remembered geometry still shows its title bar beside the opener's.
func Cascade(st WindowState, n int) WindowState {
	if n <= 0 {
		return st
	}
	st.X += CascadeOffset * n
	st.Y += CascadeOffset * n
	return st
}

// ClampToWorkArea moves and shrinks st so the window opens fully inside work:
// a window that still fits keeps its size, one not overlapping work at all is
// centered in it, and one partly outside is shifted back in by the least
// amount. minW and minH win over a smaller remembered size.
func ClampToWorkArea(st WindowState, work Area, minW, minH int) WindowState {
	w := st.Width
	if w < minW {
		w = minW
	}
	if w > work.Width {
		w = work.Width
	}
	h := st.Height
	if h < minH {
		h = minH
	}
	if h > work.Height {
		h = work.Height
	}
	out := WindowState{X: st.X, Y: st.Y, Width: w, Height: h, Maximised: st.Maximised}
	if !overlaps(st, work) {
		out.X = work.X + (work.Width-w)/2
		out.Y = work.Y + (work.Height-h)/2
		return out
	}
	out.X = clampInt(st.X, work.X, work.X+work.Width-w)
	out.Y = clampInt(st.Y, work.Y, work.Y+work.Height-h)
	return out
}

// VisibleEnough reports whether enough of st sits in work to see and grab:
// the title bar on screen and a usable corner of the window. A window failing
// this test was on a screen that is gone and gets clamped back into view.
func VisibleEnough(st WindowState, work Area) bool {
	if st.Width <= 0 || st.Height <= 0 || work.Width <= 0 || work.Height <= 0 {
		return false
	}
	// The title bar must sit inside the work area for the user to grab it.
	const titleBar = 40
	if st.Y < work.Y || st.Y > work.Y+work.Height-titleBar {
		return false
	}
	const (
		minVisibleWidth  = 240
		minVisibleHeight = 96
	)
	x0 := maxInt(st.X, work.X)
	x1 := minInt(st.X+st.Width, work.X+work.Width)
	y0 := maxInt(st.Y, work.Y)
	y1 := minInt(st.Y+st.Height, work.Y+work.Height)
	return x1-x0 >= minVisibleWidth && y1-y0 >= minVisibleHeight
}

func validState(st WindowState) bool {
	return st.Width > 0 && st.Height > 0 &&
		st.Width <= savedExtent && st.Height <= savedExtent &&
		absInt(st.X) <= savedExtent && absInt(st.Y) <= savedExtent
}

func overlaps(st WindowState, work Area) bool {
	return st.X < work.X+work.Width && work.X < st.X+st.Width &&
		st.Y < work.Y+work.Height && work.Y < st.Y+st.Height
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

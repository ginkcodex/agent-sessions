//go:build desktop

package main

import (
	"context"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/ginkcode/agent-sessions/frontend"
	"github.com/ginkcode/agent-sessions/internal/app"
)

// shell opens the app's windows and connects each one to its own App in
// app.Windows.
type shell struct {
	desktop *application.App
	windows *app.Windows

	mu sync.Mutex
	// running is set once the application has started. Windows opened
	// before then wait in pending and start with it.
	running bool
	pending []shellWindow
	// opened counts the windows opened this run: the first is the primary
	// window and remembers its own geometry, later ones are extras.
	opened int
	// about is the open About window, if any.
	aboutMu sync.Mutex
	about   *application.WebviewWindow
}

type shellWindow struct {
	name   string
	window *application.WebviewWindow
}

// callingWindow names the window that made a bound call; Wails puts it in
// the call's context.
func callingWindow(ctx context.Context) string {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok && w != nil {
		return w.Name()
	}
	return ""
}

// openWindow opens a window with an App of its own.
func (s *shell) openWindow() error {
	name := s.windows.Reserve()
	// The first window of a run is the primary one and remembers its own
	// geometry; windows opened later are extras and each remembers theirs.
	role := app.RoleExtra
	s.mu.Lock()
	if s.opened == 0 {
		role = app.RolePrimary
	}
	step := s.opened
	s.opened++
	s.mu.Unlock()
	state, positioned := initialState(role, step)

	opts := application.WebviewWindowOptions{
		Name:      name,
		Title:     "Agent Sessions",
		Width:     state.Width,
		Height:    state.Height,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
		// Files dropped on the terminal panel arrive as paths. Only elements
		// marked data-file-drop-target receive them.
		EnableFileDrop: true,
		Mac: application.MacWindow{
			// Escape belongs to in-app dialogs, not to fullscreen.
			DisableEscapeExitsFullscreen: true,
		},
		Linux: application.LinuxWindow{
			Icon:             frontend.AppIcon,
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	}
	if positioned {
		opts.InitialPosition = application.WindowXY
		opts.X = state.X
		opts.Y = state.Y
	} else {
		opts.InitialPosition = application.WindowCentered
	}
	// Only Wayland takes the maximise as part of the window's creation; the
	// other platforms ask for it once the window is shown (see
	// app.MaximiseBeforeShow and the WindowShow listener below).
	preShowMaximise := state.Maximised && app.MaximiseBeforeShow(runtime.GOOS, os.Getenv)
	if preShowMaximise {
		opts.StartState = application.WindowStateMaximised
	}
	window := s.desktop.Window.NewWithOptions(opts)

	// Remember the geometry the user settles on: as it changes, and once
	// more when the window closes, quit included (shutting down closes every
	// window through the closing hook). Moves and resizes keep the normal
	// bounds fresh, so closing maximised saves the size the window had
	// before it was maximised.
	statePath := ""
	if p, err := app.WindowStatePath(); err == nil {
		statePath = p
	}
	var mu sync.Mutex
	normal := state
	// The maximised flag follows the window's own events: a minimised or
	// already destroyed window does not report it at close time, and losing
	// the flag would grow the window by a state every run.
	maximised := state.Maximised
	refreshNormal := func() {
		if window.IsMinimised() || window.IsMaximised() || window.IsFullscreen() {
			return
		}
		b := window.Bounds()
		// A closed window reports empty bounds; keep the last real ones
		// instead of remembering a zero-sized window.
		if b.Width <= 0 || b.Height <= 0 {
			return
		}
		normal = app.WindowState{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height}
	}
	remember := func() {
		if statePath == "" {
			return
		}
		mu.Lock()
		saved := normal
		wasMaximised := maximised
		mu.Unlock()
		saved.Maximised = window.IsMaximised() || (window.IsMinimised() && wasMaximised)
		_ = app.SaveWindowState(statePath, role, saved)
	}
	track := func(*application.WindowEvent) {
		mu.Lock()
		refreshNormal()
		mu.Unlock()
		remember()
	}
	window.OnWindowEvent(events.Common.WindowDidMove, track)
	window.OnWindowEvent(events.Common.WindowDidResize, track)
	window.OnWindowEvent(events.Common.WindowMaximise, func(*application.WindowEvent) {
		mu.Lock()
		maximised = true
		mu.Unlock()
		remember()
	})
	window.OnWindowEvent(events.Common.WindowUnMaximise, func(*application.WindowEvent) {
		mu.Lock()
		maximised = false
		mu.Unlock()
		remember()
	})
	window.OnWindowEvent(events.Common.WindowRestore, func(*application.WindowEvent) {
		mu.Lock()
		maximised = window.IsMaximised()
		mu.Unlock()
		remember()
	})

	// Once the window is first shown, two things the creation code could not
	// do when it ran blind (the first window is made before the application
	// runs, when Wails has no screen information): check the geometry
	// against the real screens — a window whose screen is gone comes back
	// into view instead of opening off-screen — and maximise a window that
	// was closed maximised, unless Wayland already did it at creation. Both
	// are no-ops when nothing is wrong.
	//
	// Only the first show counts. On Linux, WindowShow is the webview's
	// load-finished event and fires again on every reload; on Windows it
	// fires on every hide/show. Running this again would maximise a window
	// the user has un-maximised since.
	var firstShow sync.Once
	window.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		firstShow.Do(func() {
			maximise := func() {
				if state.Maximised && !window.IsMaximised() {
					window.Maximise()
				}
			}
			b := window.Bounds()
			if !positioned || b.Width <= 0 || b.Height <= 0 || window.IsMaximised() || window.IsFullscreen() {
				maximise()
				return
			}
			want := app.WindowState{X: state.X, Y: state.Y, Width: b.Width, Height: b.Height}
			clamped := false
			if screen := application.ScreenNearestDipRect(savedRect(want)); screen != nil {
				work := screenArea(screen)
				if !app.VisibleEnough(want, work) {
					want = app.ClampToWorkArea(want, work, windowMinWidth, windowMinHeight)
					clamped = true
				}
			}
			// Programmatic moves report back no event, so the placement
			// target is the normal bounds here.
			mu.Lock()
			normal = want
			mu.Unlock()
			if runtime.GOOS != "linux" {
				// Windows and macOS created the window at exactly these
				// bounds; only a clamp has to move it. The maximise follows
				// the placement, so un-maximising returns to it.
				if clamped {
					window.SetBounds(savedRect(want))
				}
				maximise()
				return
			}
			if clamped {
				window.SetSize(want.Width, want.Height)
			}
			// GTK/X11 lands moves a frame margin off (see place). The
			// maximise waits for the placement to settle: on un-maximise
			// the window manager brings back the geometry it saw when the
			// maximise happened.
			place(window, want.X, want.Y, maximise)
		})
	})

	// The hook runs before the window closes and may keep it open; the
	// listener runs only once the window really closes. Only the hook saves:
	// it runs while the window is alive, and a save after the destroy would
	// record empty bounds over the good ones.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		mu.Lock()
		refreshNormal()
		mu.Unlock()
		remember()
		if s.windows.BeforeClose(name) {
			event.Cancel()
		}
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		s.windows.Closed(name)
	})

	// Wails hands dropped paths only to Go; pass them on to the page, which
	// works out the terminal under the CSS-pixel drop point.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		files := event.Context().DroppedFiles()
		target := event.Context().DropTargetDetails()
		if len(files) == 0 || target == nil {
			return
		}
		window.DispatchWailsEvent(&application.CustomEvent{
			Name: filesDroppedEvent,
			Data: map[string]any{"filenames": files, "x": target.X, "y": target.Y},
		})
	})

	// Native integration needs the window to exist: Linux hooks mouse side
	// buttons per webview; macOS tests an opaque backing for this solid page.
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		configureWindowOpacity(window)
		hookWindowMouseNavigation(window)
	})

	w := shellWindow{name: name, window: window}
	s.mu.Lock()
	running := s.running
	if !running {
		s.pending = append(s.pending, w)
	}
	s.mu.Unlock()
	if running {
		go s.startWindow(w)
	}
	return nil
}

// start starts the windows opened before the application ran. The shell
// calls it once the application has started.
func (s *shell) start() {
	s.mu.Lock()
	s.running = true
	pending := s.pending
	s.pending = nil
	s.mu.Unlock()
	for _, w := range pending {
		s.startWindow(w)
	}
}

func (s *shell) startWindow(w shellWindow) {
	desktop := newDesktop(s.desktop, w.window)
	desktop.ShowAbout = s.showAbout
	s.windows.Start(w.name, s.desktop.Context(), desktop)
}

// Size floors for remembered geometry, the same as the window options.
const (
	windowMinWidth  = 960
	windowMinHeight = 600
)

// initialState is the geometry for a new window of role: the remembered
// bounds while enough of them sits on a screen to see and grab, the same
// bounds pulled back into view when the screen is gone, and the first-run
// default otherwise. positioned is false only when no screen is known,
// leaving the placement to Wails.
func initialState(role app.WindowRole, step int) (app.WindowState, bool) {
	if path, err := app.WindowStatePath(); err == nil {
		if saved, ok := app.LoadWindowStates(path)[role]; ok {
			screen := application.ScreenNearestDipRect(savedRect(saved))
			if screen == nil {
				// No screen information at all (the first window is made
				// before the application runs): keep the saved geometry
				// whole, maximised flag included. The WindowShow listener
				// checks it against the real screens once they exist.
				return saved, true
			}
			work := screenArea(screen)
			if app.VisibleEnough(saved, work) {
				return saved, true
			}
			return app.ClampToWorkArea(saved, work, windowMinWidth, windowMinHeight), true
		}
	}
	work, ok := primaryWorkArea()
	st := app.DefaultWindowState(work)
	if !ok {
		return app.WindowState{Width: st.Width, Height: st.Height}, false
	}
	if role == app.RoleExtra {
		st = app.Cascade(st, step)
	}
	return st, true
}

// primaryWorkArea is the work area of the primary screen, if Wails reports one.
func primaryWorkArea() (app.Area, bool) {
	screen := application.GetScreenByIndex(0)
	if screen == nil {
		return app.Area{}, false
	}
	return screenArea(screen), true
}

func screenArea(screen *application.Screen) app.Area {
	work := screen.WorkArea
	return app.Area{X: work.X, Y: work.Y, Width: work.Width, Height: work.Height}
}

func savedRect(st app.WindowState) application.Rect {
	return application.Rect{X: st.X, Y: st.Y, Width: st.Width, Height: st.Height}
}

// place positions the window so the bounds it reports start at x, y, then
// calls after. The position a window reports is not always the one its move
// accepts: on GTK/X11 the window manager reads a move as the frame's
// position and reports the client's, a title bar apart, so a restored
// window would walk down the screen on every run. A move lands
// asynchronously and at a constant offset, so once things settle, read
// where the window actually landed and ask again for the difference: the
// second ask's landing spot is x, y exactly — one corner for both the save
// and the restore.
func place(window *application.WebviewWindow, x, y int, after func()) {
	window.SetPosition(x, y)
	go func() {
		time.Sleep(placeSettle)
		got := window.Bounds()
		// A platform that cannot report or move windows (Wayland) reads a
		// constant 0,0: there the compositor places the window, not us.
		// The landing spot also only means anything while the window is
		// still in the state it was placed in: a maximise in the meantime
		// reports the maximised bounds instead.
		movable := (got.X != 0 || got.Y != 0) &&
			!window.IsMaximised() && !window.IsMinimised() && !window.IsFullscreen()
		// Only a frame margin may ever sit between the two. A larger gap
		// means the move is being re-interpreted or ignored, and a second
		// one would only fight the window manager.
		const maxMargin = 256
		near := got.X-x <= maxMargin && x-got.X <= maxMargin &&
			got.Y-y <= maxMargin && y-got.Y <= maxMargin
		if movable && near && (got.X != x || got.Y != y) {
			cx, cy := x-(got.X-x), y-(got.Y-y)
			window.SetPosition(cx, cy)
			// Let the correction land first: after may maximise, and the
			// window manager remembers the geometry at that moment.
			time.Sleep(placeSettle)
		}
		if after != nil {
			after()
		}
	}()
}

// placeSettle is how long place lets a move land before reading the landing
// spot back. The landing takes well under 100ms on GTK/X11; the wait is
// generous because a read that is early just misses the correction, and the
// saved and restored corners stay the same regardless.
const placeSettle = 300 * time.Millisecond

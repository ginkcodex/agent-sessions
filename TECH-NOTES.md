# Technical notes

Platform quirks and pitfalls that cost time to find. Check here before
changing the affected code.

## Scan checkpoint versions

Each provider saves a checkpoint per source in the index so the next scan
can skip what it already read. These versions are **ours**: the format of
what we cache, not the agent's app version or database layout.

| Provider | Constant | Checkpoint |
|---|---|---|
| Claude Code | `checkpointVersion` in `internal/provider/claude/scan.go` | per session file |
| Codex | `checkpointVersion` in `internal/provider/codex/scan_rollout.go` | per rollout file |
| OpenCode | `dbCheckpointVersion` in `internal/provider/opencode/scan.go` | one for `opencode.db`: every session's metadata plus an `updated_at` cursor |

Raise the version whenever cached metadata gains a field or changes meaning.
Incremental scans never re-read what the checkpoint already covers (an
OpenCode session older than the cursor, a Claude or Codex file whose size and
mtime are unchanged), so without a bump old sessions never get the new field.

**Rule: a checkpoint of any other version, older or newer, is rescanned in
full and rewritten. Never reject it.** Builds share one index (the
`release` app and a dev build on the same machine), so each will read
checkpoints the other wrote. A rejected checkpoint is never replaced,
because only a successful scan writes a new one, so the provider fails on
every scan and restart. Only checkpoints that can't be trusted are rejected:
corrupt JSON, or invalid IDs and generations.

This happened with OpenCode. `eb837bc` raised the version from 4 to 5, but
`decodeCheckpoint` upgraded only a fixed list of old versions (2 and 3), so
the release build's version 4 was rejected as "invalid v2 checkpoint". ("v2"
there means OpenCode's database layout, not our version.) OpenCode sessions
stopped updating and the catalog showed "Discovery incomplete" until the
rule above replaced the list (`cfbb6a4`). `TestScanRescansCheckpointOfOtherVersion`
covers every version from 2 up to the current one plus the next, so a bump is
tested without editing the test.

The index schema is separate: a database migrated by a newer build is refused
(`errNewerVersionDB`), and the engine falls back to an uncached in-memory
scan.

To find out why discovery is degraded, hover over "Discovery incomplete": the
tooltip lists each failed provider's error (`providerErrors` in the discovery
status). To reproduce a failure, open the index with `index.OpenReadOnly`,
load the saved states with `LoadSnapshot` and run each provider's `Scan` with
its state. This reads the live index in place, so nothing needs to be copied.

## Linux rendering (DMA-BUF)

**Don't set `WEBKIT_DISABLE_DMABUF_RENDERER=1` for everyone.** Without
DMA-BUF, WebKitGTK hands every frame to the UI process in shared memory, and
the GTK main thread copies and composites it with cairo/pixman. Any
animation (a live session's pulse, a spinner) then keeps that thread busy.
On a 1.5× GNOME Wayland laptop with an AMD GPU it sat at 100 % while idle,
and the app stuttered. With DMA-BUF it idles around 10 %.

The app used to force it from the Wails v2 days (blank windows on NVIDIA).
Wails v3 now does it itself, only where WebKit renders on NVIDIA
(`isNVIDIAGPU` in `wails/v3/pkg/application/gpu_linux.go`), before the app's
own `init()`. A value the user exports wins either way: `=1` if some driver
shows a blank window, `=0` to try DMA-BUF on NVIDIA. See
[docs/plan/linux-gpu-rendering.md](docs/plan/linux-gpu-rendering.md).

To see where the main thread goes when `perf` and `gdb -p` need root
(`perf_event_paranoid` 4, `ptrace_scope` 1), start a copy under `gdb`
(`gdb -batch -ex run -ex 'thread apply 1 bt' … --args <binary>`) and send
SIGINT to that child only, never to a running app picked by name.

## Fullscreen shortcut

Control+Command+F toggles fullscreen on macOS; other platforms use F11
(`isFullscreenShortcut` in `frontend/src/lib/fullscreen.ts`). Reserve
Control+Shift+F for the planned Go to File directory-content search.

In the desktop app the shortcut calls the `ToggleFullscreen` App binding
(`internal/app/fullscreen.go`), which reads the window's real state through
the `app.Desktop` callbacks before entering or leaving fullscreen. On Windows,
leaving fullscreen restores a window that was maximised before. Toggles are
serialised, and the binding fails with `ErrDesktopUnavailable` instead of
falling back to the browser Fullscreen API.

Escape deliberately does **not** leave fullscreen in the desktop app, because
dialogs, find bars and the terminal use it. The window sets
`DisableEscapeExitsFullscreen` on macOS, and `preventDesktopEscapeFullscreen`
cancels Escape's default action in the capture phase without stopping it.
Handlers that need to know whether an earlier one used the key must call
`keyHandled`, not read `defaultPrevented`, which the safeguard always sets.

History: under Wails v2, Control+Command+F entered fullscreen but pressing it
again did not leave it. After the Wails v3 upgrade the user verified on a
macOS desktop build that fullscreen enters and exits. F11 on Linux and
Windows, and maximised-window restore on Windows, have not been tested on
real machines (see [docs/plan/wails-v3-upgrade.md](docs/plan/wails-v3-upgrade.md)).
Unit tests cover the frontend and toggle logic, not native shortcut delivery.

## Mouse side buttons (back / forward)

The app moves back and forward between sessions with the mouse's side
buttons. In the DOM these are `MouseEvent.button` 3 (back) and 4 (forward),
but only some webviews report them that way, so each platform takes a
different path. All paths end in the same navigation as Alt+←/→ and ⌘[ / ⌘].

| Platform | Webview | What the page sees | How the app handles it |
|---|---|---|---|
| Windows | WebView2 | `mousedown`/`mouseup` with button 3/4 | DOM handler in `frontend/src/App.svelte` (`onMouseButton`, `navigationButton` in `frontend/src/lib/navHistory.ts`). Not yet confirmed on real hardware. |
| Linux | WebKitGTK 6.0 (GTK 4) | Under WebKit2GTK (GTK 3, Wails v2): a **left** click (`button=0`, `buttons=0`, `which=0`) plus `auxclick`, identical for back and forward. Not re-checked under GTK 4. | GTK 4 hook in `cmd/agent-sessions/mousenav_linux.c` claims buttons 8/9 in the capture phase before WebKit sees them. Not yet tested on real hardware. |
| macOS | WKWebView | Nothing usable | `NSEvent` local monitor in `cmd/agent-sessions/mousenav_darwin.m` |

The native hooks send the Wails event `nav:mouse` with `"back"` or
`"forward"` (`mouseNavigateEvent` in `cmd/agent-sessions/mousenav.go`) to
the window the button was pressed in, and only to that window. macOS has one
monitor for every window; Linux hooks each window's web view once its page
is ready.
The frontend subscribes through `subscribeMouseNavigate` in
`frontend/src/lib/api.ts`. The shared Go side is
`cmd/agent-sessions/mousenav_native.go`.

Go to File uses the same buttons for the editor's back and forward through
files and places (`sideButton` in
`frontend/src/lib/components/common/GoToFileDialog.svelte`, history in
`FilesStore`, per workspace). It listens to both paths, on `mouseup` and
`nav:mouse`, only while it is the one open modal (`onlyOpenModal` in
`frontend/src/lib/search.ts`). App keeps cancelling the webview's own
navigation on every side button and ignores them while any modal is open,
so one press moves either the session list or the editor, never both.

### Linux

- GDK numbers the side buttons 8 (back) and 9 (forward), not 4/5. GDK's
  4–7 are the scroll wheel.
- WebKit2GTK under GTK 3 passed them to the page as a left-button press, so
  without the hook a side button also acted like a left click in the page.
  The GTK 4 hook is a `GtkGestureClick` for all buttons in the capture
  phase; it claims the sequence for buttons 8 and 9 on press and release and
  navigates on release.
- The hook is attached to the `WebKitWebView` found by walking
  `gtk_window_get_toplevels()` and each widget's children, retried every
  100 ms (up to 100 tries) until Wails has created it.
- cgo: a Go file with `//export` can't define C functions in its preamble,
  so the C code lives in a `.c` file. Go honours its `//go:build` line.
- **Two-finger swipes** are scrolls in GTK. The same web view also gets a
  `GtkEventControllerScroll` (capture phase) that always returns `FALSE`,
  so WebKit still scrolls. Only `GDK_SOURCE_TOUCHPAD` counts. A gesture runs
  from `scroll-begin` to `scroll-end`, which GTK sends when the fingers lift
  (Wayland). Until a `scroll-end` has come (X11 may send none), a scroll
  after 200 ms also starts one; after that the gap is ignored, so fingers
  resting mid-swipe do not go a second step. Past `SWIPE_DISTANCE` (80 px; X11 wheel
  units × 40) sideways, more than twice the vertical, it sends `nav:swipe`
  once.
- **The controller sees each scroll twice** (SN-07). The copy has the same
  time and deltas and comes later, sometimes after the swipe's
  `scroll-end`, probably WebKit handing back what the page did not use.
  GTK even emits `scroll-begin` for it. Counted, the copies doubled every
  swipe's distance and, once they came after the end, sent a second step.
  So any scroll, begin or end no newer than the newest seen is skipped.
- `AGENT_SESSIONS_TRACE_NAV=1` prints each gesture's begin, end, scrolls
  (time, deltas, unit, device source) and sends to stderr, and each event
  Go sends to a window. Run the app from a terminal with it to see what a
  touchpad really sends. It follows the scroll, as Chrome does: toward the left is back,
  which is fingers right with natural scrolling (GNOME's and Plasma 6's
  touchpad default). GNOME has no *Swipe between pages* setting to honour.

### macOS

- Without a mouse utility, `NSEvent` reports the side buttons as
  `NSEventTypeOtherMouseDown`/`Up` with `buttonNumber` 3 (back) and 4
  (forward).
- **Logi Options+ never sends these.** It turns each side-button press into
  a horizontal **swipe** (`NSEventTypeSwipe`), the same event as a
  three-finger trackpad swipe. Each press arrives as a swipe with
  `deltaX = 0`, then one with `deltaX = +1` (back) or `-1` (forward). This
  matches Safari's convention.
- So the monitor handles both: buttons 3/4, and swipes with a non-zero
  `deltaX`. Zero-delta swipes are passed on. A three-finger trackpad swipe
  sends the same swipe event only when *Trackpad → More Gestures → Swipe
  between pages* is set to three fingers. By default macOS keeps three
  fingers for its own gestures (spaces, Mission Control), and the app sees
  nothing.
- **Two-finger swipes** (the default *Swipe between pages*, and a Magic
  Mouse's one-finger swipe) are no swipe event: they arrive as scroll
  events with a gesture phase, and apps such as Safari recognise the page
  swipe themselves. The monitor watches `NSEventTypeScrollWheel` too and
  always passes it on. It adds up each gesture's movement, ignoring wheels
  and momentum (no phase). Past `SWIPE_DISTANCE` sideways, and more than
  twice the vertical, it sends `nav:swipe` once, when
  `isSwipeTrackingFromScrollEventsEnabled`. Right is back, whatever the
  scroll direction setting. WKWebView's own
  `allowsBackForwardNavigationGestures` stays off: the app is one page.
- A two-finger swipe is also a scroll, so the frontend drops `nav:swipe`
  when the same gesture could move something sideways, such as a long code
  line or a wide transcript block (`frontend/src/lib/swipeNavigation.ts`).
  Otherwise `subscribeMouseNavigate` delivers it like a button.
- Other mouse utilities (SteerMouse, BetterMouse, Mac Mouse Fix,
  SensibleSideButtons) may remap the buttons differently again, for example
  to ⌘[ / ⌘] keystrokes, which the keyboard shortcuts already cover.

### Windows

- WebView2 should report buttons 3/4 to the page like Chromium, and the
  DOM handler calls `preventDefault` so WebView2 doesn't try its own page
  navigation. This still needs testing on a real machine.
- Logitech's Windows software may remap the buttons too; check what the
  app receives before assuming the handler is broken.
- **Two-finger swipes:** WebView2 runs in its own process, so the app's
  window never sees its scroll messages, and its own swipe navigation is
  for touchscreens. The page spots the swipe from wheel events instead
  (`SwipeGuard` with detect on, `frontend/src/lib/swipeNavigation.ts`): at
  least 3 pixel-mode events, 80 px sideways and more than twice the
  vertical, with no Shift and no delta of a whole wheel step (100 px).
  Toward the left is back, as in Edge. A smooth sideways mouse wheel can
  pass for a touchpad.
- Without a phase, the wheel stream does not say where one swipe ends: the
  momentum after the fingers lift, or a quick lift and touch, runs straight
  into the next swipe. The guard splits the stream into strokes: a new one
  starts only after the fingers let go (the speed fell to a quarter of its
  peak through three falling events in a row) and then rises for two
  events. Momentum only slows down, so it never starts one. Each stroke
  swipes once and has its own scrolled flag.
- **Don't loosen the let-go rule to "half speed, then a rise"** (SN-05). A
  finger's speed dips that much within one swipe, so one swipe went several
  steps (SN-06). Missing a step in a very fast double swipe is the lesser
  fault.

### How to debug

The page often can't tell you what happened, because the event never
reaches it. Log at the native layer instead:

- **macOS:** add a second `NSEvent` local monitor with `NSEventMaskAny`
  that `NSLog`s each event's `type`, `buttonNumber`, `deltaX` and
  `keyCode` (skip mouse moves and drags), build with `make app` (a plain
  `go build -tags desktop,production`), run the binary from a terminal and
  press the buttons. Use the binary inside the `.app` from
  `make package-macos` when the bundle matters.
- **Synthetic presses on macOS:** a small Swift tool that posts
  `CGEvent(mouseType: .otherMouseDown/.otherMouseUp, mouseButton: 3 or 4)`
  with `postToPid` tests the button path. It doesn't reproduce what a
  remapping utility sends, so always also test with the real mouse.
- **Linux:** under GTK 3, a headless WebKitGTK probe synthesised
  `GdkEventButton` with `gtk_main_do_event` and logged what the page
  received (for example under `broadwayd` with `GDK_BACKEND=broadway`).
  GTK 4 has no public way to synthesise input events, so test the GTK 4
  hook with a real mouse.

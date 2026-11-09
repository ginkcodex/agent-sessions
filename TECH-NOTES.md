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

## Linux session-list scrollbar (WebKitGTK)

**Keep `transform: translateZ(0)` on `.virtual-list-container`** in
`frontend/src/lib/components/sessionlist/VirtualList.svelte`. Without its own
compositing layer, WebKitGTK (2.52.6, GTK4) misdraws that scroller's
`::-webkit-scrollbar` (style.css, Linux only):

- the thumb is missing while the top of the list is in view, and appears only
  after about the 16th session scrolls in;
- it then flashes each time the pointer highlights a new row.

macOS uses overlay scrollbars and never showed it. The workspace tree uses the
same scrollbar theme and was fine. `frontend/test/virtual-list.test.mjs` fails
if the declaration is removed.

The geometry was never wrong: the 10px lane is reserved, and scrollHeight is
rows × 84 at every offset. So don't fix it in the list. These changes did
nothing (SS-01 to SS-06):

- removing `-webkit-overflow-scrolling`;
- removing the row hover transition;
- removing `contain`;
- `top` instead of `translateY` rows;
- rebuilding the list as normal-flow rows with spacers.

The engine-level cause was not traced. See
[docs/plan/session-scrollbar.md](docs/plan/session-scrollbar.md).

The transform makes the container the containing block for `position: fixed`
descendants. A fixed popover inside a row would be placed relative to the
list, so render menus outside VirtualList, as the session menu already does.

How to test scrollbar paint on Linux:

- **Only the real desktop app with native input reproduces it.** A standalone
  WebKitGTK window loading the Vite page showed the thumb at the top, which
  was false reassurance. Scrolling by setting `scrollTop` from JavaScript
  isn't native input either.
- **The packaged app has no Web Inspector.**
- Build a candidate outside `build/bin`:
  `npm --prefix frontend run build && go build -tags desktop,production -o /tmp/<name> ./cmd/agent-sessions`.
  Run it with the real local data.
- Drive it with a python-evdev `UInput` device (`/dev/uinput` is writable for
  the user): `ABS_X`/`ABS_Y` move the pointer, and `REL_WHEEL` -1/+1 scrolls
  down/up.
- Capture with the `org.freedesktop.portal.Screenshot` D-Bus call
  (non-interactive). GNOME Shell's own Screenshot interface is denied.
  Crop the scrollbar lane before judging.
- Portal captures can miss a flash shorter than one frame. Take several
  during a pointer sweep, and have the user confirm on their build.

## Linux transcript-header actions flashing (WebKitGTK)

Rapid scrolling with the pointer over the session list flashed Resume/New in
its neighboring transcript header. Slow scrolling did not reproduce it. Apply
Linux-only `transform: translateZ(0)` to each Resume/New button, not menu
wrappers or the entire header. The user confirms those buttons no longer flash.
This suggests a paint/compositing issue, but the engine cause is not traced.
Native menu/top-thumb checks remain pending in
[SS-08](docs/tasks/session-scrollbar/SS-08.md). Remaining left-header flashing
is separate follow-up work.

## Remote shell locale (SSH and WSL shell tabs)

**Keep `panelRemoteLocale` at the start of `PanelShellScript`**
(`internal/launch/panel_locale.go`, `panel.go`). Without it, a plain shell tab
on an SSH host can start with the POSIX/ASCII locale, which breaks Vietnamese
input there.

The symptom: Vietnamese typed with OpenKey (EVKey and GoTiengViet work the same
way) came out as wrong text, and the cursor jumped to the first line. It
happened only in plain shells on a remote host. These all worked on the same
Mac:

- a local shell tab;
- Claude Code, Codex and OpenCode, both local and remote;
- `ssh` to the same host from iTerm or Terminal.

The failing shell showed:

- `LANG` was empty and `LC_CTYPE` was `POSIX`.
- `locale charmap` reported `ANSI_X3.4-1968`.
- `locale -a` listed `C.utf8` and `en_US.utf8`, so UTF-8 locales were
  installed; nothing selected one.

These IMEs delete the typed letters and send the accented replacement as
UTF-8. In an ASCII locale, bash's readline treats each byte as one character.
Its redraw then garbles the line and moves the cursor. Agents work because
their TUIs decode UTF-8 themselves and don't rely on the shell's locale.

Why only the app:

- `sshEnvironment` (`internal/remote/ssh.go`) starts from `os.Environ()`.
  Local shells use `term.LoginEnv`, which re-reads the login shell's
  environment, but SSH does not.
- An app opened from Finder often has no `LANG`, so `SendEnv LANG LC_*`
  (macOS default, `/etc/ssh/ssh_config.d/100-macos.conf`) has nothing to send.
- Without a forwarded locale, many Linux servers fall back to POSIX.
- Terminal apps export `LANG`, which is why `ssh` from them works.

The fix runs on the remote host, before the login shell starts. When `LANG`,
`LC_CTYPE` and `LC_ALL` are all unset or empty, it reads `LC_ALL=C locale -a`
there and exports `LANG` as the first match in this order:

1. `C.UTF-8` or `C.utf8`.
2. `en_US.*` UTF-8.
3. Any other UTF-8 locale.

Rules to keep if you change it:

- **Never guess a locale name.** A locale available on the client may not
  exist on the host. Setting a missing one causes `setlocale` warnings and
  leaves the shell in ASCII anyway. Take names from the host's own `locale -a`.
- **Never override an explicit setting**, including `LANG=C` or
  `LC_CTYPE=POSIX`. Do nothing when `locale` is missing, fails, or lists no
  UTF-8 locale.
- **Only remote plain shells.** Local shells already get the login
  environment. Agent launches don't need the fallback, and adding it there
  would change their environment for no reason.
- The user's startup files still run after the fallback, so a `LANG` they
  set wins.

Forwarding the client's `LANG` from the app instead was not chosen: the name
may not exist on the host, and the server's `AcceptEnv` may drop it anyway.

`internal/launch/panel_locale_test.go` runs the real bootstrap, both raw and
in its quoted SSH form. A stub `locale` replays host lists (including the
reported one) and failure cases. A fake login shell prints the result. One
more test runs the bootstrap with the machine's real `locale -a` and checks
that the charmap becomes UTF-8. A quick check on a remote tab is
`locale charmap`: it should print `UTF-8`. If a host still fails, a user can
put `export LANG=C.UTF-8` in `~/.bashrc` or `~/.profile` there. See
[docs/plan/macos-terminal-input.md](docs/plan/macos-terminal-input.md) and
its QA note.

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
  for touchscreens. A message-only window on its own thread reads the
  Precision Touchpad through Raw Input (`mousenav_windows.go`, usage page
  `0x0D` / usage `0x05`, `RIDEV_INPUTSINK`) and parses the reports with
  `HidP_*`; `internal/touchswipe` turns the contacts into one swipe per
  touch: two fingers, 10 mm sideways, more than twice the vertical. Fingers
  right is back, as on macOS. It goes to the app window under the cursor,
  where Windows sends the scroll. Touchpads that are not Precision
  Touchpads do not swipe.
- **Don't spot swipes from the page's wheel events** (SN-02 to SN-08). They
  carry no phase, and while the page is busy the renderer merges them into
  a few large deltas and leaves gaps in the stream. On a real touchpad
  swipes were missed for minutes after start, and stepped several times
  after a step that was slow to draw. Splitting the stream by speed made
  one swipe go several steps even when idle.
- `AGENT_SESSIONS_TRACE_NAV=1` writes touchpad layouts, fingers down,
  swipes and sends to `%TEMP%\agent-sessions-nav-trace.log` (the app has
  no console). `mousenav_windows_test.go`, cross-compiled with
  `GOOS=windows go test -c -tags desktop,production ./cmd/agent-sessions/`
  and run from WSL, prints each touchpad's report layout.

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

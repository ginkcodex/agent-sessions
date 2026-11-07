# Two-finger swipe back and forward on Windows and Linux

Planned 2026-11-07. Tasks: [swipe-navigation-windows-linux](../tasks/swipe-navigation-windows-linux/README.md).

## Goal

A two-finger horizontal swipe on a touchpad goes back and forward on
Windows and Linux as it does on macOS ([swipe-navigation](swipe-navigation.md)):
in the session list's history and, while Go to File is open, in the editor's
history. A gesture that scrolls something sideways stays a scroll; only
once nothing under the fingers can scroll that way does a swipe navigate.

## Today

- macOS: `mousenav_darwin.m` spots the swipe from phased scroll events and
  sends `nav:swipe`. `swipeNavigation.ts` (`SwipeGuard`) drops it if the
  same wheel gesture could still scroll something sideways, and
  `subscribeMouseNavigate` (api.ts) delivers the rest.
- Linux: `mousenav_linux.c` hooks each window's web view for the side
  buttons only. A touchpad swipe reaches WebKitGTK as scrolling and does
  nothing else.
- Windows: WebView2 reports the side buttons to the page, so nothing is
  native. Its own swipe navigation (`IsSwipeNavigationEnabled`) is off and
  the app keeps no page history anyway. A touchpad swipe only scrolls.
- On every platform, `scrollsSideways` only counts elements whose CSS
  overflow scrolls. Browsers give text inputs `overflow: clip`, so a
  sideways scroll through a long value in a text field is not seen as a
  scroll, and on macOS today it can navigate.

## Design

### Linux: native, like macOS

The web view of each window also gets a `GtkEventControllerScroll` (both
axes, capture phase) next to the side-button gesture. Its `scroll` handler
always returns `FALSE`, so WebKit scrolls as before.

- Only touchpads count: the event's device source is
  `GDK_SOURCE_TOUCHPAD`. Wheels, tilt wheels and trackpoints do not.
- A gesture starts on `scroll-begin` and ends on `scroll-end`, when the
  fingers lift (Wayland). Where GTK has never sent a `scroll-end` (X11), a
  scroll more than 200 ms after the last one also starts a gesture. Once
  `scroll-end` has come, fingers resting mid-swipe do not split the gesture
  (SN-06). Scrolls with Ctrl held are ignored.
- The controller gets each scroll twice; the copy, with the same time,
  can come after `scroll-end` and make GTK begin again. Anything no newer
  than the newest event seen is skipped (SN-07).
- Deltas add up in pixels: `GDK_SCROLL_UNIT_SURFACE` as is, wheel units
  (X11) times 40, WebKit's line step.
- At `SWIPE_DISTANCE` (80 px) sideways, more than twice the vertical,
  `goSwipeNavigate` sends `nav:swipe` once per gesture, to that window.

### Windows: native, from the touchpad's contacts

WebView2 runs in another process: the app's window never sees its scroll
messages. The page's wheel events (the first design, SN-02 to SN-08) carry
no phase and no device, and the renderer reshapes them under load: while
the page is busy it merges them into a few large ones, and leaves gaps in
the stream. A swipe guessed from them was missed while the app was busy
(at startup, while it indexes) and went several steps after a step that
took long to draw. So Windows reads the touchpad itself (SN-09):

- A message-only window on its own thread registers for Raw Input from
  Precision Touchpads (HID usage page `0x0D`, usage `0x05`) with
  `RIDEV_INPUTSINK`. It parses each report with the device's preparsed
  data (`HidP_*`): contact count, and per finger its contact ID, tip
  switch, X and Y. Reports in hybrid mode (a frame split over several
  reports) are joined into frames.
- A touch lasts from the first finger down to the last one up. While
  exactly two fingers are down, their midpoint's movement since they came
  down counts; three or more fingers are a system gesture and never swipe.
  At `SwipeDistanceMM` (10 mm) sideways, more than twice the vertical, it
  sends `nav:swipe` once per touch; a touch that first moves 10 mm up or
  down is a scroll and never swipes. One touch is one step, whatever the
  page does; the next step needs the fingers to lift.
- The swipe goes to the app window under the cursor, as the scroll does
  (Windows scrolls the window under the pointer). If that is not an app
  window, nothing is sent.
- A frame with no contacts ends the touch; so does a second without
  reports, in case the lift was lost.
- Touchpads that are not Precision Touchpads send no such reports and do
  not swipe.
- `AGENT_SESSIONS_TRACE_NAV=1` writes the touches and sends to
  `%TEMP%\agent-sessions-nav-trace.log`: the app has no console.

The page then treats `nav:swipe` as on macOS and Linux: `SwipeGuard` drops
it while the same wheel gesture could scroll something sideways (SN-10). The
page's own detector is removed.

### Direction

On Linux the swipe follows the scroll, as Chrome does: a gesture that
scrolls toward the left (negative `deltaX`) is back, toward the right is
forward. With the touchpad's default direction on GNOME and Plasma 6
(content follows the fingers), that is fingers right = back, as on macOS.
The scroll direction setting is not read, so a user with the traditional
direction gets the opposite, as in Chrome there. On Windows the fingers
themselves count (SN-09): fingers right = back, whatever the scroll
direction setting, as on macOS.

### Text fields scroll too

`scrollsSideways` also counts `<input>` and `<textarea>` elements with
room left, whatever their CSS overflow. Scrolling through a long value in a
text field then never navigates, on any platform.

## Decisions

- Raw Input from the Precision Touchpad on Windows (decided with the user
  2026-11-07, after SN-08). The first choice, also with the user, was a
  heuristic on the page's wheel events: small and unit-tested, but on a
  real touchpad it missed swipes while the app was busy and stepped several
  times after slow steps, which no tuning of the wheel stream can fix. A
  mouse, a thumb wheel or a touchscreen never swipes now.
- No system setting gates it: neither Windows nor GNOME has *Swipe between
  pages*.
- Same distance and ratio as macOS, so tuning one informs the other.
- `nav:swipe` stays the native event name; the comments that say "macOS
  only" are updated.

## Risks

- On Windows, 10 mm and the touchpad's reported size are guesses to tune on
  a real touchpad. A touchpad that reports no physical size is taken as
  100 mm wide. The HID parsing is checked only by its unit tests until it
  runs on real touchpads; reports from vendors that bend the PTP spec may
  need fixes.
- `nav:swipe` from Windows can reach the page before the wheel events of
  the same gesture, while the page is busy; then a swipe over sideways
  scrollable content may navigate. macOS and Linux share that race. A thumb wheel or a
  free-spinning wheel may still pass for a touchpad.
- X11 touchpads report wheel units; the 40 px scale may make swipes too
  easy or too hard there.
- `mousenav_linux.c` compiles where the GTK 4 and WebKitGTK 6 headers are
  installed, and in the CI's `linux-build` job. Whether GTK sends
  `scroll-end` on lift needs a real touchpad (SN-04). One Wayland trace
  shows it does.
- Skipping copies by event time assumes the copies keep their time and
  real events never share one. Two real scrolls in the same millisecond
  would lose the second; touchpads send one about every 7 ms.
- WebKitGTK may deliver momentum to the page after the fingers lift. That
  keeps the page's gesture going, which only makes it more careful.

## Release criteria

- Windows (touchpad): swipe right goes back, left goes forward, in the
  session list and in Go to File. A long code line, a wide transcript block
  and a long text field scroll sideways without navigating; a new swipe at
  their edge navigates. A mouse wheel, Shift+wheel and the side buttons
  behave as before.
- Linux (touchpad): the same; a mouse's tilt wheel does not navigate.
- macOS: unchanged, except that text fields now count as scrollable.
- `go vet`, `go test`, `npm test`, `npm run check` pass; CI's Linux desktop
  build compiles the C.

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
- A gesture starts on `scroll-begin`, or on a scroll more than 200 ms after
  the last one (X11 may send no `scroll-end`). `scroll-end` finishes it.
  Scrolls with Ctrl held are ignored.
- Deltas add up in pixels: `GDK_SCROLL_UNIT_SURFACE` as is, wheel units
  (X11) times 40, WebKit's line step.
- At `SWIPE_DISTANCE` (80 px) sideways, more than twice the vertical,
  `goSwipeNavigate` sends `nav:swipe` once per gesture, to that window.

### Windows: from the page's wheel events

WebView2 runs in another process: the app's window never sees its scroll
messages, and the page's wheel events carry no phase and no device. So the
page spots the swipe itself. `SwipeGuard` gets a detector, turned on only
on Windows (`navigator.platform` starts with `Win`):

- It uses the guard's strokes. Wheel events less than 200 ms apart are one
  stream, but a stream may hold several swipes: the touchpad's momentum
  after the fingers lift, or a quick lift and touch, joins the next swipe
  to the last. So a stroke also ends when the fingers come back: the speed
  fell to half its peak or less, then rose for two events in a row.
  Momentum only slows down. Each stroke swipes once and is a scroll or not
  on its own (SN-05).
- A gesture is not a swipe if any event is Shift+wheel (a mouse scrolling
  sideways), counts in lines or pages (`deltaMode` not 0), or moves a whole
  wheel step at once (`|delta| ≥ 100` px). Ctrl+wheel (pinch) is ignored, as
  today.
- At `SWIPE_DISTANCE` (80 px) sideways, more than twice the vertical, over
  at least 3 events, it fires once per gesture, unless the gesture scrolled
  something. Listeners subscribed through `subscribeMouseNavigate` get it
  like `nav:swipe`.

### Direction

On Windows and Linux the swipe follows the scroll, as Chrome and Edge do:
a gesture that scrolls toward the left (negative `deltaX`) is back, toward
the right is forward. With the touchpad's default direction on Windows,
GNOME and Plasma 6 (content follows the fingers), that is fingers right =
back, as on macOS. The scroll direction setting is not exposed to the page,
nor read reliably on either system, so a user with the traditional
direction gets the opposite, as in Chrome there.

### Text fields scroll too

`scrollsSideways` also counts `<input>` and `<textarea>` elements with
room left, whatever their CSS overflow. Scrolling through a long value in a
text field then never navigates, on any platform.

## Decisions

- Wheel heuristic on Windows, not Raw Input from the Precision Touchpad
  (decided with the user 2026-11-07). It is small and unit-tested; the cost
  is that a smooth sideways mouse wheel (such as a thumb wheel) can also
  swipe.
- No system setting gates it: neither Windows nor GNOME has *Swipe between
  pages*.
- Same distance and ratio as macOS, so tuning one informs the other.
- `nav:swipe` stays the native event name; the comments that say "macOS
  only" are updated.

## Risks

- The Windows thresholds (3 events, 100 px wheel step, the stroke's
  half-speed decay and two-event rise) are guesses that
  need a real touchpad and a real mouse to tune. A thumb wheel or a
  free-spinning wheel may still pass for a touchpad.
- X11 touchpads report wheel units; the 40 px scale may make swipes too
  easy or too hard there.
- `mousenav_linux.c` cannot be compiled in this WSL environment (no GTK 4
  headers, no sudo); the CI's `linux-build` job compiles it.
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

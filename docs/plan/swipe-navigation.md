# Two-finger swipe back and forward (macOS)

Planned 2026-11-07. Tasks: [swipe-navigation](../tasks/swipe-navigation/README.md).

## Goal

On macOS, a two-finger horizontal swipe on the trackpad (or a one-finger
swipe on a Magic Mouse) goes back and forward, as in Safari: in the session
list's history and, while Go to File is open, in the editor's history
([editor-history](editor-history.md)). It follows the system setting
*Trackpad → More Gestures → Swipe between pages*.

## Today

`cmd/agent-sessions/mousenav_darwin.m` has an `NSEvent` local monitor for the
side buttons (`OtherMouseDown`/`Up`, buttons 3/4) and for `NSEventTypeSwipe`.
Logi Options+ sends its side buttons as such swipes, and so does a
three-finger swipe when *Swipe between pages* is set to three fingers. With
the default two-finger setting, macOS sends no swipe event: the app gets
ordinary scroll events, and apps that want page swipes (Safari, Chrome)
detect them from those. WKWebView's own gesture
(`allowsBackForwardNavigationGestures`) is off, which is right for a
one-page app, so nothing happens today. TECH-NOTES says a three-finger swipe
navigates "as in Safari"; that only holds with the three-finger setting.

## Design

### Native: spot the swipe, never take the scroll

The monitor also watches `NSEventTypeScrollWheel` and always passes the
event on, so the page scrolls as before.

- Only scrolls with a gesture `phase` count: trackpads and the Magic Mouse
  have one, wheels do not, nor does momentum after the fingers lift.
- `Began` (or `MayBegin`) starts a gesture; `scrollingDeltaX`/`Y` add up.
- Once the gesture has gone at least `SWIPE_DISTANCE` (80 points)
  sideways, and more than twice as far sideways as up or down, the window
  gets `nav:swipe` with `"back"` or `"forward"`, once per gesture.
- Only when `[NSEvent isSwipeTrackingFromScrollEventsEnabled]` (the system's
  two-finger *Swipe between pages*) is on.
- Direction follows the fingers: right is back, left is forward, whatever
  the scroll direction setting (`isDirectionInvertedFromDevice`).

`goSwipeNavigate` in `mousenav_native.go` sends it like `nav:mouse`, to the
window it happened in.

### Frontend: a scroll is not a swipe

The page drops a swipe when the gesture moved something that can still
scroll sideways in its direction. Examples: a long line in the code view,
a wide code block or table in a transcript, the tab strip.
`swipeNavigation.ts` watches `wheel` events on the window (passive, never
cancelled):

- A gesture is a run of wheel events less than 200 ms apart.
- It counts as a scroll if, for any of its events with a sideways delta,
  the target or one of its ancestors can still scroll in that delta's
  direction.

`subscribeMouseNavigate` (api.ts) delivers `nav:swipe` to its callers
unless the current gesture is a scroll. App and Go to File listen there
already, with their modal rules: one swipe moves the session list or the
editor, never both, and nothing while a prompt or Settings is open.

The swipe's direction comes from the native side; the frontend only checks
whether to drop it. So a gesture that started by scrolling never
navigates, even once it reaches the edge, as in Safari. A new swipe from
the edge does.

## Decisions

- macOS only. Windows and Linux touchpads report no gesture phase to the
  page, and their webviews have no such convention in this app. They are
  left for later.
- No sliding page animation; the step happens once the swipe is long
  enough.
- The system setting decides, as in Safari: with *Swipe between pages* off
  (or set to three fingers only), two-finger swipes only scroll.
- `NSEventTypeSwipe` handling stays: Logi Options+ side buttons need it.

## Risks

- `SWIPE_DISTANCE` and the 2:1 ratio are guesses. They need tuning on a real
  trackpad, so that they neither trigger while scrolling nor feel sluggish.
- The ObjC cannot be built on Linux; the macOS CI job compiles it
  (`go build -tags desktop,production`), and only a Mac can run it.
- The frontend's scroll check relies on CSS overflow. Content that scrolls
  sideways by script would not count as scrollable. None is known in the
  app today.

## Release criteria

- On a Mac with the default two-finger setting, a swipe right goes back
  and left goes forward, in the session list and in Go to File.
- Scrolling a long code line or a wide transcript block sideways does
  not navigate. Swiping again at its edge does.
- With *Swipe between pages* off, two-finger swipes do not navigate.
- Mouse side buttons, Logi Options+ and the keys behave as before.
- `go vet`, `go test`, `npm test`, `npm run check` pass; the macOS CI job
  builds the desktop binary.

# Session list scrollbar stability

## Goal

Stop the session list scrollbar misbehaving on Linux (WebKitGTK). It is missing
while the top of the list is in view, appears only after about the 16th session
scrolls in, and then flashes each time a new row is highlighted on hover. The
workspace scrollbar on the same platform is stable. macOS uses overlay
scrollbars and is not affected.

## Design

Give the session list's scroll container (`.virtual-list-container` in
`VirtualList.svelte`) its own compositing layer with `transform: translateZ(0)`.
Nothing else changes: the virtualization, the transformed row positioning, the
shared Linux scrollbar theme and the workspace list all stay as they are.

The scroll geometry was already correct: a 10px scrollbar lane is reserved and
scrollHeight is rows × 84 at every offset. So the fault is in how WebKitGTK
paints the scrollbar of a non-composited scroller, not in the list. Giving the
scroller its own layer avoids it. The engine-level cause was not traced.

## Experiment results

- SS-01: removing the legacy scrolling hint did not help on the user's Linux
  device; reverted. The user observes one flash per newly highlighted hover row.
- SS-02: disabling Linux row background animation did not help; reverted.
  User reports scrollbar absent at the top, appearing after the 16th session
  enters, then flashing on each new hover highlight.
- SS-03: removing virtual-row containment did not help; restored. User confirms
  rebuilding with `make package-linux` and running the new app for these tests.
- SS-04: absolute `top` positioning did not help; restored transforms.
- SS-05: a standalone WebKitGTK probe did not reproduce the bug. The actual
  desktop binary with real data and native evdev wheel input did: the scrollbar
  was missing after scrolling back to the top.
- SS-06: normal-flow rows with top/bottom spacers (no phantom, transforms or
  containment) still had no scrollbar at the top; reverted.
- SS-07: `transform: translateZ(0)` on the scroll container fixed both symptoms,
  with the SS-06 structure and with the original one. Kept on the original
  structure.

## Decisions

- Keep the one-line fix on the original structure; SS-06 is not needed and was
  reverted to keep the change small.
- Judge each candidate in the actual desktop app with native input and
  screenshots. The standalone WebKit probe gave false reassurance.
- A source-level regression test guards the declaration; it cannot show the
  rendering, which only native testing can.

## Risks

- The transform makes the container the containing block for `position: fixed`
  descendants. None exist today (the session menu renders outside VirtualList;
  the count tooltip is absolute). A future fixed popover inside a row would be
  placed relative to the list, not the window.
- One extra compositing layer for the list; no slowdown was noticed.
- Behaviour on other WebKitGTK versions or distributions is untested.

## Release criteria

- Frontend checks and tests pass.
- Native desktop test with real data: thumb visible at the top after a wheel
  scroll down and back up, and stays visible while hovering across rows.
- User confirms on their Linux device with `make package-linux`: no missing
  thumb at the top, no flashing on hover or scroll, normal scrolling and
  selection.

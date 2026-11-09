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

## Follow-up: transcript action flicker (SS-08)

The user reports that Resume and New in the adjacent transcript header flash
while rapidly scrolling the session list under the pointer on Linux. Slow
scrolling does not trigger it; Windows and macOS are unaffected. The scrollbar
itself remains fixed. This suggests paint/compositing under rapid hover/scroll
invalidation, but the engine-level cause is not established.

Test a narrowly scoped Linux-only compositing layer on the Resume/New button
surfaces. Do not transform the header or split-button menu wrappers: that would
also change dropdown stacking/containing blocks. Keep the existing list fix,
virtualization, button behavior, transitions and GPU renderer settings intact.
A source regression test can guard the scope, not prove the visual fix.

Risks: a small extra layer per filled button; native WebKit may still exhibit
the defect. Verify rapid wheel scrolling under the pointer, slow scrolling,
the scrollbar at the top, button hover/focus and dropdown overlap. Native
Linux/user confirmation remains a release gate; do not label an unconfirmed
candidate as a verified fix.

## Follow-up: navigation and badges flicker (SS-09)

The user confirms SS-08 stops Resume/New flashing during rapid scrolling, but
reports that the left navigation, agent icon, Live badge and model text still
flash. Give their shared `.header-badges` group the same Linux-only compositing
layer. This covers the version and Archived badge too, without promoting each
small element separately. The group contains no dropdowns or fixed descendants;
leave the header, action wrappers and SS-08 button layers unchanged.

Risks: one additional layer; future fixed descendants in this group would use it
as their containing block. Keep the Live pulse and navigation semantics intact.
Guard the Linux-scoped rule, run frontend checks/tests/build, and request native
fast-scroll confirmation for this newly covered group. Automated tests do not
prove painting behavior.

## Follow-up: individual navigation/badge surfaces (SS-10)

The user rebuilt and reports SS-09's shared-group layer does not stop the left
header flashing. Remove that ineffective group transform. SS-10 instead applies
the same Linux-only transform that worked on Resume/New directly to each
navigation button and agent/Live/Archived/model/version badge. NavButtons is
only used in this header. Keep wrappers untransformed and the Live animation
unchanged; do not add broad GPU/environment or scroll behavior workarounds.

The user confirms the remaining flashing is gone after rebuilding and testing
SS-10 (2026-11-09). Keep individual surface layers; the engine-level cause is
not established. Several small layers replace the ineffective group layer.
Automated source guards verify scope only; native confirmation comes from the
user's test, which did not itemize live/non-live cases separately.

## Release criteria

- Frontend checks and tests pass.
- Native desktop test with real data: thumb visible at the top after a wheel
  scroll down and back up, and stays visible while hovering across rows.
- User confirms on their Linux device with `make package-linux`: no missing
  thumb at the top, no flashing on hover or scroll, normal scrolling and
  selection.

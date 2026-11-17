# CSB-07 — Change the target machine from the rail's status dot

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-04
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

The rail's connection dot (CSB-04) only showed the status. Make it a button
that opens the footer chip's host menu (Local, WSL, SSH hosts, the alias
field, Disconnect and the environment overrides), so the target machine can
change without expanding the sidebar.

The rail clips overflow, so the menu is fixed beside the rail (left 44 px,
like the session switcher), its bottom level with the dot, its height capped
to the window with scrolling.

Likely files: `frontend/src/lib/components/sidebar/HostSelector.svelte`,
`frontend/src/App.svelte`, `frontend/test/connection.test.mjs`.

## Acceptance criteria and tests

- The dot keeps CSB-04's color, tooltip and accessible name in every phase,
  and is a button with `aria-haspopup` and `aria-expanded`.
- Clicking it opens the same menu as the footer chip; picking a target
  connects or disconnects as the chip does and closes the menu.
- Escape, an outside click and opening the session switcher close it; Escape
  returns focus to the dot.
- The menu stays inside the window and scrolls when the window is short.
- The footer chip's menu is unchanged.
- Run the frontend tests, `npm run check`, `npm run build`, and drive the
  browser mock.

## Evidence

2026-11-17:

- `HostSelector`'s `dotOnly` became `rail`: a 28 px `button.rail-trigger`
  holding the dot, sharing the chip's menu markup; in rail mode the menu gets
  `rail-menu` (fixed, left 44 px) and an inline `bottom` and `max-height`
  measured from the dot when it opens. Escape now also returns focus to the
  trigger, in both the rail and the footer chip.
- New test in `connection.test.mjs`: server-renders the rail variant with the
  menu open and checks the trigger's title, label and ARIA, the connected
  dot, no chip, the full menu with its placement, and that the footer variant
  has none of it.
- `npm test`: 1539 passed, 1 skipped, 0 failed. `npm run check`: 0 errors,
  0 warnings. `npm run build`: passed.
- Browser mock (Playwright, Chrome, 1400×800), collapsed: dot at (6, 764);
  menu opened at (44, 465), 250×327, bottom level with the dot, focus in the
  alias field. Picked `prod-server`: menu closed, dot green, "Connected to
  prod-server". Escape closed it with focus on the dot; an outside click
  closed it; the switcher and the host menu close each other. Picked Local:
  back to "Target: Local Machine". At 900×320 the menu was 250×304 at
  (44, 8) and scrolled. Expanded, the footer chip's menu opened at (10, 440)
  as before. No page errors.

Follow-up, same day: the button now looks like the footer chip shrunk to its
dot: the chip's `--bg-tertiary` fill, `--border-color` border and hover
colors, as a 25 px circle. The rail's content is 39 px wide, so the chip's
24 px would sit at x = 7.5 and blur the 1 px border on 1× screens; at 25 px the
button sits at x = 7 and the 7 px dot at x = 16.

- Browser mock (Chrome, 900×500, light and dark): the button's computed
  background and border colors equal the footer chip's in both schemes;
  screenshots at 1× and 2×, at rest and hovered, looked right. No page errors.
- `npm test`: 1539 passed, 1 skipped, 0 failed. `npm run check`: 0 errors,
  0 warnings. `npm run build`: passed.

Not checked: the desktop app (CSB-02); the connecting, reconnecting and
disconnected colors on the button (unchanged classes); resizing the window
while the menu is open (its position is measured when it opens, like the
switcher's).

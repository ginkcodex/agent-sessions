# CSB-03 — Terminal button in the collapsed rail

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-01
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

While the sidebar is collapsed its footer's terminal button is hidden. Show
the same button at the bottom of the rail, where the app has a terminal panel.

Likely files: `frontend/src/App.svelte`.

## Acceptance criteria and tests

- The rail's bottom holds the terminal button, with the footer's title,
  label, `aria-pressed`, active color and attention dot.
- One definition serves both places.
- No button where the app has no terminal panel.
- Run `npm run check`, `npm run build` and the frontend tests; check the
  layouts in the browser mock.

## Evidence

2026-11-16:

- Moved the footer's terminal button into a `terminalButton` snippet rendered
  in the sidebar footer and at the bottom of the rail (`.rail-bottom`,
  `margin-top: auto`), both behind `launcher.info.panel`.
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passed.
  `npm test`: 1,512 passed, 1 skipped, 0 failed.
- Built frontend in mock mode (`vite preview`, which has a terminal panel)
  driven by Playwright with Chrome at 1400×800: the rail's Open terminal
  button is visible while collapsed and opens the panel (`aria-pressed`
  true, active color). With the panel set to the left and collapsed, the
  workspace has no `panel-left` class and the panel lies below the content;
  expanding puts it back on the left under the sidebar and list. Dark theme
  screenshot of the collapsed rail looked right.
- Not checked: the desktop app, a maximized terminal while collapsed (the
  rail is a `.pane`, which the maximized layout hides), and the attention
  dot in the rail.

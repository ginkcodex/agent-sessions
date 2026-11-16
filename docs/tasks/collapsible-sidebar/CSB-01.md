# CSB-01 — Collapse the sidebar and session list

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** —
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

Add the saved `navigationCollapsed` preference, the collapse button in the
sidebar header, the expand rail, and the CSS that hides the left panes and
moves a left terminal panel to the bottom while collapsed.

Likely files: `frontend/src/lib/stores/preferences.svelte.ts`,
`frontend/src/App.svelte`, `frontend/src/lib/icons.ts`, a preferences test.

## Acceptance criteria and tests

- The collapse button hides the sidebar, the session list and both splitters;
  the rail's button brings them back with the same widths.
- The state persists; unreadable or failing storage keeps the default
  (expanded) or the choice for this run.
- Buttons have labels, titles and `aria-expanded`; focus moves to the visible
  toggle.
- Terminal left, bottom and maximized layouts stay correct.
- Test: the preference saves, restores and removes its key.
- Run the frontend tests, `npm run check` and `npm run build`.

## Evidence

2026-11-16:

- Added `PreferencesStore.navigationCollapsed` with `setNavigationCollapsed`,
  saved as `agent-sessions:navigation-collapsed=on` and removed when expanded.
  Added a Hide sidebar button at the end of the sidebar header, a 40 px rail
  with Show sidebar while collapsed, Lucide `panel-left-close/open` icons, and
  CSS that hides the sidebar, the session list and both splitters without
  unmounting them. A left terminal panel lays out at the bottom while
  collapsed; the rail is a `.pane`, so a maximized terminal hides it.
- `npm test`: 1,512 passed, 1 skipped, 0 failed, including the new test
  (persist, restore, reset, failing storage keeps the choice for the run).
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passed.
- Built frontend in mock mode (`vite preview`) driven by Playwright with
  Chrome at 1400×800: Hide sidebar hides both panes and moves focus to Show
  sidebar; the state survives a reload; Show sidebar restores the panes and
  focuses Hide sidebar. Screenshots looked right in the light theme.
- Not checked: the desktop app, the dark theme, the terminal panel (left,
  bottom, maximized; the browser mock has no panel), and the sidebar at its
  200 px minimum, where the header now holds one more button and the title,
  which already truncated, truncates further. See CSB-02.

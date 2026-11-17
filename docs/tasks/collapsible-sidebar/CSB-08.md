# CSB-08 — Terminal toolbar and split layout while collapsed

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-06
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

Found in desktop testing: with the sidebar collapsed, a terminal docked left
shows at the bottom, yet its dock button read "Move terminal to the bottom"
(it went by the saved dock, not where the panel shows), and clicking it only
changed the saved dock. Split panes in that full-width bottom panel stacked
top to bottom for the same reason.

While collapsed the panel spans the window's width whatever its dock, like a
maximized panel, so it gets the maximized toolbar: no dock button, and the
"Stack split panes / Place split panes side by side" toggle, sharing the
maximized panel's saved choice (`maximizedStacked`). The terminal store gets
`leftDockHidden` (set by the app from the collapse), `placement` (the saved
dock, or `bottom` while the left dock is hidden) and `layoutChosen`
(maximized or left dock hidden). `stacked` reads them; so do the panel's
toolbar and the app's left-panel layout.

Likely files: `frontend/src/lib/stores/terminals.svelte.ts`,
`frontend/src/lib/components/terminal/TerminalPanel.svelte`,
`frontend/src/App.svelte`, `frontend/test/terminal.test.mjs`.

## Acceptance criteria and tests

- Collapsed, docked left or at the bottom: the toolbar is the maximized one
  (Split, New shell, layout toggle, Maximize, Hide), with no dock button.
- The layout toggle lays split panes out and is the same choice as
  maximized; the default is side by side.
- Expanded, the dock button and the dock's layout (stacked on the left, side
  by side at the bottom) are as before. Collapsing does not change the saved
  dock or the settings dialog's choice.
- CSB-06 still hides a left terminal on collapse and brings it back.
- Run the frontend tests, `npm run check`, `npm run build`, and drive the
  browser mock.

## Evidence

2026-11-17. A first version kept the dock button while collapsed, labelled
"Move terminal to the left … (shows the sidebar)", expanding the sidebar on
click. On review the user asked for the maximized toolbar instead; that
replaced it before any commit.

- Store: `leftDockHidden`, `setLeftDockHidden()`, `get placement()`,
  `get layoutChosen()`; `stacked` uses `maximizedStacked` when the layout is
  chosen, else whether the panel shows on the left. `App.svelte` sets the
  flag in an `$effect.pre` and in `setNavigationCollapsed`, and derives
  `panelLeft` from `placement`. `TerminalPanel` shows the layout toggle when
  `layoutChosen` (was: when maximized); its dock button is unchanged.
- Tests in `terminal.test.mjs`: a store test for `placement`,
  `layoutChosen` and `stacked` with the left dock hidden (docked left and
  bottom, the shared choice with maximized, restored); a server-render test
  of the collapsed toolbar (no dock button, layout toggle both ways, still
  a docked panel). Existing toolbar fixtures gained `placement` and
  `layoutChosen`; the App source check now looks for
  `terminals.placement === 'left' && !panelMaximized`.
- `npm test`: 1541 passed, 1 skipped, 0 failed. `npm run check`: 0 errors,
  0 warnings. `npm run build`: passed.
- Browser mock (Playwright, Chrome, 1400×800), two split panes: expanded and
  docked left, 660×280, stacked, toolbar with "Move terminal to the bottom".
  Collapsed and opened from the rail: 1400×280, side by side, toolbar Split,
  New shell, "Stack split panes", Maximize, Hide (as maximized, but
  Maximize). Stack: stacked; maximize: still stacked (shared choice);
  restore and "Place side by side": side by side. Expanded: back on the
  left, stacked, dock button back. Docked bottom and collapsed: the layout
  toggle again. No page errors.

Not checked: the desktop app (CSB-02).

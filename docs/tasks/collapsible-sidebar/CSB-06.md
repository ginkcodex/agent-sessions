# CSB-06 — Hide a left terminal with the collapsed sidebar

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-03
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

Collapsing the sidebar moved a terminal docked on the left to the bottom of
the window. It now hides with the sidebar and the session list, and comes
back on the left when the sidebar expands. The rail's terminal button still
opens it (at the bottom) while collapsed.

Likely files: `frontend/src/App.svelte`.

## Acceptance criteria and tests

- Terminal on the left and open: collapsing hides it; expanding shows it on
  the left again.
- Collapsed, the rail's terminal button opens it at the bottom; if the user
  closes it there, expanding leaves it closed.
- Terminal closed or docked at the bottom: collapse and expand do not change
  it.
- Run the frontend tests, `npm run check`, `npm run build`, and drive the
  browser mock.

## Evidence

Done: `setNavigationCollapsed` in `App.svelte` closes the panel when it is
docked left and open, and marks it hidden by the collapse; expanding reopens
it when marked. An effect clears the mark whenever the panel opens, so a
terminal the user opened from the rail and closed stays closed.

- `npm test`: 1522 pass, 1 skipped, 0 fail.
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passes.
- Browser mock (Playwright, Chrome, 1400×800): opened the terminal on the
  left (660×280 under the lists); collapsed: panel hidden; expanded: back on
  the left at the same size. Collapsed, opened from the rail: at the bottom,
  1400 wide; closed it, expanded: stays closed. Collapse and expand with the
  terminal closed: stays closed. No page errors.

Not checked: the desktop app (CSB-02); a restart while collapsed (the mark is
not saved, see the plan's Risks); terminal docked at the bottom (the code
path only acts on a left-docked panel).

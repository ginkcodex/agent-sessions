# CSB-05 — Session switcher in the collapsed rail

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** CSB-01
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

A Sessions button under the rail's expand button opens a floating panel that
lists workspaces and, inside each, their sessions; picking one opens it
without expanding the sidebar. See the plan's "Session switcher in the rail".

Likely files: a new `frontend/src/lib/components/sidebar/SessionSwitcher.svelte`,
a small pure helper with its test, `frontend/src/App.svelte`.

## Acceptance criteria and tests

- Workspaces come from the directory grouping with the sidebar's filters,
  starred-only and age filter; empty ones and subagents are left out.
- The open session's workspace starts open and the session is marked.
- Picking a session opens it like the session list does and closes the
  panel; Escape and an outside click close it and return focus.
- Late replies (closed panel, host switch) are dropped; load errors show in
  the panel.
- Button and panel have accessible names; workspace rows have
  `aria-expanded`; the open session has `aria-current`.
- Test the pure helper. Run the frontend tests, `npm run check`,
  `npm run build`, and drive the browser mock.

## Evidence

Done: `SessionSwitcher.svelte` (button and floating dialog) under the rail's
expand button, disabled while the data is stale; `switcherWorkspaces` in
`tree.ts` picks the outermost directory nodes with sessions. Workspace rows
show the directory name (`directoryName`, as the tree does) with the path as
tooltip. Opening the panel moves focus into it; replies are dropped by an
open/close generation counter and `isStaleReply`; the panel also closes when
the button becomes disabled (host switch).

- `npm test`: 1513 pass, 1 skipped, 0 fail (new test: "the session switcher
  lists outermost directories that hold sessions").
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passes.
- Browser mock (Playwright, Chrome): with a session open, collapsed, opened
  the switcher: 2 workspaces, the open session's workspace expanded and its
  session `aria-current`; focus was in the dialog. Escape closed it and focus
  returned to the button; a click outside closed it. Opened a closed
  workspace, picked its session: the panel closed, the transcript switched,
  and on reopen that session was current. Light and dark looked right. No
  page errors.

Not checked: the desktop app (CSB-02), a real host switch while the panel is
open, load errors (the mock does not fail), long lists on a small window.

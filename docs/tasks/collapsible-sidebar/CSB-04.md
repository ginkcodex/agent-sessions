# CSB-04 — Connection indicator in the collapsed rail

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CSB-03
- **Plan:** [collapsible-sidebar.md](../../plan/collapsible-sidebar.md)

## Scope and likely files

While collapsed, the footer's host selector is hidden, so nothing says
whether the window works on Local or a remote host. Show the host
selector's status dot, with its tooltip, below the rail's terminal button.

Likely files: `frontend/src/lib/components/sidebar/HostSelector.svelte`,
`frontend/src/App.svelte`.

## Acceptance criteria and tests

- The dot's color and tooltip match the footer's host chip in every phase
  (local, connecting, connected, reconnecting, disconnected), from the same
  code.
- It has an accessible name; it shows with or without a terminal panel.
- Run `npm run check`, `npm run build` and the frontend tests; check Local and
  a remote host in the browser mock.

## Evidence

2026-11-16:

- `HostSelector` takes `dotOnly`: it renders only the status dot in a 28 px
  `role="img"` box whose `title` and `aria-label` are the chip's status text.
  The rail's bottom stacks the terminal button and `<HostSelector dotOnly />`.
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passed.
  `npm test`: 1,512 passed, 1 skipped, 0 failed.
- Browser mock driven by Playwright with Chrome: after connecting to the mock
  SSH host `prod-server` and collapsing, the rail dot was green with
  "Connected to prod-server"; back on Local it was grey with "Target: Local
  Machine". Screenshots matched.
- Not checked: the connecting, reconnecting and disconnected colors in the
  rail (same classes as the footer), and the desktop app.

# PFC-01 — One-line rule rows with icon actions

- **Status:** 🟡 Partial (not checked visually in the running app)
- **Estimate:** 1 h
- **Depends on:** —
- **Plan:** [port-forward-compact.md](../../plan/port-forward-compact.md)

## Scope

Put each forwarding rule on one line: status, route, auto-start badge and
icon buttons on the right. Keep every action, its enabled state and an
accessible name.

## Likely files

- `frontend/src/lib/components/common/PortForwardDialog.svelte`
- `frontend/src/lib/icons.ts`
- `frontend/test/port-forward.test.mjs`

## Acceptance criteria

- One line per rule at the default dialog width; wraps cleanly when narrow.
- Open, copy, start/stop/retry/cancel, edit, remove behave as before.
- Each icon button has an `aria-label` and a tooltip.

## Evidence

- `svelte-check`: 0 errors, 0 warnings.
- `npm test` (after pulling origin/develop at 4a342f2): 1619 tests, 1614
  pass, 0 fail (5 skipped), including a new
  check that all five icon buttons have an `aria-label` and `title`.
- Not checked: the dialog in the running app (no browser automation set up
  here), light/dark appearance, narrow-window wrapping.

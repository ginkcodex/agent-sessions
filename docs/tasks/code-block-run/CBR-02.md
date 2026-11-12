# CBR-02 — Run a command in a new shell tab

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** CBR-01
- **Plan:** [code-block-run.md](../../plan/code-block-run.md)

## Scope

- `terminals.runCommand(command, dir)`: opens a shell tab with `newShell`,
  then at the tab's first directory report (or after 5 s without one) pastes
  the command through the tab's sink and sends Enter, once.
- A tab that has already reported a directory when it is returned gets the
  command at once; a tab that exits or closes first drops it.

## Likely files

- `frontend/src/lib/stores/terminals.svelte.ts` and its tests

## Acceptance criteria

- The command is pasted, then Enter is written, in that order, once.
- Nothing is typed before the first report, or into a closed or exited tab.
- A failed start shows the existing toast and types nothing.

## Tests

- `npm test -- terminals`

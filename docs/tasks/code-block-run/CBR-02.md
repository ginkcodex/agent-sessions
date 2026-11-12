# CBR-02 — Run a command in a new shell tab

- **Status:** ✅ Done
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

## Evidence

- `TerminalsStore.runCommand(command, cwd)` opens the tab with `newShell`
  and keeps the command in `pendingRuns`. `setCwd` (the view's OSC 7
  report) types it: `sink.paste(command)`, then `write(id, '\r')`, and
  forgets it. Without a report it is due after `RUN_FALLBACK_MS` (5 s); a
  due command whose tab has no view yet is typed when the view attaches. A
  tab that is not running drops it, and `remove` forgets it.
- `npm test`: 1414 tests, 1413 pass, 0 fail, 1 skipped. New in
  `test/terminal.test.mjs`: the command is typed at the first report,
  Enter after the paste, once (later reports and the fallback add
  nothing); the fallback, with a view mounted before and after it; a tab
  that already reported its directory; a closed tab, an exited tab and a
  failed start (toast shown) get nothing.
- Removing the flush from `setCwd`, the running check, or the forget
  after typing each makes these tests fail.
- `npm run check`: 0 errors, 0 warnings.
- Not checked: a real shell's first prompt in the desktop app (CBR-04).

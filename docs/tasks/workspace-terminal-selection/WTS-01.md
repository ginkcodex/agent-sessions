# WTS-01 — Directory clicks select the opened session's terminal

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [workspace-terminal-selection.md](../../plan/workspace-terminal-selection.md)

## Scope

When a workspace-tree row opens the first visible session, activate the terminal tab with that session's identity and directory, the same call a session-row click makes. Remove the earlier meta-less activation. Leave `TerminalsStore.selectDirectory` ranking unchanged.

## Likely files

- `frontend/src/lib/tree.ts`
- `frontend/src/lib/components/sidebar/GroupTree.svelte`
- `frontend/test/tree.test.mjs`
- `frontend/test/terminal.test.mjs`

## Acceptance criteria and tests

- The opened session's own tab wins over another agent tab of the same directory, even when it was launched elsewhere.
- An opened subagent selects its parent's tab.
- Without a session to open, the row's directory still selects a workspace tab.
- A superseded click or a host switch activates nothing.
- The panel neither opens nor takes typing focus.
- Run tree and terminal tests, the frontend check and build, and the full frontend suite.

## Evidence

Done:

- Added `workspaceTerminalTarget(visible, ref, rowDirectory)` to `tree.ts`. It returns the chosen session's meta and `cwd`. It falls back to the row's directory when the session's directory is unknown, or when no session is visible.
- `GroupTree.handleSelect` no longer calls `selectDirectory(cwd, host)` before the sessions load. Once `sessionToSelect` picks the session, it calls `terminals.selectDirectory(target.dir, host, target.session)` before `appState.selectSession`. It returns early when a later click or a host switch has superseded the click.

Verified on macOS (2026-11-09):

- New tests:
  - `workspaceTerminalTarget passes the opened session like a session-row click` (tree tests).
  - `directory click activates the opened session tab, not the first agent tab of the workspace` (terminal store). It covers the own tab launched elsewhere, the old directory-only result, the subagent's parent tab, the no-session fallback, and that the panel stays closed without taking focus.
- The wiring test now asserts the tree's new call, the single `selectDirectory` call and the host/supersede guard. With the old `GroupTree.svelte` restored, that test fails (52 pass, 1 fail). With the fix, all pass.
- The `GroupTree` render test now uses the real `tree.ts` instead of stubs, because the compiled import is split across lines.
- `node --loader ./test/resolver.mjs --test test/tree.test.mjs test/terminal.test.mjs`: 69/69 pass.
- `npm --prefix frontend run check`: 0 errors, 0 warnings.
- `npm --prefix frontend test`: 1,179 pass, 0 fail, 1 skipped (the existing Chrome layout test).
- `npm --prefix frontend run build`: succeeded, with only the usual chunk-size advice.
- `git diff --check`: clean.

Not checked:

- Clicking in the live desktop app. The tests drive the store and helper directly, and assert the component wiring by source text.
- The Go test suite. No Go code changed.

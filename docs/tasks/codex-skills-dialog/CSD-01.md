# CSD-01 — Codex skills inventory

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** —
- **Plan:** [codex-skills-dialog.md](../../plan/codex-skills-dialog.md)

## Scope

Update command routing, reuse InventoryDialog with the live Codex skill catalog, and add regression coverage.

## Acceptance criteria and tests

Only a bare `/skills` opens the dialog. List `$` skills and descriptions, support filtering and close/focus restoration, make no model or CLI call, and retain ordinary invocation. Run frontend type checks, relevant tests, and production build.

## Evidence

- Added Codex `/skills` to inventory command routing and the picker, replacing any advertised command of the same name.
- Reused InventoryDialog with reactive `$skill` catalog entries and descriptions; no model prompt or CLI inventory request.
- Regression coverage verifies bare-command recognition, builtin exclusion, description filtering, live catalog replacement, empty results, Escape/focus restoration, and unchanged skill invocation.
- `npm run check`: zero errors and warnings.
- `npm test`: all 156 test files passed. The expanded interaction test was then rerun directly: all 19 tests passed.
- `npm run build`: passed (chunk-size warning).
- `git diff --check`: passed.
- `make test`: sandbox attempt could not open test-server sockets. Retry with socket access completed, but failed Git-history tests in internal/engine, internal/files, and internal/rpc with `invalid Git worktree: git rev-parse: fatal: not a git repository`. All remaining packages passed. These failures are outside the frontend change.
- Native desktop/manual visual checks were not performed.

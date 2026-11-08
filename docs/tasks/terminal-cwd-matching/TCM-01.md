# TCM-01 — Match shells by detected cwd

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [terminal-cwd-matching.md](../../plan/terminal-cwd-matching.md)

## Scope

Use reported cwd for shell workspace matching, falling back to launch directory only when unavailable. Leave agent matching and selection/focus precedence unchanged.

## Likely files

- `frontend/src/lib/stores/terminals.svelte.ts`
- `frontend/test/terminal.test.mjs`

## Acceptance criteria and tests

- An active shell that reached the workspace with `cd` stays selected over a second shell launched there, including repeated transcript-style activation.
- A shell reporting another cwd no longer matches its launch workspace; unknown cwd falls back to launch directory.
- Agent cwd does not change session or launch-workspace matching.
- Same-host/running guards, path normalization, split-pane selection and no focus theft remain covered.
- Run terminal tests, frontend checks and full project tests; record actual results and any unverified desktop behavior.

## Evidence

- Shell matching now uses detected cwd, falling back to launch directory; original launch metadata remains intact. Agent matching is unchanged.
- Added repeated session activation regression for two shells, moving away, agent cwd stability and normalized detected paths; adapted split coverage.
- Terminal tests: 52/52 passed (`node --loader ./test/resolver.mjs --test test/terminal.test.mjs` from `frontend`).
- `npm --prefix frontend run check`: zero errors or warnings.
- `npm --prefix frontend test`: 1,148 passed, zero failed, one skipped (existing Chrome removed-row scrolling layout test).
- `make test` (`go test -race ./...`): passed (exit 0).
- `git diff --check`: passed.
- Live desktop/macOS cwd reporting and click behavior not checked; tests exercise the store with reported cwd values.


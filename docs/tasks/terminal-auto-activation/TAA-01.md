# TAA-01 — Make automatic terminal activation optional

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** None
- **Plan:** [terminal-auto-activation.md](../../plan/terminal-auto-activation.md)

## Scope

Add a default-enabled preference in the terminal store, persist it in the terminal-panel localStorage record, and expose it in Settings → Terminal. Gate automatic directory/session matching without affecting explicit terminal controls.

## Likely files

- `frontend/src/lib/stores/terminals.svelte.ts`
- `frontend/src/lib/components/common/ManageSettingsDialog.svelte`
- `frontend/test/terminal.test.mjs`
- `frontend/test/settings.test.mjs`

## Acceptance criteria

- New and legacy settings default to enabled; explicit false survives reload.
- Disabling takes effect immediately and keeps active tab, panel state and activation focus unchanged on automatic selection.
- Re-enabling restores existing matching; manual selection and explicit terminal actions remain available while disabled.
- Settings shows a clearly described checkbox with the current saved value.

## Tests

Run terminal/settings regressions, the full frontend suite, Svelte check, and frontend production build. Record results and any manual testing gaps below.

## Evidence

Done:

- `TerminalsStore.autoActivate` (default `true`) is saved in the `agent-sessions:terminal-panel` record. A missing or non-boolean stored value keeps it enabled.
- `selectDirectory` returns `false` first when it is off, so the active tab, the open state and `focusOnActivation` stay as they were. The three automatic callers (session list, workspace tree, transcript pointer/focus) all go through it. `select`, `selectNth`, `show` and the start/split actions are not gated.
- Settings → Terminal has the checkbox "Switch terminal tab when I select a session", with a hint, under the panel position.

Verified (from `frontend/`):

- `node --loader ./test/resolver.mjs --test test/terminal.test.mjs test/settings.test.mjs`: 66 pass, 0 fail. New tests cover default on, off and on again live, unchanged state while off, explicit select/selectNth/show while off, persistence and legacy/invalid values, and the checkbox render and handler.
- `npm test`: 1372 tests, 1371 pass, 0 fail, 1 skipped (already skipped).
- `npm run check`: 0 errors, 0 warnings. `npm run build`: succeeds.

Not checked: clicking through the toggle in the running desktop app.

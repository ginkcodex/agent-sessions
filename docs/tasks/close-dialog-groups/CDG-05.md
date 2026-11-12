# CDG-05 — The unsaved-files question

- **Status:** ✅ Done
- **Estimate:** 1.5 h
- **Depends on:** CDG-02, CDG-03
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

- `frontend/src/lib/closeConfirm.ts`: pure `unsavedDrafts(dirty)` — the
  `CloseWarning` for the unsaved-changes question: message "Unsaved
  changes in:" and one **Unsaved files** card (icon `file`) whose rows
  name the tab's path and `in <workspace dir>`, every file named without
  a cap.
- `frontend/src/lib/stores/filePrompt.svelte.ts`: the `icon` unions gain
  `'file'`.
- `frontend/src/lib/components/common/go-to-file/FilePrompt.svelte`:
  the file icon in the header and row icon maps.
- `frontend/src/lib/stores/fileActions.svelte.ts`: `resolve` asks with
  the builder's message and card instead of its flat `dir/path` list.
- Tests for `unsavedDrafts` in `frontend/test/close-confirm.test.mjs`
  and the icon maps in `frontend/test/file-prompt.test.mjs`.

## Likely files

- `frontend/src/lib/closeConfirm.ts`
- `frontend/src/lib/stores/filePrompt.svelte.ts`
- `frontend/src/lib/components/common/go-to-file/FilePrompt.svelte`
- `frontend/src/lib/stores/fileActions.svelte.ts`
- `frontend/test/close-confirm.test.mjs`
- `frontend/test/file-prompt.test.mjs`

## Acceptance criteria and tests

- `unsavedDrafts` returns the plan's message and card: one row per dirty
  file (path, `in <dir>`, file icon), all files named, and null-free
  since `resolve` asks only with dirty tabs.
- `resolve` asks with the card in every title it serves ("Close
  window?", "Switch host?", "Close files?"), and the discard and save
  paths behave as before.
- The file icon renders in the group header and the rows.

## Evidence

- `frontend/test/close-confirm.test.mjs`: "the unsaved-changes question
  names every dirty file in its own card" — the message, the Unsaved
  files rows (path, `in <dir>`, file icon), and all eight files named
  without a cap or `more`.
- `frontend/test/file-actions-coordinator.test.mjs`: the bulk-close
  question carries the card (rows for `a.txt` and `b.txt` in `/p`)
  beside the unchanged save/discard flow — the harness now loads the
  real `closeConfirm` module (as does `delete-file.test.mjs`'s).
- `frontend/test/file-prompt.test.mjs`: the header and row icon maps
  include the file glyph.
- `npm test` 1381 tests, 0 failures (1 pre-existing skip);
  `npm run check` clean; `npm run build` succeeds.

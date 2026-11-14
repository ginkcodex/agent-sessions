# LFV-06 — Keep typing in a large file responsive

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** LFV-05
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope

Measure the time per keystroke while editing the demo's 2.5 MB file in a
browser, find the whole-text work on that path, and remove it. Expected:
the document copy for the draft, and the compares of the draft with the
editor's text in `update()` and `editorDiagnostics`. If that is not enough,
report a large draft after a short pause and flush it before a save, a tab
switch or anything else that reads the draft.

## Likely files

`fileEditor.ts`, `FileEditor.svelte`, `stores/files.svelte.ts`,
`FileViewer.svelte`.

## Acceptance criteria and tests

- Keystroke time on the 2.5 MB file is recorded before and after.
- Saving right after typing writes the last keystroke.
- Frontend tests cover any new skip or flush; existing editor tests pass.

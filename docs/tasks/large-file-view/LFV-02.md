# LFV-02 — Show large files in the read-only editor

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** LFV-01
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope and likely files

- `FileViewer.svelte`: `largeView` (over 5,000 lines or `HIGHLIGHT_LIMIT`
  characters, not editing, no preview or diff) renders `FileEditor` disabled
  instead of `SourceView`, and routes find, hover, Problems and link
  navigation and the overview ruler to it. No highlight.js pass for it.
- `FileEditor.svelte`: `autofocus` prop, off for this view.
- `fileEditor.ts`: no highlight.js coloring past `HIGHLIGHT_LIMIT`.
- `codeHighlight.ts`: `HIGHLIGHT_LIMIT` moves here, shared by both.

## Acceptance criteria and tests

- A file over either limit renders the editor, disabled, without autofocus;
  small files and a large file's diff view keep `SourceView`.
- The editor adds no highlight marks to text past the limit.
- Existing viewer, editor, prompt and navigation tests still pass.

## Evidence

- `frontend/test/file-viewer-edit.test.mjs`: new test, 5,001-line and
  500,001-character files render the disabled editor with `autofocus=false`
  and no source view; a small file and a large file's diff keep the source
  view.
- `frontend/test/file-editor.test.mjs`: new test, a 560,000-character JSON
  document gets no decorations once both lazy loads report back. It fails with
  the limit check removed and passes with it.
- Harness updates: the viewer stub records `autofocus`; the navigation harness
  in `problems.test.mjs` provides `largeView`; `file-prompt.test.mjs` matches
  the new focus condition.
- `npm test`: 1,477 tests, 1,476 pass, 0 fail, 1 skipped. `npm run check`:
  0 errors, 0 warnings.
- Not checked: the desktop app with the reported file (LFV-03), and how the
  read-only editor feels for a 5,000–10,000-line file next to the old view.

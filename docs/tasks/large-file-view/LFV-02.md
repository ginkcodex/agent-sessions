# LFV-02 — Show large files in the read-only editor

- **Status:** ⬜ Not started
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


# LFV-07 — Color large files in the editor, lines in view only

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** LFV-06
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope

Optional. Documents over `HIGHLIGHT_LIMIT` show plain in the editor, because
highlight.js colors the whole document on every change. For them, use the
CodeMirror language the editor already loads (`loadLanguage`) with
`syntaxHighlighting` and a highlight style mapped to the viewer's `hljs-*`
colors. Lezer parses incrementally and in the background.

## Likely files

`fileEditor.ts`, `codeHighlight.ts` (shared class names).

## Acceptance criteria and tests

- A 2.5 MB JSON file is colored in view, and typing stays as fast as in LFV-06.
- Files under `HIGHLIGHT_LIMIT` keep highlight.js and look exactly as before.
- A frontend test checks which highlighter a large and a small document get.

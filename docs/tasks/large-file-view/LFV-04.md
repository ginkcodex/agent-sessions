# LFV-04 — Keep the diff view of a large file responsive

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** LFV-02
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope

With Git diff on, a large file with changes still renders every diff row in
`SourceView` (`largeView` excludes `showDiff`). Either render only the rows in
view, or show only the changed hunks with some context for files over the
large-file limits.

## Likely files

`SourceView.svelte`, `FileViewer.svelte` (change navigation, find), `files.ts`
(`diffRows`).

## Acceptance criteria and tests

- The diff view of a 100,000-line file with changes opens without freezing.
- Change navigation and find still reach every change.
- Small files' diff view is unchanged; frontend tests cover the large case.

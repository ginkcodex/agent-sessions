# Large file view

## Goal

Opening a large text file in Go to File must not freeze the app. The report
was `.ua/fingerprints.json`: 2.5 MB, 107,506 lines. The whole window stopped
responding.

## Cause

`SourceView` renders every line of the file: one gutter `<button>` with an
`aria-label` and a `title`, plus one line `<span>`, per line. For this file
that is more than 200,000 elements, built by Svelte and laid out by WebKit in
one go. Syntax highlighting was not the cause: files over 500,000 characters
already show plain.

## Design

The editor (CodeMirror, `FileEditor`) already renders only the lines in view,
and already has search, diagnostics, the overview ruler, code navigation,
wrapping and position restore. A file with more than 5,000 lines or more than
500,000 characters (`HIGHLIGHT_LIMIT`) is shown in that editor, read-only,
instead of in `SourceView`:

- `FileViewer` gets `largeView`: text content, not editing, no preview, no diff
  view, and over either limit. It skips the viewer's highlight.js pass then.
- The editor gets the viewer's text with version `-1` (never an edit's), is
  disabled, and does not take focus (`autofocus={false}`), so arrowing through
  the file picker keeps working.
- Find (button and Ctrl/Cmd+F), hover, Problems navigation, links to a line and
  the overview ruler go to the editor, as they do while editing.
- The editor's own highlight.js coloring now stops at `HIGHLIGHT_LIMIT` too. It
  colored the whole document at once, which would freeze on the same file.

## Decisions

- Reuse the editor instead of virtualizing `SourceView`. Virtualizing it would
  touch find, diagnostics painting, source navigation, scroll and font restore,
  change navigation and the overview measurer, all of which read the line DOM.
- 5,000 lines: well above normal source files (the largest in this repo is
  about 2,400), well below the size where the DOM cost is felt.
- The diff view keeps `SourceView`. Diffing a large file is rare; it is a
  separate task (LFV-04).

## Risks

- A large file's view loses what only `SourceView` has: the presentation font
  size, and blame following each click (the editor reports its selection
  when it is left, not on every change).
- The diff view of a large file still renders every row.
- Measured only in tests; the desktop app has not been checked with the
  reported file yet (LFV-03).

## Release criteria

- Opening the reported 2.5 MB JSON file keeps the app responsive, and scrolling,
  find and wrap work.
- Files under both limits look and behave exactly as before.
- Frontend tests and `svelte-check` pass.

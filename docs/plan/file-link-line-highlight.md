# Highlight the line a file link opens

## Goal

A file link with a line, such as `SettingsView.swift:23` in a session message
or the terminal, opens the file and scrolls to that line. In the source view
the line was not marked: the reveal placed an empty selection (a caret) at the
start of the line, which the read-only source view does not draw. Mark the
linked line, or every line of a range link, so the eye finds it at once.

## Design

- After `FileViewer` reveals a link's range, it records the range's lines as
  the tab's line selection, the same state a click in the line-number gutter
  sets. The source view already draws that selection (`.line.selected`, an
  accent tint on the line and its number).
- `file:23` marks line 23; `file#L23-L30` marks lines 23 to 30; `file:23:5`
  marks line 23 and still places the caret at column 5.
- The mark stays until the user selects another line, clicks outside the
  lines, or follows another link, as a gutter selection does.
- The editor and the large-file view are unchanged: they already put the caret
  on the line, and their active-line tint shows it. The recorded selection is
  the same line, so leaving and reopening the editor keeps it.

## Decisions

- Reuse the gutter's line selection instead of adding a second, link-only
  highlight. One marked-lines state means no second style, no timer and no
  rules about which highlight wins.
- Like a gutter click, a marked line shows its blame line when blame is
  available. That is the existing behaviour of a selected line, not new UI.
- Only links mark lines. Problems, search matches and code navigation already
  select text, and their range is not meant as a line selection.
- Lines are marked only when the reveal succeeded; a link outside the text
  still reports so and marks nothing.

## Risks

- A stale link (another tab, host or file version) must not mark lines in the
  wrong tab: the mark is set after the existing validity checks, on the tab
  the request named.

## Release criteria

- Tests: a line link marks its line; a range link marks every line in it; a
  link outside the text marks nothing; a problem navigation marks nothing.
- Frontend tests, Svelte check and production build pass; record checks not
  run.

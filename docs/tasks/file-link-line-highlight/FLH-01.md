# FLH-01 — Mark the lines a file link opens

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [file-link-line-highlight.md](../../plan/file-link-line-highlight.md)

## Scope

When the file viewer has revealed a file link's range, set the tab's line
selection to the range's lines so the source view tints them, as a gutter
click does. Other navigation kinds are unchanged.

## Likely files

- `frontend/src/lib/components/common/go-to-file/FileViewer.svelte`
- `frontend/test/problems.test.mjs`

## Acceptance criteria

- `file:N` marks line N; `file#LN-LM` marks lines N to M.
- A link outside the text, or a stale link, marks nothing.
- Problem, search and symbol navigation do not mark lines.

## Tests

Run the problems tests, the frontend suite, Svelte check and production build.

## Evidence

Done:

- In the navigation effect of `FileViewer.svelte`, after a successful reveal
  of a `link` request, `lineSelections` gets
  `{ anchor: start line + 1, head: end line + 1 }` for the tab and
  `selectionVersion` is bumped.
- `test/problems.test.mjs`: the viewer harness now provides `lineSelections`
  and `selectionVersion`. The file link test checks a line link marks its line,
  a range link marks lines 1–2, and a link to line 99 leaves the mark as it was.
  The problem navigation test checks that nothing is marked.

Verified (from `frontend/`):

- `node --loader ./test/resolver.mjs --test test/problems.test.mjs`: 14 pass,
  0 fail.
- `npm test`: 1618 tests, 1613 pass, 0 fail, 5 skipped.
- `npm run check`: 0 errors, 0 warnings. `npm run build`: succeeds.

Not checked: clicking a `file:line` link in the running desktop app.

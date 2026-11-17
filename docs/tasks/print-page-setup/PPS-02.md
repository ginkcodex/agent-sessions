# PPS-02 — Fit wide tables to the printed page

- **Status:** 🟡 Partial (no macOS WebKit print checked yet)
- **Estimate:** 2 h
- **Depends on:** PPS-01
- **Plan:** [print-page-setup.md](../../plan/print-page-setup.md)

## Scope

Short table cells keep to one line (`keepsCellOnOneLine`, the `nowrap`
class) so a long column beside them can't squeeze them. On screen a table
too wide for that scrolls; on paper it ran past the text column into the
right margin. In `data/demo.md` the 7-column "Options considered" table was
780 px wide in a 686 px column, so it looked wider than the "Fix ladder"
table, which was the correct width.

Before printing, a table wider than the page lets its one-line cells wrap:
its headers first, then the rest if still needed. Tables that fit are left
alone.

## Likely files

- `frontend/src/lib/print.ts` (`fitTables`)
- `frontend/src/lib/components/common/Print{Page,Sheet}.svelte`
- `frontend/test/print-page.test.mjs`, `print-setup-ui.test.mjs`

## Acceptance criteria

- No printed table is wider than the text column when its cells can wrap.
- Tables that fit keep their one-line cells.
- The print preview fits tables the same way and refits them when the page
  width or content changes.

## Tests

Unit test for the wrap steps with fake layout; CSS checks for the sheet;
headless Chrome render of `data/demo.md` before and after.

## Evidence

- The print sheet is now laid out off screen and hidden on screen (it was
  `display: none`), at the paper's width with its 2 px right inset, so
  `fitTables` can measure it before the print dialog opens. `PrintPage` refits
  on resize (a ResizeObserver, one frame later) for the preview.
- New tests in `print-page.test.mjs`: the wrap steps (fits / headers enough /
  needs all, with stale classes cleared) and the sheet CSS. The
  `print-setup-ui.test.mjs` import stub was updated for the new import.
- Full frontend suite: 1,544 passed, 0 failed. `npm run check`: 0 errors,
  0 warnings.
- Headless Chrome, `data/demo.md` at A4 / 14 mm, with the app's CSS:
  before, Options table 780 px in a 686 px column; after, 686 px with headers
  wrapped and every other table unchanged at 686 px. The printed PDF shows
  the table inside the text column.
- Not checked: printing from the real app on macOS WebKit or Windows, and
  the preview in the running app. A table whose single words or code are
  wider than the page still overflows.

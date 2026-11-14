# LFV-06 — Keep typing in a large file responsive

- **Status:** ✅ Done
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

## Evidence

Measured in the browser demo (Vite, Playwright, headless Chromium) on the
demo's `.ua/fingerprints.json` (2,576,493 characters, 107,506 lines): Edit,
caret at the end of line 4, 30–40 `x` key presses. Event Timing processing
time per key, and a CPU profile of 30 keys:

| | `input` median | p90 | max | Profile, 30 keys |
|---|---|---|---|---|
| Before | 13.6 ms | 15.9 ms | 22.9 ms | `update()` 366 ms (`lf` 67, `editorDiagnostics` 164, `sliceString` 188) |
| After | 4.4 ms | 14.0 ms | 19.8 ms | `update()` 11 ms; the draft's string 115 ms; GC 63 ms |

- Cause: on every key, `FileEditor`'s `update()` normalized the draft coming
  back (`lf`) and built the document's string again to compare them, and
  `editorDiagnostics` did both again, plus one more string per diagnostic.
- Fix (`fileEditor.ts`): `docText` builds each (immutable) document's string
  once, kept in a `WeakMap`; `showsText` compares a draft with it, first by
  identity, which is O(1) when the draft is the string the editor reported.
  `update()`, `adopt`, `adoption`, `editorDiagnostics`, `askedText`,
  `revealRange` and the highlighter use them. Behavior is the same: an equal
  draft, CRLF included, is still the document.
- Left per key: one string of the whole document for the draft (about 4 ms
  here) and the garbage it makes, which shows in the p90. Reporting a large
  draft only after a pause in typing would remove it, but the draft would then
  lag the editor, and every reader of the draft (save, tab switch, replace in
  files, close guards) would need a flush first. Not done: 4 ms is well inside
  a frame.
- Save right after typing: `y` then Ctrl+S, then Discard; the view shows the
  text with the `y`.
- Test: `file-editor.test.mjs` "a keystroke builds the document string once;
  the draft coming back is not compared again" counts `Text.toString` calls:
  one per keystroke, none for the draft coming back, and an equal CRLF draft
  is not adopted. It fails on the old code.
- `npm test`: 1,478 tests, 1,477 pass, 0 fail, 1 skipped. `npm run check`:
  0 errors, 0 warnings.
- Not checked: WebKitGTK and WebView2, likely slower than Chromium; a large
  file with many diagnostics; typing in files between 100,000 and 500,000
  characters, where highlight.js still colors the whole document per key
  (see LFV-07).

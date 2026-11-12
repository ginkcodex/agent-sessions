# TTR-01 — Readable tool cards

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** —
- **Plan:** [transcript-tool-readability.md](../../plan/transcript-tool-readability.md)

## Scope
Format recognized text envelopes; add command/path summaries, bounded output previews and accessible expansion controls to shared tool cards. Preserve raw copy, blob loading and search expansion.

## Likely files
`frontend/src/lib/toolPresentation.ts`, `frontend/src/lib/components/transcript/parts/ToolPart.svelte`, frontend tests.

## Acceptance criteria and tests
Text envelopes have actual newlines, unknown data is preserved, large output is collapsed until requested, search reveals full output, parameters remain independently expandable. Run frontend tests, check and build.

## Evidence
- Added conservative JSON text-envelope formatting, command/path summaries and eight-line/1,200-character previews with explicit expansion. Plain text escapes and mixed content are preserved; Copy still copies the original output.
- Tool output and parameters use the session's code-size token and 1.6 line height. Removed per-result nested scrolling and arbitrary word breaking; added visible parameter disclosure and keyboard focus outlines.
- `npm test`: 1,391 passed, 1 skipped, 0 failures (includes real-component DOM expansion/collapse, raw copy, search opening, blob retry/load; formatting, SSR and lazy-parameter tests).
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed; Vite reports its large-chunk warning.
- Native visual validation is separately tracked in TTR-02, not claimed by these DOM tests.


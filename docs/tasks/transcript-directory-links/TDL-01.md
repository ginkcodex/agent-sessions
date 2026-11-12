# TDL-01 — Exclude trailing-separator directory paths from file actions

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [transcript-directory-links.md](../../plan/transcript-directory-links.md)

## Scope

Reject directory paths in the shared session-content file parser after decoding URI escapes and stripping source locations. Preserve valid file links and the backend check for ambiguous paths.

## Likely files

- `frontend/src/lib/messageFileLinks.ts`
- `frontend/test/message-file-links.test.mjs`
- `frontend/test/markdown.test.mjs`

## Acceptance criteria

- Trailing forward and backward separators cannot become transcript file-opening actions.
- Encoded separators and source-location suffixes cannot bypass the check.
- Inline directory code remains code, without a file link; valid file links still render.

## Tests

Run parser/Markdown regressions, the frontend test suite, Svelte check and production build. Record results and manual testing gaps in Evidence.

## Evidence

Done:

- `parseMessageFileLink` returns null when the decoded path, after the `:line` or `#L` suffix is removed, ends in `/` or `\`. Inline code and explicit Markdown destinations in the transcript both use it, so directory paths such as `docs/tasks/terminal-auto-activation/` stay plain code and no longer raise "not a regular file". The click handler uses the same parser.
- Tests cover Unix, Windows, encoded (`%2F`, `%5C`), drive and suffixed directory paths, plus rendering in transcript Markdown with a valid file link beside them.

Verified (from `frontend/`): `npm test`: 1373 tests, 1372 pass, 0 fail, 1 skipped (already skipped). `npm run check`: 0 errors, 0 warnings. `npm run build`: succeeds.

Not checked: clicking the links in the running desktop app.

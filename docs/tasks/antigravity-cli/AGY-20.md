# AGY-20 — `file:///` links in the transcript do not open

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-16](AGY-16.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-20). In an Antigravity chat's
transcript, file names shown as blue links (`AGY-01.md`, `CLAUDE.md`,
`antigravity-cli.md`, …) do nothing when clicked. The same paths in the
terminal panel open.

## Cause

agy writes file links as local file URLs:
`[AGY-01.md](file:///home/…/docs/tasks/antigravity-cli/AGY-01.md)`, and
for code `[chat.go](file:///…/chat.go#L549)` or `#L98-L103`. 48 such links
sit in the user's agy transcripts; none has a host.

The transcript's link parser (`parseMessageFileLink` in
`frontend/src/lib/messageFileLinks.ts`) rejected every scheme, `file:`
included, so these became plain web links, which the app does not open.
The terminal shows the bare path, which its own matcher finds.

## Scope

- Map a local file URL (`file:///path` or `file://localhost/path`) to the
  absolute path it names, then parse it as before: escapes are decoded
  once, and `#L10` / `#L10-L20` give the line range. `file:///C:/x` gives
  `C:/x`.
- Absolute paths were already accepted, so this opens nothing a plain path
  could not.
- Still rejected: `file://host/…` and `file:////host/…` (network shares),
  `file:relative`, queries, directories, every other scheme.

## Evidence

Done:

- `parseMessageFileLink` strips a local `file://` prefix before its checks.
  The transcript renderer and the click handler (`TranscriptView`) both use
  it, so the link renders as a file link and opens like an absolute path.

Tests:

- `frontend/test/message-file-links.test.mjs`: agy's three link shapes,
  `localhost`, an escaped space and a Windows drive parse to the path and
  lines; a host, `file:////host`, `file:relative`, a query and a directory
  are rejected.
- `frontend/test/markdown.test.mjs`: a transcript `file:///` link renders
  as a file link, with and without `#L`; `file://server/…` does not.
- Both failed before the change and pass after.
- `npm test`: 1630 tests, 1625 pass, 0 fail, 5 skipped.
  `npm run check`: 0 errors, 0 warnings.

Not checked:

- A click in the desktop app on macOS.
- Relative links agy writes from inside a doc
  (`[antigravity-cli.md](../../plan/antigravity-cli.md)`) still resolve
  against the session's directory, not the doc's. Not seen in this
  screenshot; unchanged.

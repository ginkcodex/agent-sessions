# Exclude directory paths from transcript file links

## Goal

Do not offer a file-opening action for session-content paths ending in a directory separator, such as `docs/tasks/terminal-auto-activation/`.

## Design

Reject a trailing `/` or `\` in the shared transcript file-destination parser, after URI decoding and source-location suffix removal. Inline code then remains code without a link, explicit Markdown destinations do not become file-opening actions, and the click handler also rejects stale directory destinations.

## Decisions

- Cover Unix and Windows separators, encoded separators, and directory paths with source-location suffixes.
- Keep valid file paths and source locations unchanged.
- Do not guess whether paths without a trailing separator refer to directories; the existing backend regular-file check remains authoritative.
- Leave document previews and terminal links outside this change.

## Risks

Checking before decoding or suffix removal would allow disguised directory destinations. Regression tests must cover both.

## Release criteria

- Parser and transcript-rendering regressions demonstrate no file-opening link for directory paths.
- Valid file links remain supported.
- Frontend tests, type check and production build pass; record checks not run.

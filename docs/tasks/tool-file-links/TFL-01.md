# TFL-01 — Open a file tool's path from its card header

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** None
- **Plan:** [tool-file-links.md](../../plan/tool-file-links.md)

## Scope

Add `toolFileTarget` to find the file of a file tool call, show that path as a
button in the tool card header, and open it through the message file opener.

## Likely files

- `frontend/src/lib/toolPresentation.ts`
- `frontend/src/lib/components/transcript/parts/ToolPart.svelte`
- `frontend/src/lib/components/transcript/MessageBubble.svelte`
- `frontend/src/lib/components/chat/LiveTail.svelte`
- `frontend/src/lib/components/transcript/TranscriptView.svelte`
- `frontend/test/tool-presentation.test.mjs`
- `frontend/test/tool-card.test.mjs`

## Acceptance criteria

- Read, Edit, Write, MultiEdit and NotebookEdit (Claude Code) and read, edit
  and write (OpenCode) cards show their path as a button; clicking it opens
  the file in the file viewer.
- Claude Code's Read with `offset` opens at that line.
- Grep, Glob, Bash and other tools keep a plain-text summary.
- A long path still truncates and leaves the status badge in place.
- The button can be reached with the keyboard and has a focus ring.

## Tests

Unit tests for `toolFileTarget` and tool-card tests for the button. Run the
frontend tests, `npm run check` and `npm run build`. Record what ran in
Evidence.

## Evidence

Done:

- `toolFileTarget(name, input)` in `toolPresentation.ts`: `file_path`,
  `filePath` or `notebook_path` for any tool; `path` only for read, edit,
  write, multiedit and notebookedit. Paths that are empty, contain control
  characters or end in a separator give nothing. Claude Code's `Read` with a
  positive whole `offset` gives that line. The path is not URI-decoded.
- `ToolPart` shows the path as a `button.tool-summary.tool-file-link`
  (tooltip `Open <path>`) when it has `onOpenFile`; otherwise the summary
  stays a `span`, as before. The button keeps the summary's truncation rule
  and the card's `button:focus-visible` ring; hover shows the accent color
  and an underline.
- `MessageBubble` and `LiveTail` pass `onOpenFile` down. In `TranscriptView`,
  `openMessageFile` now parses and calls `openSessionFile`, which holds the
  old opening code; tool cards call it through `openToolFile` with the
  undecoded target.

Verified (from `frontend/`):

- `node --test test/tool-presentation.test.mjs test/tool-card.test.mjs`:
  16 pass. New tests: file targets for Claude Code, OpenCode, notebooks,
  string input, ACP-style names; `path` refused for Grep/Glob; directories,
  blanks and control characters refused; Read's offset (and no line for 0,
  negative, fractional, string or OpenCode offsets); `%25` kept. Card: the
  button's text and tooltip, a click passes `{ path, line }`, no button
  without the callback or for a Bash command.
- `npm test`: 1469 tests, 1468 pass, 0 fail, 1 skipped (already skipped).
- `npm run check`: 0 errors, 0 warnings. `npm run build`: succeeds (only the
  usual chunk-size warning).

Not checked: a click in a browser or the desktop app, which would cover
`openToolFile` → opener → file viewer and the header layout with a long path.
Installing Playwright for the mock app was not allowed in this session. That
is [TFL-02](TFL-02.md).

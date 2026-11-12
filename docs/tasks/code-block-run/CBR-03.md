# CBR-03 — Run button on shell code blocks

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CBR-02
- **Plan:** [code-block-run.md](../../plan/code-block-run.md)

## Scope

- `lib/runCommand.ts`: `runnableCommand(lang, code)` (shell languages,
  control characters removed, a leading `!` and uniform `$ ` prompts
  removed, trimmed).
- `renderMarkdown(…, { runnable })` adds
  `<button class="run-code-btn" data-command="…">Run</button>` before Copy.
- `handleMessageClick` calls `onRunCommand` for it; `TextPart`,
  `MessageBubble` (assistant text only), `TranscriptView` and `LiveTail`
  pass the handler where the panel is available and there is a directory.
- The button's style matches Copy.

## Likely files

- `frontend/src/lib/runCommand.ts`, `markdown.ts`, `messageFileClick.ts`
- `frontend/src/lib/components/transcript/parts/TextPart.svelte`,
  `MessageBubble.svelte`, `TranscriptView.svelte`,
  `frontend/src/lib/components/chat/LiveTail.svelte`, tests

## Acceptance criteria

- Shell blocks with a command get Run left of Copy only when `runnable`;
  other languages, empty commands and other renderers do not.
- A click runs the cleaned command in the session's (or chat's) directory.

## Tests

- `npm test`, `npm run check`

## Evidence

- `lib/runCommand.ts` `runnableCommand`: `bash`, `sh`, `shell`, `zsh`
  (any case); removes C0 controls except tab and newline (so CR too), DEL
  and C1; trims; removes one leading `!` and the spaces after it; removes
  `$ ` prompts (a bare `$` line counts) when every non-empty line has one.
- `renderMarkdown(source, fileLinks, headingIds, { runnable })` wraps Run
  and Copy in `.code-block-actions` (as Mermaid blocks do) when a block has
  a command; without the option the output is unchanged. Quoted blocks
  count. `handleMessageClick(event, onFileLink, onRunCommand)` handles
  `.run-code-btn` inside the message.
- `TextPart` renders runnable only with an `onRunCommand`; `MessageBubble`
  passes it to assistant text parts only (not reasoning); `TranscriptView`
  gives it where `launcher.info.panel` is set and the session's directory
  is known (`cwd`, not `cwdMissing`), and to `LiveTail` with `chat.dir`.
  Run uses Copy's styles.
- `npm test`: 1420 tests, 1419 pass, 0 fail, 1 skipped. New
  `test/code-block-run.test.mjs`: languages and empty commands; control
  characters, including an ESC that would end a bracketed paste; the
  leading `!` (with the command from the bug report); prompts, uniform or
  not; Run before Copy only on shell blocks with a command, only when
  runnable, escaped, never from raw HTML; the click handler; a server-
  rendered `TextPart` with and without the handler; the wiring in the
  components. `test/type-scale.test.mjs` now names the Run selector too.
- `npm run check`: 0 errors, 0 warnings; `npm run build` passes.
- Not checked: the button in the desktop app, and the command running in
  a real shell tab (CBR-04).

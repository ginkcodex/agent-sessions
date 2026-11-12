# CBR-03 — Run button on shell code blocks

- **Status:** ⬜ Not started
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

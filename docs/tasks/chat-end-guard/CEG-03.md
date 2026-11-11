# CEG-03 — End chat asks while the agent works

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** CEG-01
- **Plan:** [chat-end-guard.md](../../plan/chat-end-guard.md)

## Scope

- `ChatComposer.svelte`: End chat asks through `filePrompt.ask` while the
  phase is `working`; an idle chat ends at once.
- The rule as a pure function, so it is tested without the dialog.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`, `chat.ts`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria

- Working: asks "End the chat?"; cancel keeps the chat, End chat ends it.
- Idle: no question.

## Tests

- Unit tests for the rule and the prompt's text; `npm test`, `npm run check`.

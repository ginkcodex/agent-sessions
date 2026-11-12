# CEG-03 — End chat asks while the agent works

- **Status:** ✅ Done
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

## Evidence

- `chat.ts`: `endChatQuestion(phase, agentName)`, the question for a
  `working` chat, null otherwise. `ChatComposer.svelte`: End chat asks
  through `filePrompt.ask` (the app's prompt dialog, always mounted with
  `GoToFileDialog`) and ends the chat only on "End chat".
- `chat-ui.test.mjs`: no question when idle or starting; the title, message
  and danger choice when working. `node --test test/chat-ui.test.mjs`: 23
  passed. `npm run check`: 0 errors.

Not checked: the dialog in the desktop app, and the click path itself (the
SSR tests do not run handlers) (CEG-05).

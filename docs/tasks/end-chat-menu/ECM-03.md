# ECM-03 — Switch to terminal

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** ECM-02
- **Plan:** [end-chat-menu.md](../../plan/end-chat-menu.md)

## Scope

- `frontend/src/lib/chat.ts`: pure `switchToTerminalQuestion(phase,
  agentName)` with the switch wording.
- `frontend/src/lib/stores/chatOpen.svelte.ts`: `switchToTerminal(chat,
  meta)` — close the chat, dismiss it, resume the session in a panel
  terminal tab.
- `frontend/src/lib/components/transcript/TranscriptView.svelte`: wire
  `onSwitchToTerminal` to it, as `onRestart` is wired.
- Tests for the question, the switch sequence and the failure paths.

## Likely files

- `frontend/src/lib/chat.ts`
- `frontend/src/lib/stores/chatOpen.svelte.ts`
- `frontend/src/lib/components/transcript/TranscriptView.svelte`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria and tests

- A working chat asks "Switch to terminal?" first; cancelling changes
  nothing. An idle chat switches at once.
- The sequence is close, then dismiss (no Dismiss/Restart), then resume in
  a focused panel tab with the session's directory and title.
- A failed close keeps the chat and its ended UI; a failed resume says so
  in a toast and the session stays saved.

## Evidence

- `frontend/test/chat-ui.test.mjs`: "Switch to terminal asks while the
  agent works, worded for switching" (the pure question's wording).
- `frontend/test/chat-end-menu.test.mjs`: "Switch to terminal asks while
  working and switches at once when idle", "switching to a terminal stops
  the chat and resumes the session in a panel tab" (close → dismiss →
  `runResume` with the meta's directory and title), "a failed close keeps
  the chat; a failed resume says so and leaves the session saved" — all
  pass.
- The exit-before-close ordering that makes the auto-dismiss safe was
  verified in `internal/engine/chat.go` (`watch` sends `chat:exit` before
  `close(ch.done)`; `Close` waits on `done`) and recorded in the plan.
- `npm test` 1369 tests, 0 failures (1 pre-existing skip); `npm run check`
  clean; `npm run build` succeeds.

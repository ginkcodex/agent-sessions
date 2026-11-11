# ACP-10 — Permission cards and failure paths

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `PermissionCard.svelte`: the tool call title and kind, a diff or command
  preview when present, and one button per option, labelled with its kind.
  There is no default option and Enter does not approve. `allow_always`
  options say that the choice lasts beyond this chat (Codex's
  `accept_execpolicy_amendment` writes an exec-policy rule).
- After an answer, the chat follows the turn's `stopReason`: a reject ends
  the turn `cancelled` for Codex and OpenCode, and Claude goes on.
- Fail-closed paths: closing the chat, cancelling, the agent exiting, a host
  switch and quitting all clear pending cards. The backend answers them
  `cancelled`.
- `AdapterMissing.svelte`: the install command to copy, and the host it
  applies to.
- An exit or error banner with the stderr tail, and Restart.
- A reloaded page loses the permission cards: `ChatList` returns no pending
  requests, so the store (ACP-08) finds the chat but not its cards. Add the
  pending requests to `ChatState` (engine and binding) and seed the queue
  from them in `chats.init()`, or cancel them on reload.
- An auth error shows the agent's `authMethods` description (for example
  "Run `opencode auth login`") with an action that opens a terminal tab in
  the chat's directory.

## Likely files

- `frontend/src/lib/components/chat/`
- `frontend/src/lib/stores/chats.svelte.ts`

## Acceptance criteria

- No path approves a permission without a click on an allow option.
- A missing adapter never starts a process.

## Tests

Component tests for card rendering, keyboard behavior and each clearing path.
Go tests (in ACP-06) cover the backend side.

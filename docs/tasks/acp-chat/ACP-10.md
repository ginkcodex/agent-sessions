# ACP-10 — Permission cards and failure paths

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `PermissionCard.svelte`: the tool call title and kind, a diff or command
  preview when present, and one button per option, labelled with its kind.
  There is no default option and Enter does not approve.
- Fail-closed paths: closing the chat, cancelling, the agent exiting, a host
  switch and quitting all clear pending cards. The backend answers them
  `cancelled`.
- `AdapterMissing.svelte`: the install command to copy, and the host it
  applies to.
- An exit or error banner with the stderr tail, and Restart.

## Likely files

- `frontend/src/lib/components/chat/`
- `frontend/src/lib/stores/chats.svelte.ts`

## Acceptance criteria

- No path approves a permission without a click on an allow option.
- A missing adapter never starts a process.

## Tests

Component tests for card rendering, keyboard behavior and each clearing path.
Go tests (in ACP-06) cover the backend side.

# HSG-02 — Connecting, switching and disconnecting ask before ending open chats

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** HSG-01
- **Plan:** [host-switch-chat-guard.md](../../plan/host-switch-chat-guard.md)

## Scope

- `closeConfirm.ts`: `endingChatsWarning(chats, action)`, sharing the
  chat summary and the named list with `closeWindowWarning`.
- `ConnectionStore`: `changeHost` reads the open chats while the window's
  backend serves (`!link.locked`); asks "End open chats?" when no
  previous-host terminal is open, else adds the chat warning to the
  terminal question and relabels its buttons. Connecting again to the
  serving host asks too; a retry of a dropped host does not.
- The Coding agents reconnect prompt names the host's chats.

## Likely files

- `frontend/src/lib/closeConfirm.ts`
- `frontend/src/lib/stores/connection.svelte.ts`
- `frontend/src/lib/components/common/CodingAgentsSettings.svelte`
- `frontend/test/close-confirm.test.mjs`, `frontend/test/connection.test.mjs`

## Acceptance criteria

- With an open chat, connect, switch and disconnect ask; cancel leaves the
  connection alone; confirm switches.
- Terminals and chats ask once; the terminal choice still works.
- No question for exited chats or a dropped host.

## Tests

- `close-confirm.test.mjs`: the switch warning's wording.
- `connection.test.mjs`: the questions, their buttons, cancel and confirm.
- `npm test`, `npm run check`, `npm run build`.

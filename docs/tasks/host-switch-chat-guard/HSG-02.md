# HSG-02 — Connecting, switching and disconnecting ask before ending open chats

- **Status:** ✅ Done
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

## Evidence

What was done:

- `closeConfirm.ts`: `endingChatsWarning(chats, action)`; the chat summary,
  the chat names and the "and N more" list are now shared with
  `closeWindowWarning`, whose wording is unchanged.
- `ConnectionStore.endingChats` words the warning while the window's
  backend serves (`!link.locked`): "Connecting to X", "Disconnecting from
  X" or "Reconnecting to X". `changeHost` asks "End open chats?" with one
  danger button when no previous-host terminal is open, else adds the
  warning under the terminal question, whose buttons become "End chats,
  keep terminal tabs" and "End chats and terminal tabs". `connect` to the
  serving host asks the same; a retry of a dropped host goes straight on.
- The Coding agents reconnect prompt says it ends the host's open chats.

Verified:

- `close-confirm.test.mjs`: the switch warning for one working chat, for
  several (a starting one counted, exited ones left out), and past six
  names; the close warning tests pass unchanged.
- `connection.test.mjs`: Local → host, host → host and host → Local ask
  with the right action and button; cancel does not connect or disconnect,
  confirm does. Terminals and a chat ask once, with the chat warning and
  the new labels; keep, close and cancel behave as before. A locked
  (dropped) host and exited chats do not ask. Connecting again to the
  serving host asks "Reconnecting to box". The existing terminal tests pass
  unchanged.
- `npm test`: 1353 tests, 1352 passed, 1 skipped. `npm run check`:
  0 errors, 0 warnings. `npm run build`: built.

Not checked: the questions in the desktop app with real chats (HSG-03).

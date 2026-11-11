# Host switch chat guard

## Goal

Connecting to a host, switching to another one, or disconnecting back to
this computer asks about the previous host's terminal tabs before it goes
ahead. It does not ask about the window's chats, though the switch always
ends them:

- leaving this computer, the backend's `connection.leaveLocal` ends the
  window's local chats;
- leaving a host, the old connection closes and the host's server ends its
  chats (`rpc.Server.closeChats`).

A user can lose a working chat with one click in the host menu. The switch
should ask first, as it does for terminals and as closing the window does
for chats ([chat-end-guard](chat-end-guard.md)).

## Design

`ConnectionStore.changeHost` (`frontend/src/lib/stores/connection.svelte.ts`)
already asks about the previous host's terminal tabs, after the unsaved
file check. It now also reads the window's chats from the chats store:

- **Only chats open:** "End open chats?", with the warning
  "A chat is open and its agent is working. Connecting to box ends it:" and
  a list naming each chat, as the close warning does. One danger button,
  "End chats and connect" (or "…and disconnect"); dismissing cancels the
  switch.
- **Terminals and chats:** the terminal question stays, with the chat
  warning added under it, and its buttons say that chats end either way:
  "End chats, keep terminal tabs" and "End chats and terminal tabs".
- **Connecting again to the host that serves the window** (typing its
  name in the host menu) also ends its chats: it asks the same way, with
  "Reconnecting to box" and "End chats and reconnect".

The chat wording lives in `closeConfirm.ts` beside the close warning:
`endingChatsWarning(chats, action)` shares the summary line and the named
list with `closeWindowWarning`.

The Coding agents settings' reconnect prompt says that reconnecting ends
the host's open chats too.

## Decisions

1. **Ask, do not offer to keep chats.** A chat cannot outlive the switch:
   its agent runs under the connection (or the window's local chat
   manager) that the switch ends.
2. **One question when terminals are open too**, not two dialogs in a row.
3. **No question when the chats are already gone.** A dropped host
   (`link.locked`: reconnecting or disconnected) has no live chats, and
   neither has Local while a connect is pending; a retry from the
   reconnect banner never asks. Exited chats are left out.
4. **Starting chats count**, as for the window close: the start is
   refused when it answers after a switch.

## Risks

- The chats store could list a chat whose agent just ended before its exit
  event arrived: the question then names a chat already gone. Harmless.
- `filePrompt.ask` answers null while another prompt is open; the switch
  then cancels, and the user tries again.

## Release criteria

- With an open chat, choosing another host, Local, or a WSL distribution
  asks; cancelling keeps the connection and the chat; confirming switches.
- With previous-host terminal tabs and a chat, one question covers both.
- Without chats, the terminal question and a plain switch are unchanged.
- A retry of a dropped host does not ask.

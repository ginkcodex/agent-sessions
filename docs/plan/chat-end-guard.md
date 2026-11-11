# Chat end guard

## Goal

Ending a chat should be quiet when the user ends it, and never by accident
while the agent works:

- **A quiet exit notice.** Ending a chat with End chat shows the agent's
  stderr tail (its own debug log) under "The chat ended.". The user asked
  for just "The chat ended.". The log helps only when the agent stopped by
  itself.
- **End chat asks while the agent works.** A click on End chat during a
  turn stops the turn and the agent at once. It should ask first.
- **Closing the window or quitting asks about open chats.** Today the close
  guard asks only about running terminals and unsaved files; a window with
  a live chat closes, or the app quits, and ends the chat without a word.

## Design

### Exit notice

`LiveTail.svelte` shows the exit reason. The stderr tail shows only for an
agent that quit or crashed by itself (`exited`, `crashed`), folded under a
"Show the agent's log" disclosure. A chat the user ended (`closed`) or that
a dropped connection ended (`disconnected`) shows one line.

### End chat

`ChatComposer.svelte`'s End chat asks through the app's prompt dialog
(`filePrompt.ask`) while the chat's phase is `working` (a turn runs, or a
permission request waits): "End the chat?", "<Agent> is still working.
Ending the chat stops its turn.", with a danger "End chat" button. An idle
chat ends at once, as today: nothing is lost, the session is saved.

### Window close and quit

The native close guard (`fileCloseGuard.busy`) is what sends a close or a
quit to the frontend to ask. It reads the terminals only. It now also
reads the window's chats:

- this computer's chats from the local chat manager (`ChatManager.Any`, a
  map length under its mutex: it runs on the UI thread and must not wait);
- a host's chats, which run on the host, from a flag the frontend sets
  through a new binding, `SetChatsOpen(open bool)`, whenever the window's
  set of open chats changes from none to some or back.

The frontend's close confirmation (`closeConfirm.ts`) words one warning for
both: running terminals as today, then the open chats ("A chat is open and
its agent is working", "2 chats are open, 1 of them working"), and a list
naming each ("Chat with Claude Code in agent-sessions (working)"). The
restart prompt in Coding agents settings names chats too.

## Decisions

1. **Any open chat asks on window close**, not only a working one, as any
   running terminal does: the user may be about to write the next prompt.
2. **End chat asks only while working.** Ending an idle chat loses nothing.
3. **The frontend reports host chats** rather than the guard asking the
   host: the guard runs on the UI thread and cannot wait on the network.
   This computer's chats are read in Go as well, so a window whose
   frontend has not reported yet still asks.
4. **The stderr tail stays available** for an agent that stopped by itself,
   folded, since it is how a crash is diagnosed.

## Risks

- The flag could lag a chat that just started. The local check covers this
  computer; for a host, the frontend sets the flag as soon as the start
  answers, before the user could close.
- A prompt dialog already open (another decision) makes `filePrompt.ask`
  answer null: End chat then does nothing, and the user tries again.

## Release criteria

- Ending a chat shows "The chat ended." alone; a crash still shows the log,
  folded.
- End chat during a turn asks; cancelling keeps the chat; confirming ends
  it. End chat on an idle chat does not ask.
- Closing a window or quitting with an open chat, local or on a host, asks;
  cancelling keeps the window and the chat.
- Tests: SSR for the notice, composer and close warning; Go tests for the
  guard; `npm test`, `npm run check`, `npm run build`, `go test -race
  ./internal/app/... ./internal/engine/...`.

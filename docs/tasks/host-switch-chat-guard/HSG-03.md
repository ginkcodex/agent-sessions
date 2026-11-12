# HSG-03 — QA in the desktop app

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** HSG-02
- **Plan:** [host-switch-chat-guard.md](../../plan/host-switch-chat-guard.md)

## Scope

In the desktop app:

- A local chat open (idle, then working): choose an SSH host or a WSL
  distribution. The question names the chat; cancel keeps the chat live;
  confirm connects and the chat is gone.
- A chat on a host: choose Local, then another host. Same checks.
- A chat and a previous-host terminal tab: one question; both buttons.
- A dropped host with a chat: Retry and Local do not ask about chats.

## Acceptance criteria

- Every case above behaves as the plan's release criteria say.

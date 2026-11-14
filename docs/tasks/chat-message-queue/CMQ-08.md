# CMQ-08 — QA in the desktop app

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** CMQ-07
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

In the desktop app, with Claude Code, Codex and OpenCode chats, locally and
on a WSL or SSH host:

- queue two messages during a long turn; they run in order after it;
- Stop with a message queued: the queue pauses; Send now resumes it;
- Send now during a turn: the agent answers it in the same turn, and the
  saved transcript shows the message;
- Edit and Remove a queued message, with and without an image;
- reload the window during a turn with a queued message: the queue is back;
- End chat with queued messages asks.

## Acceptance criteria

- Each check above passes on each agent, or the failure is a new task.

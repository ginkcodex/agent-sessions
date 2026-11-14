# Chat message queue — tasks

- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)
- **Progress:** 7/8 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CMQ-01](CMQ-01.md) | ✅ Done | Plan and task docs, index rows | 0.5 h | None |
| [CMQ-02](CMQ-02.md) | ✅ Done | ACP client: steering capability and `_session/steering`; the fake agent steers | 2 h | CMQ-01 |
| [CMQ-03](CMQ-03.md) | ✅ Done | Engine queue: Prompt queues while working, sent after a normal end, paused otherwise; Unqueue; state and events | 4 h | CMQ-01 |
| [CMQ-04](CMQ-04.md) | ✅ Done | Engine Send now: steer into the running turn, or start the turn when idle | 3 h | CMQ-02, CMQ-03 |
| [CMQ-05](CMQ-05.md) | ✅ Done | RPC and app bindings: `chatQueue` capability, `chatPrompt` result, `chatSendNow`, `chatUnqueue` | 3 h | CMQ-04 |
| [CMQ-06](CMQ-06.md) | ✅ Done | Frontend store, API and mock: queue state, queued turn starts, steered messages in the live tail | 3 h | CMQ-05 |
| [CMQ-07](CMQ-07.md) | ✅ Done | Frontend UI: queue from the composer, queued messages with Send now, Edit and Remove; End chat asks | 3 h | CMQ-06 |
| [CMQ-08](CMQ-08.md) | ⬜ Not started | QA in the desktop app with Claude Code, Codex and OpenCode | 1.5 h | CMQ-07 |

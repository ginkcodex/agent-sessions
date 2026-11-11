# Chat end guard — tasks

- **Plan:** [chat-end-guard.md](../../plan/chat-end-guard.md)
- **Progress:** 3/5 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CEG-01](CEG-01.md) | ✅ Done | Plan and task docs, index rows | 0.5 h | None |
| [CEG-02](CEG-02.md) | ✅ Done | Exit notice: one line when the user ends the chat; the agent's log folded, only when it stopped by itself | 1 h | CEG-01 |
| [CEG-03](CEG-03.md) | ✅ Done | End chat asks while the agent works | 1 h | CEG-01 |
| [CEG-04](CEG-04.md) | ⬜ Not started | Window close and quit ask about open chats: close guard, `SetChatsOpen`, warning text | 3 h | CEG-01 |
| [CEG-05](CEG-05.md) | ⬜ Not started | QA in the desktop app: end, close and quit with a live chat | 1 h | CEG-02, CEG-03, CEG-04 |

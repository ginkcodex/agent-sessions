# Chat questions — tasks

- **Plan:** [chat-questions.md](../../plan/chat-questions.md)
- **Progress:** 4/7 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CQ-01](CQ-01.md) | ✅ Done | Plan, adapter notes, task docs, index rows | 0.5 h | None |
| [CQ-02](CQ-02.md) | ✅ Done | `elicitation/create`, `$/cancel_request`, schema check, answer validation in `internal/acp` | 4 h | CQ-01 |
| [CQ-03](CQ-03.md) | ✅ Done | Questions per chat, `question` updates, `Answer`, cancel on every exit path | 3 h | CQ-02 |
| [CQ-04](CQ-04.md) | ✅ Done | `ChatAnswer` binding, `chatAnswer` RPC, `ChatQuestions` capability | 2 h | CQ-03 |
| [CQ-05](CQ-05.md) | ⬜ Not started | `chatQuestions.ts`, the store's pending questions, api, mock agent | 4 h | CQ-04 |
| [CQ-06](CQ-06.md) | ⬜ Not started | Tabs, single and multi choice, own answer, Review and Submit, Skip, keyboard | 4 h | CQ-05 |
| [CQ-07](CQ-07.md) | ⬜ Not started | Real questions from Claude Code, Codex and OpenCode in a chat | 2 h | CQ-06 |

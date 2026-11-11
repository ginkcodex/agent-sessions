# ACP chat mode — tasks

- **Plan:** [acp-chat.md](../../plan/acp-chat.md)
- **Progress:** 5/15 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [ACP-01](ACP-01.md) | ✅ Done | Plan, task group, SPEC and M8 non-goal reversal | 1.5 h | — |
| [ACP-02](ACP-02.md) | ✅ Done | Spike with the real adapters: capabilities, storage, IDs, load and resume | 4 h | ACP-01 |
| [ACP-03](ACP-03.md) | ✅ Done | `internal/acp` protocol types and two-way JSON-RPC connection | 4 h | ACP-02 |
| [ACP-04](ACP-04.md) | ✅ Done | Agent process, tree kill, client lifecycle, fake agent | 4 h | ACP-03 |
| [ACP-05](ACP-05.md) | ✅ Done | Adapter commands, detection and overrides per provider | 3 h | ACP-04 |
| [ACP-06](ACP-06.md) | ⬜ Not started | Engine chat manager: owners, events, permissions, cleanup | 4 h | ACP-05 |
| [ACP-07](ACP-07.md) | ⬜ Not started | Local bindings, quit cleanup, manage lock | 3 h | ACP-06 |
| [ACP-08](ACP-08.md) | ⬜ Not started | Frontend API, mock agent, chat store and live-tail reducer | 4 h | ACP-07 |
| [ACP-09](ACP-09.md) | ⬜ Not started | Chat UI, entry points, terminal/chat exclusivity | 4 h | ACP-08 |
| [ACP-10](ACP-10.md) | ⬜ Not started | Permission cards, fail-closed paths, missing adapter | 3 h | ACP-09 |
| [ACP-11](ACP-11.md) | ⬜ Not started | Reconcile with saved transcripts; continue existing sessions | 4 h | ACP-10 |
| [ACP-12](ACP-12.md) | ⬜ Not started | Remote chat over RPC with connection ownership | 4 h | ACP-06 |
| [ACP-13](ACP-13.md) | ⬜ Not started | Remote routing in the app and frontend | 3 h | ACP-12, ACP-09 |
| [ACP-14](ACP-14.md) | ⬜ Not started | QA with real agents on Linux, WSL, SSH and Windows | 4 h | ACP-11, ACP-13, ACP-15 |
| [ACP-15](ACP-15.md) | ⬜ Not started | Session option pickers: mode, model, effort | 3 h | ACP-09 |

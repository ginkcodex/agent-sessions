# Chat login environment — tasks

- **Plan:** [chat-login-env.md](../../plan/chat-login-env.md)
- **Progress:** 3/4 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CLE-01](CLE-01.md) | ✅ Done | Plan and task docs, index rows | 0.5 h | None |
| [CLE-02](CLE-02.md) | ✅ Done | Shared `internal/loginenv` reader: `/etc/passwd` shell fallback, home directory, process-group kill; terminal tabs use it | 1.5 h | CLE-01 |
| [CLE-03](CLE-03.md) | ✅ Done | `ChatLaunch` lays the login environment over the process's own | 1.5 h | CLE-02 |
| [CLE-04](CLE-04.md) | ⬜ Not started | QA: a mise tool in a chat on this computer and on a WSL host | 0.5 h | CLE-03 |

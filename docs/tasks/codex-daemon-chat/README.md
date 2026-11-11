# Codex daemon chat — tasks

- **Plan:** [codex-daemon-chat.md](../../plan/codex-daemon-chat.md)
- **Progress:** 4/5 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CDC-01](CDC-01.md) | ✅ Done | Plan, spike notes, task docs, index rows | 0.5 h | None |
| [CDC-02](CDC-02.md) | ✅ Done | A clear "session in use" error with Try again, locally and on a host | 2 h | CDC-01 |
| [CDC-03](CDC-03.md) | ✅ Done | `internal/codexbridge`: stdio to the Codex daemon's WebSocket, or the real `codex` | 3 h | CDC-01 |
| [CDC-04](CDC-04.md) | ✅ Done | Launch Codex chats through the bridge; both binaries run it | 2 h | CDC-03 |
| [CDC-05](CDC-05.md) | ⬜ Not started | QA in the desktop app: chat right after the CLI quits, no daemon, a host, a prompt turn | 1 h | CDC-02, CDC-04 |

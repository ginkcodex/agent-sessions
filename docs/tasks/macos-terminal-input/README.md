# macOS terminal input — tasks

- **Plan:** [macos-terminal-input.md](../../plan/macos-terminal-input.md)
- **Progress:** 1/3 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [MTI-01](MTI-01.md) | ➖ Superseded by MTI-03 — cause was the remote locale, not frontend routing | Reproduce native Unicode injection, fix terminal routing and regression tests | 3 h | None |
| [MTI-02](MTI-02.md) | 🟡 Partial — user confirmed remote typing; EVKey/GoTiengViet, Linux/Windows unverified | Native third-party Vietnamese IME and cross-platform regression QA | 1 h | MTI-01 |
| [MTI-03](MTI-03.md) | ✅ Done | Installed UTF-8 fallback for unconfigured remote plain shells | 2 h | None |

# M8 — Windows support (≈ 62 h) — tasks

- **Plan:** [M8.md](../../plan/M8.md)
- **Progress:** 2/13 done

Local-first (M8-01…07), then
key/agent-only SSH (M8-09, requested 2026-10-04) and WSL distributions as
remotes (M8-13, approved 2026-10-04). Password/passphrase dialogs remain
gated on the M8-08 named-pipe credential channel.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M8-01](M8-01.md) | ✅ Done | `internal/paths` Windows `FromEnv` | 3 h | — |
| [M8-02](M8-02.md) | 🟡 Partial | `explorer /select` for Reveal and `rundll32`/`explorer` for OpenURL via a per-GOOS… | 2 h | M8-01 |
| [M8-03](M8-03.md) | ✅ Done | Windows path audit: grouping keys/labels, `pathutil` normalize + git resolver, Claude… | 6 h | M8-01 |
| [M8-04](M8-04.md) | 🟡 Partial | Live-session detection fallback: sessions never show live | 1 h | M8-01 |
| [M8-05](M8-05.md) | 🟡 Partial | Recycle Bin via native `IFileOperation` with recycle-only guards | 5 h | M8-01 |
| [M8-06](M8-06.md) | 🟡 Partial | PowerShell command quoting for resume/handoff commands | 4 h | M8-01 |
| [M8-07](M8-07.md) | 🟡 Partial | CI `windows-latest` job | 6 h | M8-01 |
| [M8-08](M8-08.md) | ⬜ Not started | Askpass over a per-user named pipe with DACL | 8 h | M8-07 |
| [M8-09](M8-09.md) | 🟡 Partial | Native Windows SSH without ControlMaster: persistent stdio RPC session | 5 h | M8-01 |
| [M8-10](M8-10.md) | ⬜ Not started | QA: real-machine pass over scan, search, export/import, handoff, Recycle Bin delete… | 6 h | M8-01…07 |
| [M8-11](M8-11.md) | 🟡 Partial | Open in terminal on Windows for local Resume and Continue in | 5 h | M8-06 |
| [M8-12](M8-12.md) | 🟡 Partial | Open in terminal on Linux and macOS: detect installed terminal apps, choose one in… | 5 h | M8-11 |
| [M8-13](M8-13.md) | 🟡 Partial | WSL as a remote: the Windows host picker lists the user's registered distributions | 6 h | M8-09, M8-11, M8-12 |

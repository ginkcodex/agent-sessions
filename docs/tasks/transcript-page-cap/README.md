# Transcript page cap — tasks

- **Plan:** [transcript-page-cap.md](../../plan/transcript-page-cap.md)
- **Progress:** 4/6 done

Planned 2026-11-07. PC-01 measured the cause. The engine cap (PC-02) comes
before the RPC and app plumbing (PC-03), which the viewer (PC-04) uses.
PC-05 measures again on `plgl`; PC-06 checks the desktop app there. Each
task ends with `go test` / `npm test` for what it touches passing and no
new `golangci-lint` findings.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [PC-01](PC-01.md) | ✅ Done | Measure first transcript pages by provider | 1 h | — |
| [PC-02](PC-02.md) | ✅ Done | Engine pages capped by size around an anchor | 2 h | — |
| [PC-03](PC-03.md) | ✅ Done | Cap through RPC, offline backend and the app binding | 2 h | PC-02 |
| [PC-04](PC-04.md) | ✅ Done | Viewer asks for capped pages | 4 h | PC-03 |
| [PC-05](PC-05.md) | ⬜ Not started | Measure capped pages on `plgl` | 1 h | PC-02 |
| [PC-06](PC-06.md) | ⬜ Not started | Check in the desktop app on `plgl` | 1 h | PC-04 |

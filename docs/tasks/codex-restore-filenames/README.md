# Codex restore filenames — tasks

- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)
- **Progress:** 5/5 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CRF-01](CRF-01.md) | ✅ Done | Canonical future restore filenames and Go regressions | 2 h | None |
| [CRF-02](CRF-02.md) | ✅ Done | Offline repair utility, index references, backups, tests and instructions | 3 h | CRF-01 |
| [CRF-03](CRF-03.md) | ✅ Done | Native Go repair with backups, index updates and safety checks | 4 h | CRF-02 |
| [CRF-04](CRF-04.md) | ✅ Done | Engine, desktop and capability-gated remote repair API | 2 h | CRF-03 |
| [CRF-05](CRF-05.md) | ✅ Done | Affected-session notice, repair action and user-flow tests | 2 h | CRF-04 |

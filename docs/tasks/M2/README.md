# M2 — Index, search, live updates (≈ 36 h) — tasks

- **Plan:** [M2.md](../../plan/M2.md)
- **Progress:** 8/12 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M2-01](M2-01.md) | ✅ Done | Cache DB | 3 h | M1-13 |
| [M2-02](M2-02.md) | ✅ Done | Incremental indexing: skip unchanged files, resume from offset on append, rescan fully… | 4 h | M2-01, M0-10, M1-10 |
| [M2-03](M2-03.md) | ✅ Done | OpenCode incremental refresh via `time_updated >= cursor` plus the merged-ID checkpoint | 2 h | M2-01, M1-07 |
| [M2-04](M2-04.md) | ✅ Done | FTS5 index of titles and message text, updated incrementally with the M2-02/03 pipeline | 4 h | M2-02 |
| [M2-05](M2-05.md) | ✅ Done | Search API: `Search(query, filter)` → hits with snippet, session ref and message index | 3 h | M2-04 |
| [M2-06](M2-06.md) | ✅ Done | Search UI: `/` focuses the search box | 4 h | M2-05, M1-24 |
| [M2-07](M2-07.md) | ✅ Done | fsnotify watcher: watch provider roots | 4 h | M2-02 |
| [M2-08](M2-08.md) | ✅ Done | Push backend changes to the UI | 3 h | M2-07, M1-18, M1-19 |
| [M2-09](M2-09.md) | ⬜ Not started | Live tail in the transcript view | 3 h | M2-08, M1-24 |
| [M2-10](M2-10.md) | ⬜ Not started | Live badge refresh | 2 h | M0-14, M2-08 |
| [M2-11](M2-11.md) | 🟡 Partial | Diagnostics panel: provider roots, detected generations and versions, parse-error and… | 3 h | M1-13, M1-16 |
| [M2-12](M2-12.md) | 🟡 Partial | "Clear cache and rebuild" action | 1 h | M2-01 |

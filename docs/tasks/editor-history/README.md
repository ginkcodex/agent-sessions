# Editor back and forward — tasks

- **Plan:** [editor-history.md](../../plan/editor-history.md)
- **Progress:** 3/4 done

Planned 2026-11-07. The store's per-workspace history (EH-01) comes before
the viewer's places and steps (EH-02), which the mouse buttons (EH-03)
drive. EH-04 checks it in the desktop app. Each task ends with `npm test`
and `npm run check` passing.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [EH-01](EH-01.md) | ✅ Done | Per-workspace history of places in the files store | 3 h | — |
| [EH-02](EH-02.md) | ✅ Done | Viewer places, preview-aware steps, one record per jump | 3 h | EH-01 |
| [EH-03](EH-03.md) | ✅ Done | Mouse side buttons in Go to File | 2 h | EH-02 |
| [EH-04](EH-04.md) | ⬜ Not started | Check in the desktop app | 1 h | EH-03 |

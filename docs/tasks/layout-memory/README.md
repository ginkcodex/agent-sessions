# Layout memory — tasks

- **Plan:** [layout-memory.md](../../plan/layout-memory.md)
- **Progress:** 3/5 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [LL-01](LL-01.md) | ✅ Done | Content-based pane sizing: `layoutSizes.ts` header need and fit/clamp functions with unit tests | 2 h | — |
| [LL-02](LL-02.md) | ⬜ Not started | Wire sizing into the preferences store and splitters: header-derived transcript floor at open/resize, save on drag end | 2 h | LL-01 |
| [LL-03](LL-03.md) | ✅ Done | Window geometry memory in Go: `window-state.json` save on close/quit, restore clamped to the work area, maximised flag, per-role multi-window | 4 h | — |
| [LL-04](LL-04.md) | ⬜ Not started | Cross-platform QA and evidence: fresh install, restart round trip, monitor changes, maximised close | 2 h | LL-02, LL-03 |
| [LL-05](LL-05.md) | ✅ Done | Larger default navigation widths (sidebar 280, session list 380) so the column headers show in full | 0.5 h | — |

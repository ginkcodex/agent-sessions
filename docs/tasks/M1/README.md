# M1 — Remaining providers + MVP GUI (≈ 90 h) — tasks

- **Plan:** [M1-providers.md](../../plan/M1-providers.md), [M1-gui.md](../../plan/M1-gui.md)
- **Progress:** 23/31 done

## OpenCode

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M1-01](M1-01.md) | ✅ Done | Read-only SQLite helper | 3 h | M0-01 |
| [M1-02](M1-02.md) | ✅ Done | OpenCode `Detect` and generation detection | 2 h | M1-01, M0-04 |
| [M1-03](M1-03.md) | ✅ Done | OpenCode v2 scan: `session_v2` → `SessionMeta` | 3 h | M1-02 |
| [M1-04](M1-04.md) | ✅ Done | OpenCode v2 `Load`: `session_message` | 4 h | M1-03 |
| [M1-05](M1-05.md) | ✅ Done | OpenCode v1 scan and `Load`: `session` + `message` + `part` | 4 h | M1-02 |
| [M1-06](M1-06.md) | ⬜ Not started | OpenCode legacy JSON storage scan and `Load` | 4 h | M1-02 |
| [M1-07](M1-07.md) | 🟡 Partial | OpenCode merge: dedupe by id | 3 h | M1-03, M1-05, M1-06 |

## Codex

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M1-08](M1-08.md) | ⛔ Blocked | Obtain a real Codex rollout plus a populated `state_N.sqlite`, then sanitize them into… | 1 h | M0-06 |
| [M1-09](M1-09.md) | ✅ Done | Codex `Detect` and discovery | 2 h | M0-03, M0-04 |
| [M1-10](M1-10.md) | 🟡 Partial | Codex rollout metadata scan | 3 h | M1-09, M0-08, M1-08 |
| [M1-11](M1-11.md) | ✅ Done | Codex `threads` index fast path | 3 h | M1-01, M1-10 |
| [M1-12](M1-12.md) | 🟡 Partial | Codex `Load`: `response_item` | 4 h | M1-10 |

## Core

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M1-13](M1-13.md) | ✅ Done | Scan orchestrator: run all detected providers concurrently | 3 h | M0-03 |
| [M1-14](M1-14.md) | ✅ Done | Grouping engine | 3 h | M0-05, M0-02 |

## GUI (Wails + Svelte)

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M1-15](M1-15.md) | ✅ Done | Wails **v2** scaffold | 3 h | M0-01 |
| [M1-16](M1-16.md) | ✅ Done | App service bound via `options.App.Bind`: `ListGroups(mode, filter)`… | 3 h | M1-13, M1-14, M1-15 |
| [M1-17](M1-17.md) | ✅ Done | Frontend: three-pane shell, resizable splitters | 3 h | M1-15 |
| [M1-18](M1-18.md) | ✅ Done | Frontend: sidebar group tree | 3 h | M1-16, M1-17 |
| [M1-19](M1-19.md) | ✅ Done | Frontend: session list column | 4 h | M1-16, M1-17 |
| [M1-20](M1-20.md) | 🟡 Partial | Frontend: transcript viewer base | 4 h | M1-16, M1-17 |
| [M1-21](M1-21.md) | ✅ Done | Frontend: code highlighting | 2 h | M1-20 |
| [M1-22](M1-22.md) | ✅ Done | Frontend part renderers: collapsible reasoning, tool call + result pair | 4 h | M1-20 |
| [M1-23](M1-23.md) | ✅ Done | Frontend: session header | 2 h | M1-19 |
| [M1-24](M1-24.md) | 🟡 Partial | Progressive loading for big transcripts: paged `GetMessages`, load-on-scroll, first… | 4 h | M1-20 |
| [M1-25](M1-25.md) | ✅ Done | Actions: copy resume command | 2 h | M1-23 |
| [M1-26](M1-26.md) | 🟡 Partial | Empty, error and loading states | 2 h | M1-18, M1-19, M1-20 |
| [M1-27](M1-27.md) | ✅ Done | WebView hardening: strict CSP, external links open in the system browser, no remote… | 2 h | M1-20 |
| [M1-28](M1-28.md) | ✅ Done | MVP QA pass on real data: measure against the SPEC §8 perf targets, then file and fix… | 4 h | M1-01 → M1-27 |
| [M1-29](M1-29.md) | ✅ Done | Sidebar polish: node kinds *(retroactive)* | 3 h | M1-18 |
| [M1-30](M1-30.md) | ✅ Done | Theming: manual light/dark toggle, neutral gray dark theme, higher-contrast light… *(retroactive)* | 2 h | M1-17 |
| [M1-31](M1-31.md) | ✅ Done | Transcript "Bottom" jump loads the remaining pages *(retroactive)* | 1 h | M1-24 |

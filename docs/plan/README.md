# Implementation plans

Detailed per-task plans for [TASKS.md](../../TASKS.md). Read them in order:
later milestones build on the contracts defined in M0.

| File | Covers |
|---|---|
| [M0.md](M0.md) | Conventions, **shared contracts** (model, provider interface, JSONL reader, path utils), Claude Code provider, CLI |
| [M1-providers.md](M1-providers.md) | OpenCode and Codex providers, scan orchestrator/catalog, grouping engine |
| [M1-gui.md](M1-gui.md) | Wails v2 + Svelte 5 app: bindings, `api.ts`, layout, session list, transcript viewer, security, QA |
| [M2.md](M2.md) | Cache index, incremental scans, FTS search, file watching, live updates, diagnostics |
| [M3-M5.md](M3-M5.md) | Filters, config, export, keyboard, stats; destructive actions; macOS port, packaging, CI |
| [M6.md](M6.md) | Portable bundles (export, import, opt-in restore) and cross-agent handoff; bundle manifest v1 contract |
| [M7.md](M7.md) | Remote over SSH: headless server via system SSH, browse, search, manage remote sessions |
| [M8.md](M8.md) | Windows support: local-first port (paths, Recycle Bin, PowerShell, packaging); remote over SSH later |
| [file-editing.md](file-editing.md) | Go to File editing: FE-01…FE-14 task checklist, safe local/remote saves, drafts, conflicts, and lifecycle guards |
| [code-diagnostics.md](code-diagnostics.md) | Proposed Go to File diagnostics: CD-01…CD-28, project checks, live language servers, trust, Problems panel, and local/remote lifecycle |
| [code-diagnostics-languages.md](code-diagnostics-languages.md) | Proposed: choose in Settings which languages get code checks and Go to Definition; a language registry, filtered discovery |
| [editor-essentials.md](editor-essentials.md) | Project search and replace, new agent sessions and alerts, code navigation, Git changes: PS, AG, NAV and GIT tasks |
| [git-history.md](git-history.md) | Implemented read-only Git history: IntelliJ-inspired multi-pane layout, repository/branch selection, commit search, historical diffs; GH-01…GH-18 |
| [performance.md](performance.md) | Performance roadmap: milestones 1–5 and what is next. M1 now 13/15 (PF-08 superseded, PF-10A/B complete); startup package and M2 complete; M3 automated gate complete; M4 needs design |
| [performance-m2.md](performance-m2.md) | Performance Milestone 2 design (complete 12/12): incremental transcripts, Codex resolution, byte-budgeted cache, FTS reuse, transcript deltas (IT-01…IT-12) |
| [performance-m3.md](performance-m3.md) | Shared project index design: bounded unverified path snapshots, separate status, coalesced reconciliation, mutation generations; PI-01…10 automated gate complete |

Startup package (done before Milestone 2): [startup plan](performance-startup.md)
and [briefs](../tasks/performance-startup.md), complete 6/6 plus ST-06A/B (native
GUI/SSH/Windows unperformed).

Detailed code-diagnostics task briefs: [../tasks/README.md](../tasks/README.md).
Authoritative code-diagnostics wire contract: [code-diagnostics-contract.md](code-diagnostics-contract.md).

Reference: [../formats.md](../formats.md) holds the verified on-disk formats of each tool.

**Rules:**
- Contracts in M0.md change there first, then in code.
- If a task turns out to be larger than 4 h, split it in TASKS.md before starting.

Git history platform evidence and remaining native gate: [QA matrix](git-history-qa.md).

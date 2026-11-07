# Implementation plans

Every feature starts here. Write the plan first, `docs/plan/<feature>.md`, then
break it into tasks in a task group, `docs/tasks/<feature>/` (see
[the task index](../tasks/README.md)). Design notes, contracts and evidence that
support one plan sit beside it as `<feature>-<topic>.md`. Overall status is in
[TASKS.md](../../TASKS.md).

## Milestones

Read them in order: later milestones build on the contracts defined in M0.

| Plan | Tasks | Covers |
|---|---|---|
| [M0.md](M0.md) | [M0](../tasks/M0/README.md) | Conventions, **shared contracts** (model, provider interface, JSONL reader, path utils), Claude Code provider, CLI |
| [M1-providers.md](M1-providers.md) | [M1](../tasks/M1/README.md) | OpenCode and Codex providers, scan orchestrator/catalog, grouping engine |
| [M1-gui.md](M1-gui.md) | [M1](../tasks/M1/README.md) | Wails v2 + Svelte 5 app (historical; now Wails v3): bindings, `api.ts`, layout, session list, transcript viewer, security, QA |
| [M2.md](M2.md) | [M2](../tasks/M2/README.md) | Cache index, incremental scans, FTS search, file watching, live updates, diagnostics |
| [M3-M5.md](M3-M5.md) | [M3](../tasks/M3/README.md), [M4](../tasks/M4/README.md), [M5](../tasks/M5/README.md) | Filters, config, export, keyboard, stats; destructive actions; macOS port, packaging, CI |
| [M6.md](M6.md) | [M6](../tasks/M6/README.md) | Portable bundles (export, import, opt-in restore) and cross-agent handoff; bundle manifest v1 contract |
| [M7.md](M7.md) | [M7](../tasks/M7/README.md) | Remote over SSH: headless server via system SSH, browse, search, manage remote sessions |
| [M8.md](M8.md) | [M8](../tasks/M8/README.md) | Windows support: local-first port (paths, Recycle Bin, PowerShell, packaging), WSL as a remote |

## Features

| Plan | Tasks | Covers |
|---|---|---|
| [wails-v3-upgrade.md](wails-v3-upgrade.md) | [wails-v3-upgrade](../tasks/wails-v3-upgrade/README.md) | Move the desktop app to Wails v3 and GTK 4; native manual gates |
| [file-editing.md](file-editing.md) | [file-editing](../tasks/file-editing/README.md) | Go to File editing: safe local/remote saves, drafts, conflicts and lifecycle guards |
| [code-diagnostics.md](code-diagnostics.md) | [code-diagnostics](../tasks/code-diagnostics/README.md) | Go to File diagnostics: project checks, live language servers, trust, Problems panel, local/remote lifecycle |
| [code-diagnostics-languages.md](code-diagnostics-languages.md) | [code-diagnostics-languages](../tasks/code-diagnostics-languages/README.md) | Choose in Settings which languages get code checks and Go to Definition; language registry, filtered discovery |
| [code-diagnostics-ruby.md](code-diagnostics-ruby.md) | [code-diagnostics-ruby](../tasks/code-diagnostics-ruby/README.md) | Ruby code checks with RuboCop: runtime resolution, Bundler, standalone mode |
| [code-diagnostics-ignore-scope.md](code-diagnostics-ignore-scope.md) | [code-diagnostics-ignore-scope](../tasks/code-diagnostics-ignore-scope/README.md) | Project discovery leaves out what Git ignores, still walks ignored nested repositories, and always sees a root marker |
| [code-diagnostics-ruby-lsp.md](code-diagnostics-ruby-lsp.md) | [code-diagnostics-ruby-lsp](../tasks/code-diagnostics-ruby-lsp/README.md) | Ruby Go to definition, references and hover with ruby-lsp, beside RuboCop; add-ons not loaded |
| [tool-installation.md](tool-installation.md) | [tool-installation](../tasks/tool-installation/README.md) | Consent-based managed language-server installation |
| [editor-essentials.md](editor-essentials.md) | [editor-essentials](../tasks/editor-essentials/README.md) | Project search and replace, new agent sessions and alerts, code navigation, Git changes |
| [git-history.md](git-history.md) | [git-history](../tasks/git-history/README.md) | Read-only Git history: multi-pane layout, repository/branch selection, commit search, historical diffs |
| [nested-repo-blame.md](nested-repo-blame.md) | [nested-repo-blame](../tasks/nested-repo-blame/README.md) | Line blame, changed-line markers and status tints for repositories under a non-Git directory |
| [nested-repo-gitignore.md](nested-repo-gitignore.md) | [nested-repo-gitignore](../tasks/nested-repo-gitignore/README.md) | Files, search and blame follow each repository's Git ignore rules under a non-Git directory |
| [performance.md](performance.md) | [performance](../tasks/performance/README.md) | Performance roadmap (milestones 1–5); Milestone 1 tasks: baselines and targeted reuse |
| [performance-startup.md](performance-startup.md) | [performance-startup](../tasks/performance-startup/README.md) | Startup responsiveness, the priority package before performance Milestone 2 |
| [performance-m2.md](performance-m2.md) | [performance-m2](../tasks/performance-m2/README.md) | Performance Milestone 2: incremental transcripts, byte-budgeted cache, FTS reuse, transcript deltas |
| [performance-m3.md](performance-m3.md) | [performance-m3](../tasks/performance-m3/README.md) | Performance Milestone 3: shared project index, bounded path snapshots, coalesced reconciliation |
| [transcript-page-cap.md](transcript-page-cap.md) | [transcript-page-cap](../tasks/transcript-page-cap/README.md) | Transcript pages capped by size as well as count, so large Codex and OpenCode sessions open quickly |
| [dark-tone.md](dark-tone.md) | [dark-tone](../tasks/dark-tone/README.md) | Dark surfaces layered around the native #2C2C2B tone |
| [dialog-close.md](dialog-close.md) | [dialog-close](../tasks/dialog-close/README.md) | Centered shared close button for dialogs |
| [selection-translate.md](selection-translate.md) | [selection-translate](../tasks/selection-translate/README.md) | Copy/Translate menu for a transcript selection, with its sentence as context |
| [terminal-maximize.md](terminal-maximize.md) | [terminal-maximize](../tasks/terminal-maximize/README.md) | Maximize and restore the internal terminal without restarting sessions |
| [terminal-split.md](terminal-split.md) | [terminal-split](../tasks/terminal-split/README.md) | Several shells side by side in one terminal tab |
| [font-settings.md](font-settings.md) | [font-settings](../tasks/font-settings/README.md) | Settings → Appearance: editor font family and size; one type scale (ratios of body text) for project docs and session content, scaled by a body size |
| [editor-history.md](editor-history.md) | [editor-history](../tasks/editor-history/README.md) | Back and forward through files and places in Go to File, per workspace, with the mouse's side buttons |
| [swipe-navigation.md](swipe-navigation.md) | [swipe-navigation](../tasks/swipe-navigation/README.md) | Two-finger trackpad swipe back and forward on macOS, in the session list and Go to File |
| [swipe-navigation-windows-linux.md](swipe-navigation-windows-linux.md) | [swipe-navigation-windows-linux](../tasks/swipe-navigation-windows-linux/README.md) | The same two-finger touchpad swipe on Windows (from wheel events) and Linux (GTK, native) |
| [linux-gpu-rendering.md](linux-gpu-rendering.md) | [linux-gpu-rendering](../tasks/linux-gpu-rendering/README.md) | Linux: keep WebKit's DMA-BUF GPU renderer; only NVIDIA (Wails' check) falls back to shared memory |

## Supporting documents

**[code-diagnostics.md](code-diagnostics.md):**
- [code-diagnostics-checkers.md](code-diagnostics-checkers.md): Checker format and invocation evidence (CD-04)
- [code-diagnostics-contract.md](code-diagnostics-contract.md): Code diagnostics contract v1 (CD-01)
- [code-diagnostics-engine.md](code-diagnostics-engine.md): Engine/App integration (CD-08)
- [code-diagnostics-frontend.md](code-diagnostics-frontend.md): Code diagnostics frontend (CD-10 / CD-11 / CD-12 / CD-13 / CD-14 / CD-15)
- [code-diagnostics-go-build-contexts.md](code-diagnostics-go-build-contexts.md): Automatic Go build contexts
- [code-diagnostics-go-vet.md](code-diagnostics-go-vet.md): Go vet adapter evidence (CD-05)
- [code-diagnostics-gopls.md](code-diagnostics-gopls.md): Code diagnostics: gopls adapter and supported-version decision (CD-22)
- [code-diagnostics-live-qa.md](code-diagnostics-live-qa.md): CD-27 — Live diagnostics QA preparation and blocked release record
- [code-diagnostics-manager.md](code-diagnostics-manager.md): Backend result manager (CD-07)
- [code-diagnostics-ownership-audit.md](code-diagnostics-ownership-audit.md): R01 — live diagnostics ownership and freshness decision
- [code-diagnostics-project-watch.md](code-diagnostics-project-watch.md): Approved project watcher + `go-syntax` saved-file provider (2026-10-23)
- [code-diagnostics-python.md](code-diagnostics-python.md): Code diagnostics: Python adapters (Ruff and Pyright)
- [code-diagnostics-qa.md](code-diagnostics-qa.md): CD-17 — Stage 1 QA preparation and blocked release record
- [code-diagnostics-r04.md](code-diagnostics-r04.md): R04 — production discovery/trust resolver seam
- [code-diagnostics-r05.md](code-diagnostics-r05.md): R05 — internal Engine/session owner route
- [code-diagnostics-r07.md](code-diagnostics-r07.md): R07 — current live source/editor ranges
- [code-diagnostics-r08.md](code-diagnostics-r08.md): R08 — failure, recovery and bounded coverage UI
- [code-diagnostics-r09.md](code-diagnostics-r09.md): R09 — contained saved-disk probe (2026-10-22)
- [code-diagnostics-r10.md](code-diagnostics-r10.md): R10 — probe-driven reconciliation wiring (2026-10-22)
- [code-diagnostics-r14.md](code-diagnostics-r14.md): R14 — full-route fake-attachment tests (2026-10-22)
- [code-diagnostics-r15.md](code-diagnostics-r15.md): R15 — owned cleanup instrumentation and repeated lifecycle tests (2026-10-22)
- [code-diagnostics-rpc.md](code-diagnostics-rpc.md): Code diagnostics RPC (CD-09)
- [code-diagnostics-rust.md](code-diagnostics-rust.md): Rust code checks
- [code-diagnostics-supervisor.md](code-diagnostics-supervisor.md): Checker supervisor (CD-03)
- [code-diagnostics-svelte.md](code-diagnostics-svelte.md): Svelte-check adapter evidence (CD-06)
- [code-diagnostics-sync-recovery.md](code-diagnostics-sync-recovery.md): Code diagnostics — live sync recovery (evidence)
- [code-diagnostics-trust.md](code-diagnostics-trust.md): Code diagnostics trust policy v1 (CD-02)
- [code-diagnostics-typescript.md](code-diagnostics-typescript.md): Code diagnostics: TypeScript adapter and version spike (CD-23)
- [code-diagnostics-workspaces.md](code-diagnostics-workspaces.md): Nested diagnostics workspaces (2026-10-23)
- [code-diagnostics-zed-model.md](code-diagnostics-zed-model.md): Code diagnostics: the editor model (2026-10-23)

**[git-history.md](git-history.md):**
- [git-history-contract.md](git-history-contract.md): Git history contract v1
- [git-history-qa.md](git-history-qa.md): Git history QA — 2026-10-26

Reference: [../formats.md](../formats.md) holds the verified on-disk formats
of each tool; [../perf.md](../perf.md) holds performance measurements.

## Rules

- Contracts in M0.md change there first, then in code.
- A new feature gets a plan here before any task group or code.
- If a task turns out to be larger than 4 h, split it in its task group before starting.

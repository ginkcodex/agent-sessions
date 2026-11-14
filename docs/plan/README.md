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
| [chat-plugin-commands.md](chat-plugin-commands.md) | [chat-plugin-commands](../tasks/chat-plugin-commands/README.md) | Read-only MCP/plugin inventories, installed plugin command invocation and visible user command messages live and after reopen |
| [layout-memory.md](layout-memory.md) | [layout-memory](../tasks/layout-memory/README.md) | Content-fit pane sizing and remembered window and pane geometry across launches |
| [transcript-tool-readability.md](transcript-tool-readability.md) | [transcript-tool-readability](../tasks/transcript-tool-readability/README.md) | Readable tool summaries, decoded text envelopes and expandable output previews at Activity and All levels |
| [message-file-single-match.md](message-file-single-match.md) | [message-file-single-match](../tasks/message-file-single-match/README.md) | A file name or partial path in a message or terminal that matches one project file opens it directly; several matches still offer a toast |
| [transcript-directory-links.md](transcript-directory-links.md) | [transcript-directory-links](../tasks/transcript-directory-links/README.md) | Exclude trailing-separator directory paths from session-content file-opening links |
| [terminal-auto-activation.md](terminal-auto-activation.md) | [terminal-auto-activation](../tasks/terminal-auto-activation/README.md) | Default-enabled preference to stop session selection/focus from switching internal terminal tabs |
| [acp-adapter-install.md](acp-adapter-install.md) | [acp-adapter-install](../tasks/acp-adapter-install/README.md) | One-click, consented install of pinned Claude and Codex ACP adapters through the managed-tools installer, on this computer and on WSL/SSH hosts |
| [acp-chat.md](acp-chat.md) | [acp-chat](../tasks/acp-chat/README.md) | Drive Claude Code, Codex and OpenCode over ACP to stream live messages, tool calls and permissions into the transcript, locally and on WSL/SSH hosts |
| [chat-attachments.md](chat-attachments.md) | [chat-attachments](../tasks/chat-attachments/README.md) | Attach images and text files to a chat message from a + button, a drop or a paste |
| [chat-end-guard.md](chat-end-guard.md) | [chat-end-guard](../tasks/chat-end-guard/README.md) | A one-line notice when the user ends a chat; ask before ending a working chat, and before closing or quitting with open chats |
| [end-chat-menu.md](end-chat-menu.md) | [end-chat-menu](../tasks/end-chat-menu/README.md) | The composer's End chat button as a menu: End the chat as today, or Switch to terminal to stop the chat and resume the session in a terminal tab |
| [switch-to-chat.md](switch-to-chat.md) | [switch-to-chat](../tasks/switch-to-chat/README.md) | Switch to chat on a terminal tab's context menu: end the tab's agent and continue the session in a live chat |
| [close-dialog-groups.md](close-dialog-groups.md) | [close-dialog-groups](../tasks/close-dialog-groups/README.md) | Closing dialogs list what they end as Terminals and Chats cards with icons and the unsaved-changes question as an Unsaved files card, in a polished prompt |
| [host-switch-chat-guard.md](host-switch-chat-guard.md) | [host-switch-chat-guard](../tasks/host-switch-chat-guard/README.md) | Ask before connecting, switching hosts or disconnecting ends the window's open chats, as it asks about terminal tabs |
| [codex-daemon-chat.md](codex-daemon-chat.md) | [codex-daemon-chat](../tasks/codex-daemon-chat/README.md) | Open a Codex session in a chat right after the Codex CLI quits, through the Codex daemon; a clear "in use" error with Try again |
| [chat-adapter-setup.md](chat-adapter-setup.md) | [chat-adapter-setup](../tasks/chat-adapter-setup/README.md) | A short Chat settings section: the app's own pinned adapters, one-click Install, and Update when the copy is older than the pin |
| [chat-questions.md](chat-questions.md) | [chat-questions](../tasks/chat-questions/README.md) | Show an agent's questions in a chat with compact tabs, clear choices and a custom answer, changeable until Submit |
| [orphan-task-notifications.md](orphan-task-notifications.md) | [orphan-task-notifications](../tasks/orphan-task-notifications/README.md) | Show the notification Claude Code writes for background tasks orphaned by a previous run as a notification, not raw XML |
| [idle-rendering.md](idle-rendering.md) | [idle-rendering](../tasks/idle-rendering/README.md) | Reduce idle animations and unnecessary compositing; native GPU validation gates |
| [file-translate.md](file-translate.md) | [file-translate](../tasks/file-translate/README.md) | Translate complete Markdown files from the preview context menu without selecting text |
| [print-page-setup.md](print-page-setup.md) | [print-page-setup](../tasks/print-page-setup/README.md) | Shared paper, orientation and margin controls with native print defaults |
| [windows-ci-test-stability.md](windows-ci-test-stability.md) | [windows-ci-test-stability](../tasks/windows-ci-test-stability/README.md) | Deterministic Git budgets and watcher startup synchronization in Windows CI tests |
| [workspace-terminal-selection.md](workspace-terminal-selection.md) | [workspace-terminal-selection](../tasks/workspace-terminal-selection/README.md) | Directory clicks in the workspace tree select the opened session's terminal tab |
| [conversation-pdf.md](conversation-pdf.md) | [conversation-pdf](../tasks/conversation-pdf/README.md) | Print a session's entire Chat-level conversation from the transcript header menu |
| [terminal-file-paths.md](terminal-file-paths.md) | [terminal-file-paths](../tasks/terminal-file-paths/README.md) | Correct inner-path detection in terminal tool-call wrappers |
| [workspace-stars.md](workspace-stars.md) | [workspace-stars](../tasks/workspace-stars/README.md) | Persistent workspace stars and a starred-only sidebar filter |
| [macos-terminal-input.md](macos-terminal-input.md) | [macos-terminal-input](../tasks/macos-terminal-input/README.md) | Vietnamese replacement input in macOS terminals without text loss or cursor movement |
| [manual-bash-transcript.md](manual-bash-transcript.md) | [manual-bash-transcript](../tasks/manual-bash-transcript/README.md) | Readable user-run Bash command/output cards and clean session titles |
| [code-diagnostics-java-php.md](code-diagnostics-java-php.md) | [code-diagnostics-java-php](../tasks/code-diagnostics-java-php/README.md) | Java/PHP source navigation and live diagnostics with JDT LS and Intelephense |
| [session-scrollbar.md](session-scrollbar.md) | [session-scrollbar](../tasks/session-scrollbar/README.md) | Linux session-list scrollbar stability and adjacent transcript header flicker during rapid scrolling |
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
| [session-mono-font.md](session-mono-font.md) | [session-mono-font](../tasks/session-mono-font/README.md) | Settings → Appearance checkbox: code in session content uses the editor font |
| [editor-history.md](editor-history.md) | [editor-history](../tasks/editor-history/README.md) | Back and forward through files and places in Go to File, per workspace, with the mouse's side buttons |
| [swipe-navigation.md](swipe-navigation.md) | [swipe-navigation](../tasks/swipe-navigation/README.md) | Two-finger trackpad swipe back and forward on macOS, in the session list and Go to File |
| [swipe-navigation-windows-linux.md](swipe-navigation-windows-linux.md) | [swipe-navigation-windows-linux](../tasks/swipe-navigation-windows-linux/README.md) | The same two-finger touchpad swipe on Windows (from wheel events) and Linux (GTK, native) |
| [linux-gpu-rendering.md](linux-gpu-rendering.md) | [linux-gpu-rendering](../tasks/linux-gpu-rendering/README.md) | Linux: keep WebKit's DMA-BUF GPU renderer; only NVIDIA (Wails' check) falls back to shared memory |
| [aur-package.md](aur-package.md) | [aur-package](../tasks/aur-package/README.md) | Arch Linux: `agent-sessions-bin` on the AUR from each stable release's `.deb`, published by CI |
| [remote-reconnect.md](remote-reconnect.md) | [remote-reconnect](../tasks/remote-reconnect/README.md) | Faster recovery from interrupted SSH and WSL connections: liveness watchdog, fresh redial, bounded retries, same-host reload |
| [terminal-cwd-matching.md](terminal-cwd-matching.md) | [terminal-cwd-matching](../tasks/terminal-cwd-matching/README.md) | Match shell workspaces by detected cwd while preserving agent identity and active equal matches |
| [codex-guardian-ancestry.md](codex-guardian-ancestry.md) | [codex-guardian-ancestry](../tasks/codex-guardian-ancestry/README.md) | Link Guardian reviews to their main session for browsing, counts and cleanup |
| [chat-live-filter.md](chat-live-filter.md) | [chat-live-filter](../tasks/chat-live-filter/README.md) | Include sessions running in chat in the Live filter |
| [codex-restore-filenames.md](codex-restore-filenames.md) | [codex-restore-filenames](../tasks/codex-restore-filenames/README.md) | Restore resumable Codex rollout filenames and repair previous imports |
| [chat-options-dropdown.md](chat-options-dropdown.md) | [chat-options-dropdown](../tasks/chat-options-dropdown/README.md) | The chat's mode, model and effort pickers as the shared themed Dropdown, opening upward at the composer |
| [workspace-filter-release.md](workspace-filter-release.md) | [workspace-filter-release](../tasks/workspace-filter-release/README.md) | A click on the group tree's empty space releases the workspace filter; the open session's group gets a quieter highlight |
| [chat-login-env.md](chat-login-env.md) | [chat-login-env](../tasks/chat-login-env/README.md) | A live chat's agent runs in the user's interactive login shell environment, so version-manager tools (mise, nvm) on PATH are found as in terminal tabs |
| [code-block-run.md](code-block-run.md) | [code-block-run](../tasks/code-block-run/README.md) | A Run button on an agent's shell code blocks opens a terminal tab in the session's directory and runs the command there |
| [linux-repos.md](linux-repos.md) | [linux-repos](../tasks/linux-repos/README.md) | Signed apt and dnf repositories on the website Worker (releases recorded in Postgres) that list every stable release, with package downloads redirected to GitHub |
| [auto-update.md](auto-update.md) | [auto-update](../tasks/auto-update/README.md) | macOS and Windows read GitHub's latest release, download and check it in the background, and install it on restart or quit |
| [chat-message-queue.md](chat-message-queue.md) | [chat-message-queue](../tasks/chat-message-queue/README.md) | While a chat's agent works, Enter queues the message for the next turn; Send now puts a queued message into the running turn (ACP steering) |
| [tool-file-links.md](tool-file-links.md) | [tool-file-links](../tasks/tool-file-links/README.md) | The path in a file tool's card header (Read, Edit, Write) opens the file in the file viewer |
| [review-fixes.md](review-fixes.md) | [review-fixes](../tasks/review-fixes/README.md) | Fixes from the 2026-11-14 architecture and feature review: release signing and supply chain, untrusted content reaching programs, redaction, watcher, RPC, chat and remote reliability, CI lint |

## Supporting documents

**[macos-terminal-input.md](macos-terminal-input.md):**
- [macos-terminal-input-qa.md](macos-terminal-input-qa.md): Native input probes, desktop reproduction attempts and remaining gaps

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

**[code-diagnostics-java-php.md](code-diagnostics-java-php.md):**
- [code-diagnostics-java-php-qa.md](code-diagnostics-java-php-qa.md): Automated results and blocked real-server/manual release gates

**[git-history.md](git-history.md):**
- [git-history-contract.md](git-history-contract.md): Git history contract v1
- [git-history-qa.md](git-history-qa.md): Git history QA — 2026-10-26

**[remote-reconnect.md](remote-reconnect.md):**
- [remote-reconnect-qa.md](remote-reconnect-qa.md): Remote reconnect measurements and native checks

**[codex-daemon-chat.md](codex-daemon-chat.md):**
- [codex-daemon-chat-spike.md](codex-daemon-chat-spike.md): Why the daemon refuses the chat, and a bridge to it (2026-11-12)

Reference: [../formats.md](../formats.md) holds the verified on-disk formats
of each tool; [../perf.md](../perf.md) holds performance measurements.

## Rules

- Contracts in M0.md change there first, then in code.
- A new feature gets a plan here before any task group or code.
- If a task turns out to be larger than 4 h, split it in its task group before starting.

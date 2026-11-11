# Agent Sessions — Task Breakdown

Derived from [SPEC.md](SPEC.md). This file is the top-level index. Every
feature has a plan in [`docs/plan/`](docs/plan/README.md) and a task group in
[`docs/tasks/<feature>/`](docs/tasks/README.md): a README with the progress
table and one file per task with its details, status and evidence. Every task
is scoped to be **≤ 4 h for a senior Go/TS developer**; estimates include
writing tests for the task.

**Current:** Wails **v3.1.0**, using GTK 4 + WebKitGTK 6.0 on Linux
(Ubuntu 24.04+, Debian 13+, Fedora 40+; Ubuntu 22.04 and Debian 12 dropped).
See SPEC §2 and [the Wails v3 upgrade](docs/plan/wails-v3-upgrade.md). The
project started on Wails v2 with GTK3 + `webkit2gtk-4.1`; the M1 task files
keep that original wording as history.

**Decided since the first draft:**
- The frontend is Svelte 5 + TS.
- Opt-in destructive actions (M4) shipped; they stay disabled by default.
- Codex fixtures are still blocked on getting a real sample (M1-08).

**Definition of Done (applies to every task):**
- Code is linted (`golangci-lint`, `svelte-check`).
- Unit or golden tests are added, and `make test` passes.
- No writes to any tool's session store, except the M4 actions and the M6
  restore after an explicit, previewed confirmation.
- No credential files are read.
- Public types and functions have doc comments where the intent is not obvious.
- The task file, its group README and the tables below show the new status.

**Status:** ✅ done · 🟡 partial (the task file says what is missing) ·
⛔ blocked · ⬜ not started · ➖ superseded. Tasks marked *(retroactive)*
record work that shipped without a planned task.

## Milestones

| Milestone | Plan | Tasks | Est. hours | ✅ | 🟡 | ⬜ / ⛔ |
|---|---|---|---|---|---|---|
| [M0 — Foundation & Claude Code provider](docs/tasks/M0/README.md) | [M0](docs/plan/M0.md) | 16 | 42 | 15 | 1 | 0 |
| [M1 — Remaining providers + MVP GUI](docs/tasks/M1/README.md) | [M1-providers](docs/plan/M1-providers.md), [M1-gui](docs/plan/M1-gui.md) | 31 | 90 | 23 | 6 | 2 |
| [M2 — Index, search, live updates](docs/tasks/M2/README.md) | [M2](docs/plan/M2.md) | 12 | 36 | 8 | 2 | 2 |
| [M3 — Polish & power features](docs/tasks/M3/README.md) | [M3-M5](docs/plan/M3-M5.md) | 14 | 37 | 2 | 6 | 6 |
| [M4 — Destructive actions (opt-in)](docs/tasks/M4/README.md) | [M3-M5](docs/plan/M3-M5.md) | 6 | 18 | 5 | 0 | 1 |
| [M5 — macOS & distribution](docs/tasks/M5/README.md) | [M3-M5](docs/plan/M3-M5.md) | 6 | 20 | 0 | 5 | 1 |
| [M6 — Portable sessions and cross-agent handoff](docs/tasks/M6/README.md) | [M6](docs/plan/M6.md) | 17 | 60 | 16 | 0 | 1 |
| [M7 — Remote over SSH](docs/tasks/M7/README.md) | [M7](docs/plan/M7.md) | 21 | 65 | 20 | 1 | 0 |
| [M8 — Windows support](docs/tasks/M8/README.md) | [M8](docs/plan/M8.md) | 13 | 62 | 2 | 9 | 2 |
| **Total** | | **136** | **≈ 430 h** | **91** | **30** | **15** |

**Critical path to the MVP:**

M0-01 → M0-02 → M0-03 → M0-07 → M0-08 → M0-09 → M0-10 → M1-13 → M1-16 → M1-20 → M1-24 → M1-28

**Parallelizable after M0-03:**
- OpenCode (M1-01…07), Codex (M1-08…12), Claude (M0-07…15) and the GUI scaffold (M1-15, M1-17) can each go to a different developer.
- Frontend work can start against a mocked `M1-16` binding layer.

## Features

| Feature | Plan | Tasks | Progress |
|---|---|---|---|
| ACP chat mode | [acp-chat](docs/plan/acp-chat.md) | [acp-chat/](docs/tasks/acp-chat/README.md) | 1/14 ✅ · 13 ⬜ |
| Idle rendering | [idle-rendering](docs/plan/idle-rendering.md) | [idle-rendering/](docs/tasks/idle-rendering/README.md) | 3/7 ✅ · 4 🟡 |
| Whole-file translation | [file-translate](docs/plan/file-translate.md) | [file-translate/](docs/tasks/file-translate/README.md) | 2/2 ✅ |
| Print page setup | [print-page-setup](docs/plan/print-page-setup.md) | [print-page-setup/](docs/tasks/print-page-setup/README.md) | 1/1 ✅ |
| Windows CI test stability | [windows-ci-test-stability](docs/plan/windows-ci-test-stability.md) | [windows-ci-test-stability/](docs/tasks/windows-ci-test-stability/README.md) | 1/1 ✅ |
| Workspace terminal selection | [workspace-terminal-selection](docs/plan/workspace-terminal-selection.md) | [workspace-terminal-selection/](docs/tasks/workspace-terminal-selection/README.md) | 1/1 ✅ |
| Conversation PDF | [conversation-pdf](docs/plan/conversation-pdf.md) | [conversation-pdf/](docs/tasks/conversation-pdf/README.md) | 1/1 ✅ |
| Terminal file path detection | [terminal-file-paths](docs/plan/terminal-file-paths.md) | [terminal-file-paths/](docs/tasks/terminal-file-paths/README.md) | 1/1 ✅ |
| Workspace stars | [workspace-stars](docs/plan/workspace-stars.md) | [workspace-stars/](docs/tasks/workspace-stars/README.md) | 3/3 ✅ |
| macOS terminal Vietnamese input | [macos-terminal-input](docs/plan/macos-terminal-input.md) | [macos-terminal-input/](docs/tasks/macos-terminal-input/README.md) | 1/3 ✅ · 1 🟡 · 1 ➖ |
| Manual Bash transcript | [manual-bash-transcript](docs/plan/manual-bash-transcript.md) | [manual-bash-transcript/](docs/tasks/manual-bash-transcript/README.md) | 2/2 ✅ |
| Java/PHP code intelligence | [code-diagnostics-java-php](docs/plan/code-diagnostics-java-php.md) | [code-diagnostics-java-php/](docs/tasks/code-diagnostics-java-php/README.md) | 5/5 ✅ |
| Session scrollbar stability | [session-scrollbar](docs/plan/session-scrollbar.md) | [session-scrollbar/](docs/tasks/session-scrollbar/README.md) | 9/10 ✅ · 1 🟡 |
| Terminal cwd matching | [terminal-cwd-matching](docs/plan/terminal-cwd-matching.md) | [terminal-cwd-matching/](docs/tasks/terminal-cwd-matching/README.md) | 1/1 ✅ |
| Code diagnostics (Go to File checks) | [code-diagnostics](docs/plan/code-diagnostics.md) | [code-diagnostics/](docs/tasks/code-diagnostics/README.md) | 19/28 ✅ · 8 🟡 · 1 ⛔ |
| Editor essentials | [editor-essentials](docs/plan/editor-essentials.md) | [editor-essentials/](docs/tasks/editor-essentials/README.md) | 25/25 ✅ |
| Git history | [git-history](docs/plan/git-history.md) | [git-history/](docs/tasks/git-history/README.md) | 17/18 ✅ · 1 🟡 |
| Code checks: choosing languages | [code-diagnostics-languages](docs/plan/code-diagnostics-languages.md) | [code-diagnostics-languages/](docs/tasks/code-diagnostics-languages/README.md) | 8/8 ✅ |
| Code checks: Git ignore rules in discovery | [code-diagnostics-ignore-scope](docs/plan/code-diagnostics-ignore-scope.md) | [code-diagnostics-ignore-scope/](docs/tasks/code-diagnostics-ignore-scope/README.md) | 4/6 ✅ · 1 🟡 · 1 ⬜ |
| Code checks: Ruby (RuboCop) | [code-diagnostics-ruby](docs/plan/code-diagnostics-ruby.md) | [code-diagnostics-ruby/](docs/tasks/code-diagnostics-ruby/README.md) | 7/7 ✅ |
| Ruby Go to definition (ruby-lsp) | [code-diagnostics-ruby-lsp](docs/plan/code-diagnostics-ruby-lsp.md) | [code-diagnostics-ruby-lsp/](docs/tasks/code-diagnostics-ruby-lsp/README.md) | 12/14 ✅ · 1 🟡 · 1 ⬜ |
| Performance M1: baselines and targeted reuse | [performance](docs/plan/performance.md) | [performance/](docs/tasks/performance/README.md) | 13/15 ✅ · 1 🟡 · 1 ➖ |
| Performance M2: incremental transcripts | [performance-m2](docs/plan/performance-m2.md) | [performance-m2/](docs/tasks/performance-m2/README.md) | 12/12 ✅ |
| Performance M3 | [performance-m3](docs/plan/performance-m3.md) | [performance-m3/](docs/tasks/performance-m3/README.md) | 10/10 ✅ |
| Startup responsiveness | [performance-startup](docs/plan/performance-startup.md) | [performance-startup/](docs/tasks/performance-startup/README.md) | 10/10 ✅ |
| Transcript page cap | [transcript-page-cap](docs/plan/transcript-page-cap.md) | [transcript-page-cap/](docs/tasks/transcript-page-cap/README.md) | 5/6 ✅ · 1 ⬜ |
| Consent-based managed tools | [tool-installation](docs/plan/tool-installation.md) | [tool-installation/](docs/tasks/tool-installation/README.md) | 5/6 ✅ · 1 ⬜ |
| Native-tone dark surfaces | [dark-tone](docs/plan/dark-tone.md) | [dark-tone/](docs/tasks/dark-tone/README.md) | 5/5 ✅ |
| Centered dialog close buttons | [dialog-close](docs/plan/dialog-close.md) | [dialog-close/](docs/tasks/dialog-close/README.md) | 2/2 ✅ |
| Blame in nested repositories | [nested-repo-blame](docs/plan/nested-repo-blame.md) | [nested-repo-blame/](docs/tasks/nested-repo-blame/README.md) | 4/4 ✅ |
| Git ignore rules in nested repositories | [nested-repo-gitignore](docs/plan/nested-repo-gitignore.md) | [nested-repo-gitignore/](docs/tasks/nested-repo-gitignore/README.md) | 4/5 ✅ · 1 ⬜ |
| Selected-text menu and contextual translation | [selection-translate](docs/plan/selection-translate.md) | [selection-translate/](docs/tasks/selection-translate/README.md) | 5/5 ✅ |
| Internal terminal maximization | [terminal-maximize](docs/plan/terminal-maximize.md) | [terminal-maximize/](docs/tasks/terminal-maximize/README.md) | 2/2 ✅ |
| Split terminals | [terminal-split](docs/plan/terminal-split.md) | [terminal-split/](docs/tasks/terminal-split/README.md) | 4/4 ✅ |
| Font settings: editor, docs and session content | [font-settings](docs/plan/font-settings.md) | [font-settings/](docs/tasks/font-settings/README.md) | 10/11 ✅ · 1 🟡 |
| Editor back and forward | [editor-history](docs/plan/editor-history.md) | [editor-history/](docs/tasks/editor-history/README.md) | 3/4 ✅ · 1 🟡 |
| Two-finger swipe back and forward | [swipe-navigation](docs/plan/swipe-navigation.md) | [swipe-navigation/](docs/tasks/swipe-navigation/README.md) | 1/3 ✅ · 1 🟡 · 1 ⬜ |
| Two-finger swipe on Windows and Linux | [swipe-navigation-windows-linux](docs/plan/swipe-navigation-windows-linux.md) | [swipe-navigation-windows-linux/](docs/tasks/swipe-navigation-windows-linux/README.md) | 7/10 ✅ · 2 🟡 · 1 ➖ |
| Linux GPU rendering | [linux-gpu-rendering](docs/plan/linux-gpu-rendering.md) | [linux-gpu-rendering/](docs/tasks/linux-gpu-rendering/README.md) | 1/2 ✅ · 1 🟡 |
| Go to File editing | [file-editing](docs/plan/file-editing.md) | [file-editing/](docs/tasks/file-editing/README.md) | 11/14 ✅ · 3 🟡 |
| Wails v3 upgrade | [wails-v3-upgrade](docs/plan/wails-v3-upgrade.md) | [wails-v3-upgrade/](docs/tasks/wails-v3-upgrade/README.md) | 1/2 ✅ · 1 ⬜ |
| Arch Linux AUR package | [aur-package](docs/plan/aur-package.md) | [aur-package/](docs/tasks/aur-package/README.md) | 4/7 ✅ · 3 🟡 |
| Remote reconnect | [remote-reconnect](docs/plan/remote-reconnect.md) | [remote-reconnect/](docs/tasks/remote-reconnect/README.md) | 8/8 ✅ |

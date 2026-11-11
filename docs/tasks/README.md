# Task groups

Each feature has a plan in [`docs/plan/<feature>.md`](../plan/README.md) and a
task group here, `docs/tasks/<feature>/`, named after the plan. A group holds:

- `README.md`: a link to the plan, a `**Progress:** N/M done` line, and a
  table of every task with its status, scope, estimate and dependencies.
- `<ID>.md`, one per task: a heading `# <ID> — <title>`, then a status block
  (Status, Estimate, Depends on, Plan) and the details: scope, files,
  acceptance criteria, and the evidence recorded while implementing it.

Tasks are at most 4 h each. Status: ✅ Done · 🟡 Partial · ⛔ Blocked ·
⬜ Not started · ➖ Superseded. Update the task file, the group README and
[TASKS.md](../../TASKS.md) whenever the work moves.

## Milestones

| Milestone | Plan | Tasks | Est. hours | ✅ | 🟡 | ⬜ / ⛔ |
|---|---|---|---|---|---|---|
| [M0 — Foundation & Claude Code provider](M0/README.md) | [M0](../plan/M0.md) | 16 | 42 | 15 | 1 | 0 |
| [M1 — Remaining providers + MVP GUI](M1/README.md) | [M1-providers](../plan/M1-providers.md), [M1-gui](../plan/M1-gui.md) | 31 | 90 | 23 | 6 | 2 |
| [M2 — Index, search, live updates](M2/README.md) | [M2](../plan/M2.md) | 12 | 36 | 8 | 2 | 2 |
| [M3 — Polish & power features](M3/README.md) | [M3-M5](../plan/M3-M5.md) | 14 | 37 | 2 | 6 | 6 |
| [M4 — Destructive actions (opt-in)](M4/README.md) | [M3-M5](../plan/M3-M5.md) | 6 | 18 | 5 | 0 | 1 |
| [M5 — macOS & distribution](M5/README.md) | [M3-M5](../plan/M3-M5.md) | 6 | 20 | 0 | 5 | 1 |
| [M6 — Portable sessions and cross-agent handoff](M6/README.md) | [M6](../plan/M6.md) | 17 | 60 | 16 | 0 | 1 |
| [M7 — Remote over SSH](M7/README.md) | [M7](../plan/M7.md) | 21 | 65 | 20 | 1 | 0 |
| [M8 — Windows support](M8/README.md) | [M8](../plan/M8.md) | 13 | 62 | 2 | 9 | 2 |
| **Total** | | **136** | **≈ 430 h** | **91** | **30** | **15** |

## Features

| Feature | Plan | Tasks | Progress |
|---|---|---|---|
| ACP chat mode | [acp-chat](../plan/acp-chat.md) | [acp-chat/](acp-chat/README.md) | 13/15 ✅ · 2 ⬜ |
| Idle rendering | [idle-rendering](../plan/idle-rendering.md) | [idle-rendering/](idle-rendering/README.md) | 3/7 ✅ · 4 🟡 |
| Whole-file translation | [file-translate](../plan/file-translate.md) | [file-translate/](file-translate/README.md) | 2/2 ✅ |
| Print page setup | [print-page-setup](../plan/print-page-setup.md) | [print-page-setup/](print-page-setup/README.md) | 1/1 ✅ |
| Windows CI test stability | [windows-ci-test-stability](../plan/windows-ci-test-stability.md) | [windows-ci-test-stability/](windows-ci-test-stability/README.md) | 1/1 ✅ |
| Workspace terminal selection | [workspace-terminal-selection](../plan/workspace-terminal-selection.md) | [workspace-terminal-selection/](workspace-terminal-selection/README.md) | 1/1 ✅ |
| Conversation PDF | [conversation-pdf](../plan/conversation-pdf.md) | [conversation-pdf/](conversation-pdf/README.md) | 1/1 ✅ |
| Terminal file path detection | [terminal-file-paths](../plan/terminal-file-paths.md) | [terminal-file-paths/](terminal-file-paths/README.md) | 1/1 ✅ |
| Workspace stars | [workspace-stars](../plan/workspace-stars.md) | [workspace-stars/](workspace-stars/README.md) | 3/3 ✅ |
| macOS terminal Vietnamese input | [macos-terminal-input](../plan/macos-terminal-input.md) | [macos-terminal-input/](macos-terminal-input/README.md) | 1/3 ✅ · 1 🟡 · 1 ➖ |
| Manual Bash transcript | [manual-bash-transcript](../plan/manual-bash-transcript.md) | [manual-bash-transcript/](manual-bash-transcript/README.md) | 2/2 ✅ |
| Java/PHP code intelligence | [code-diagnostics-java-php](../plan/code-diagnostics-java-php.md) | [code-diagnostics-java-php/](code-diagnostics-java-php/README.md) | 5/5 ✅ |
| Session scrollbar stability | [session-scrollbar](../plan/session-scrollbar.md) | [session-scrollbar/](session-scrollbar/README.md) | 9/10 ✅ · 1 🟡 |
| Terminal cwd matching | [terminal-cwd-matching](../plan/terminal-cwd-matching.md) | [terminal-cwd-matching/](terminal-cwd-matching/README.md) | 1/1 ✅ |
| Code diagnostics (Go to File checks) | [code-diagnostics](../plan/code-diagnostics.md) | [code-diagnostics/](code-diagnostics/README.md) | 19/28 ✅ · 8 🟡 · 1 ⛔ |
| Editor essentials | [editor-essentials](../plan/editor-essentials.md) | [editor-essentials/](editor-essentials/README.md) | 25/25 ✅ |
| Git history | [git-history](../plan/git-history.md) | [git-history/](git-history/README.md) | 17/18 ✅ · 1 🟡 |
| Code checks: choosing languages | [code-diagnostics-languages](../plan/code-diagnostics-languages.md) | [code-diagnostics-languages/](code-diagnostics-languages/README.md) | 8/8 ✅ |
| Code checks: Git ignore rules in discovery | [code-diagnostics-ignore-scope](../plan/code-diagnostics-ignore-scope.md) | [code-diagnostics-ignore-scope/](code-diagnostics-ignore-scope/README.md) | 4/6 ✅ · 1 🟡 · 1 ⬜ |
| Code checks: Ruby (RuboCop) | [code-diagnostics-ruby](../plan/code-diagnostics-ruby.md) | [code-diagnostics-ruby/](code-diagnostics-ruby/README.md) | 7/7 ✅ |
| Ruby Go to definition (ruby-lsp) | [code-diagnostics-ruby-lsp](../plan/code-diagnostics-ruby-lsp.md) | [code-diagnostics-ruby-lsp/](code-diagnostics-ruby-lsp/README.md) | 12/14 ✅ · 1 🟡 · 1 ⬜ |
| Performance M1: baselines and targeted reuse | [performance](../plan/performance.md) | [performance/](performance/README.md) | 13/15 ✅ · 1 🟡 · 1 ➖ |
| Performance M2: incremental transcripts | [performance-m2](../plan/performance-m2.md) | [performance-m2/](performance-m2/README.md) | 12/12 ✅ |
| Performance M3 | [performance-m3](../plan/performance-m3.md) | [performance-m3/](performance-m3/README.md) | 10/10 ✅ |
| Startup responsiveness | [performance-startup](../plan/performance-startup.md) | [performance-startup/](performance-startup/README.md) | 10/10 ✅ |
| Transcript page cap | [transcript-page-cap](../plan/transcript-page-cap.md) | [transcript-page-cap/](transcript-page-cap/README.md) | 5/6 ✅ · 1 ⬜ |
| Consent-based managed tools | [tool-installation](../plan/tool-installation.md) | [tool-installation/](tool-installation/README.md) | 5/6 ✅ · 1 ⬜ |
| Native-tone dark surfaces | [dark-tone](../plan/dark-tone.md) | [dark-tone/](dark-tone/README.md) | 5/5 ✅ |
| Centered dialog close buttons | [dialog-close](../plan/dialog-close.md) | [dialog-close/](dialog-close/README.md) | 2/2 ✅ |
| Blame in nested repositories | [nested-repo-blame](../plan/nested-repo-blame.md) | [nested-repo-blame/](nested-repo-blame/README.md) | 4/4 ✅ |
| Git ignore rules in nested repositories | [nested-repo-gitignore](../plan/nested-repo-gitignore.md) | [nested-repo-gitignore/](nested-repo-gitignore/README.md) | 4/5 ✅ · 1 ⬜ |
| Selected-text menu and contextual translation | [selection-translate](../plan/selection-translate.md) | [selection-translate/](selection-translate/README.md) | 5/5 ✅ |
| Internal terminal maximization | [terminal-maximize](../plan/terminal-maximize.md) | [terminal-maximize/](terminal-maximize/README.md) | 2/2 ✅ |
| Split terminals | [terminal-split](../plan/terminal-split.md) | [terminal-split/](terminal-split/README.md) | 4/4 ✅ |
| Font settings: editor, docs and session content | [font-settings](../plan/font-settings.md) | [font-settings/](font-settings/README.md) | 10/11 ✅ · 1 🟡 |
| Editor back and forward | [editor-history](../plan/editor-history.md) | [editor-history/](editor-history/README.md) | 3/4 ✅ · 1 🟡 |
| Two-finger swipe back and forward | [swipe-navigation](../plan/swipe-navigation.md) | [swipe-navigation/](swipe-navigation/README.md) | 1/3 ✅ · 1 🟡 · 1 ⬜ |
| Two-finger swipe on Windows and Linux | [swipe-navigation-windows-linux](../plan/swipe-navigation-windows-linux.md) | [swipe-navigation-windows-linux/](swipe-navigation-windows-linux/README.md) | 7/10 ✅ · 2 🟡 · 1 ➖ |
| Linux GPU rendering | [linux-gpu-rendering](../plan/linux-gpu-rendering.md) | [linux-gpu-rendering/](linux-gpu-rendering/README.md) | 1/2 ✅ · 1 🟡 |
| Go to File editing | [file-editing](../plan/file-editing.md) | [file-editing/](file-editing/README.md) | 11/14 ✅ · 3 🟡 |
| Wails v3 upgrade | [wails-v3-upgrade](../plan/wails-v3-upgrade.md) | [wails-v3-upgrade/](wails-v3-upgrade/README.md) | 1/2 ✅ · 1 ⬜ |
| Arch Linux AUR package | [aur-package](../plan/aur-package.md) | [aur-package/](aur-package/README.md) | 4/7 ✅ · 3 🟡 |
| Remote reconnect | [remote-reconnect](../plan/remote-reconnect.md) | [remote-reconnect/](remote-reconnect/README.md) | 8/8 ✅ |

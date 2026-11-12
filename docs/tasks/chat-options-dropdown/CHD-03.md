# CHD-03 — QA in the desktop app

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** CHD-02
- **Plan:** [chat-options-dropdown.md](../../plan/chat-options-dropdown.md)

## Scope

Manual pass in the desktop app (Linux, WebKitGTK): open a chat with
Claude Code, Codex and OpenCode and check each session-option picker —
the menu opens upward inside the window, group headings, hover
descriptions, `Not set` before
the agent reports a value, the tooltip `name: description`, the
unguarded warning (the mode's name stays bare), busy disabling, and
that a refused change re-shows the agent's value. Then the font picker
in Settings → Appearance and one other
dropdown (a branch or diff-basis picker in Go to File) still open
downward and look unchanged.

## Acceptance criteria and tests

Record what was run and what was not in Evidence. Planned QA is not a
pass.

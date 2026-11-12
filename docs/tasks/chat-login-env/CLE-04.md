# CLE-04 — QA in the desktop app

- **Status:** ⬜ Not started
- **Estimate:** 0.5 h
- **Depends on:** CLE-03
- **Plan:** [chat-login-env.md](../../plan/chat-login-env.md)

## Scope

On a host where mise is activated only in `~/.bashrc` (below the
non-interactive guard):

- Start a chat on this computer from the desktop launcher and ask the agent
  to run `node --version` (or another mise-managed tool) and `echo $PATH`.
- Do the same in a chat on a WSL host from the Windows app.
- Open a terminal tab on this computer and check that it still finds the
  same tools.

## Acceptance criteria

- The chats find the mise-managed tools that the terminal tabs find.

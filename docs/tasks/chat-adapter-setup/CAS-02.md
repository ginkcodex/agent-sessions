# CAS-02 — The app's copy runs first

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** CAS-01
- **Plan:** [chat-adapter-setup.md](../../plan/chat-adapter-setup.md)

## Scope

- `findAdapter` in `internal/engine/chat_launch.go`: the override, then
  the app's copy; the PATH lookup only where the app cannot install (no
  managed installer, or no catalog entry).
- A missing adapter's problem reads "Not installed" where the app installs.
- `ChatLaunch`'s error points to Settings › Coding agents instead of an
  npm command.

## Acceptance and tests

- Engine tests with fake installs and a fake adapter on PATH: the app's copy
  runs when both exist; a PATH copy alone is not used where the app installs;
  it is used where the app cannot; the override still wins.
- The start error names Settings and no npm command.

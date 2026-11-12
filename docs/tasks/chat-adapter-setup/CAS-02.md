# CAS-02 — The app's copy runs first

- **Status:** ✅ Done
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

## Evidence

What was done:

- `findAdapter`: the override, then, where the app installs the adapter,
  only `managedAdapter` (the app's copy); the PATH lookup only where it
  cannot. A missing copy reads "<adapter> is not installed".
- `ChatLaunch`'s error ends "Set it up in Settings › Coding agents".
- `ACPAdapter.Install` and `ChatAgent.Install` are gone from Go; the
  frontend's `install` is optional (older servers still send it) until CAS-04
  removes the copy command.

Verified:

- `TestChatAgentsManagedAdapter`: the app's copy wins over one on PATH;
  without it PATH is not even searched; with no managed installs PATH is
  used; an override still wins. `TestChatLaunchManagedAdapter`: a missing
  copy is refused with the Settings hint and no npm, even with the adapter on
  PATH. `TestChatLaunchRefusals` and
  `TestEngineChatAgentsOffersManagedInstall` follow the new messages.
- `go test -race` for `internal/engine`, `internal/provider/...`,
  `internal/rpc` and `internal/app`: ok. gofumpt clean; golangci-lint shows
  no finding in the changed files.
- `npm run check`: 0 errors. The adapter-install, chat-ui, agent-settings
  and chats-store test files pass.

Not checked: a real adapter on a real host (CAS-05).

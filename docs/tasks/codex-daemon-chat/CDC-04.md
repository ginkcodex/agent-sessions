# CDC-04 — Launch Codex chats through the bridge

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CDC-03
- **Plan:** [codex-daemon-chat.md](../../plan/codex-daemon-chat.md)

## Scope

- `provider.ACPAdapter` gets `DaemonBridge bool`, set for Codex.
- `engine.ChatLaunch`, not on Windows: `CODEX_PATH` is the running
  executable and `AGENT_SESSIONS_CODEX` the real `codex` (the user's
  `CODEX_PATH` if set, else the one found). `AGENT_SESSIONS_CODEX_DAEMON=off`
  keeps today's launch.
- `cmd/agent-sessions` and `cmd/agent-sessions-cli` call
  `codexbridge.Main` first in `main`.

## Acceptance criteria

- A local Codex chat and a host's Codex chat start the bridge; other agents
  and the Windows build launch as before.

## Tests

- `ChatLaunch` env for Codex with and without a user `CODEX_PATH`, with the
  kill switch, and for a non-bridged agent.

## Evidence

- `provider.ACPAdapter.DaemonBridge`, set for Codex. `ChatLaunch` sets
  `CODEX_PATH` to `os.Executable()` and `AGENT_SESSIONS_CODEX` to the real
  codex (the user's `CODEX_PATH`, else the one found), except on Windows,
  with `AGENT_SESSIONS_CODEX_DAEMON=off`, when no codex is known, or when the
  executable cannot be found; then it launches as before.
- `cmd/agent-sessions` and `cmd/agent-sessions-cli` call `codexbridge.Main`
  first in `main`. The bridge's passthrough now looks a bare `codex` up on
  PATH, as a user's `CODEX_PATH=codex` would need.
- `TestChatLaunchDaemonBridge` (skipped on Windows): found codex, the user's
  codex, turned off (with and without a found codex), no codex, no
  executable. `TestProvidersDeclareACPAdapters` updated. Ran
  `go test ./internal/engine/... ./internal/provider/... ./internal/codexbridge/... ./cmd/...`,
  golangci-lint on them (nothing in the changed files), and builds for
  GOOS=windows and darwin.
- End to end on this computer, with the managed codex-acp 2.6.0 and a
  freshly built `agent-sessions-cli` as `CODEX_PATH`, while another client
  held the session in the daemon: `session/resume` succeeded at once through
  the bridge; with `CODEX_PATH=codex`, and with the bridge turned off, it was
  refused with `thread_active_writer`.
- Not checked here: the desktop app and a host (CDC-05).

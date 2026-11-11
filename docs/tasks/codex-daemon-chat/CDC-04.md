# CDC-04 — Launch Codex chats through the bridge

- **Status:** ⬜ Not started
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

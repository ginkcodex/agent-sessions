# ACP-05 — Adapter commands and detection per provider

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** ACP-04
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- The optional interface `provider.ACPLauncher { ACPCommand(dir string)
  (provider.Command, error) }`.
- Implement it for Claude (`claude-agent-acp`, `CLAUDE_CODE_EXECUTABLE`),
  Codex (`codex-acp`, `CODEX_PATH`) and OpenCode (`opencode acp`), with lookup
  through `launch.Find`.
- Path overrides in the config file (`[chat]` section), with the matching
  settings keys.
- An engine `ChatAgents()`: for each agent, whether its adapter is available,
  the resolved path, and the install command to show when it is missing.
- Reuse the `knownDirectory` check from `NewSessionLaunch`.

## Likely files

- `internal/provider/provider.go`, and `claude/`, `codex/` and `opencode/`
- `internal/config/`, `internal/engine/service.go`

## Acceptance criteria

- Detection never runs a network install, and never runs the adapter itself.
- A missing adapter reports the install command for that agent.
- Overrides win over PATH, and an override that does not exist is reported.

## Tests

Unit tests with fake executables on a temporary PATH, override parsing, and
an unknown directory refused.

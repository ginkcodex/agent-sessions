# ACP-04 — Agent process and client lifecycle

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-03
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- Start the agent with a `provider.Command` (argv, dir and env) in its own
  process group or job. Close it by closing stdin, then terminate, then kill
  the tree after a grace period. Reuse or extract the code-diagnostics process
  helpers.
- Drain stderr into a bounded ring buffer.
- `acp.Client`:
  - `Initialize`, which sends protocol v1 and no `fs` or `terminal`
    capability;
  - `NewSession`, `LoadSession`, `ResumeSession`, `Prompt` (blocks until the
    stop reason) and `Cancel`;
  - an update callback, and a permission callback that returns `selected` or
    `cancelled`;
  - `Close`.
- `internal/acp/acptest`: a fake agent binary built by tests. Scripted turns
  stream chunks, a tool call, a permission request and a plan.

## Likely files

- `internal/acp/client.go`, `internal/acp/process*.go`
- `internal/acp/acptest/`
- `internal/codediagnostics/process_*.go` (if shared)

## Acceptance criteria

- A closed client leaves no child process, including children of the agent.
- A cancelled turn answers outstanding permission requests `cancelled`.
- An agent that exits mid-turn fails the turn with the stderr tail.

## Tests

Fake-agent tests on Linux, with `-race`. Process-tree cleanup is checked with
a fake agent that spawns a child. Windows job cleanup is compiled; it runs in
CI where available.

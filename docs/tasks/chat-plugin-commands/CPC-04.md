# CPC-04 — Read-only MCP and plugin inventory backend

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** CPC-02
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Implement the inventory contracts verified in CPC-02
([contract](../../plan/chat-plugin-commands-contract.md)). `/mcp` needs no
backend, because both adapters handle it. Expose a host-side, bounded,
read-only `/plugin` listing:

- Claude: `claude plugin list --json`.
- Codex: `codex plugin list --json`, with a timeout of at least 90 s and
  cancellation.

Run it with the session cwd, resolved CLI, login environment and selected
host. O‍penCode returns "unsupported" (see [CPC-09](CPC-09.md)). Return typed safe inventory fields and
explicit unsupported/error states. No mutation commands or credential reads.

## Likely files

- `internal/engine/`, `internal/provider/`, existing launch/loginenv helpers
- `internal/app/`, `internal/rpc/`, remote server handlers
- `frontend/src/lib/api.ts`, types and mock API

## Acceptance criteria and tests

- Plugins without commands and disabled installations remain visible.
- Distinguish native installed/enabled state from known loaded session state.
- Timeouts, cancellation, missing CLI, nonzero exit, malformed/oversized output
  and older CLI support are covered without falling back to model prompting.
- Fixed argv and allowlisted operations; no shell interpolation of user input,
  secret output, credentials/config payloads or installation side effects.
- Tests verify local/remote routing and workspace isolation with fake runners;
  run focused backend suites and RPC compatibility tests.

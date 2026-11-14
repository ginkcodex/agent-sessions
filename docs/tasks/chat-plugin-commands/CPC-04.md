# CPC-04 — Read-only MCP and plugin inventory backend

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** CPC-02
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Implement the inventory contracts verified in CPC-02. Prefer the adapter for
session MCP status. Where necessary expose a host-side bounded read-only native
CLI listing (Claude `plugin list --json`) using the session cwd, resolved CLI,
login environment and selected host. Return typed safe inventory fields and
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

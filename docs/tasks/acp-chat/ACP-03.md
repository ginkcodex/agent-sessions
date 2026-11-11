# ACP-03 — ACP protocol types and JSON-RPC connection

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-02
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

New package `internal/acp`:

- Protocol v1 types: initialize, session new/load/resume/prompt/cancel,
  `session/update` with the kinds in the plan, `session/request_permission`,
  `session/set_config_option` and `configOptions`, and content blocks.
  Unknown update kinds keep their raw JSON, and so does `_meta` (Codex puts
  command output there; see [the spike](../../plan/acp-chat-spike.md)).
- A two-way newline-delimited JSON-RPC connection over an `io.Reader` and
  `io.Writer`:
  - requests with IDs, notifications, and replies matched to callers;
  - the agent's request IDs echoed exactly as received. They start at `0`
    (seen in the spike), so `0` is a valid ID, never "missing";
  - a handler for the agent's requests (`fs/*` and `terminal/*` answer
    method-not-found);
  - a frame cap; a frame over it ends the connection with an error;
  - context cancellation of pending calls;
  - a clean shutdown when the reader hits EOF.

## Likely files

- `internal/acp/types.go`, `internal/acp/conn.go`, tests beside them

## Acceptance criteria

- Calls in both directions work at the same time. Writes are serialized.
- A malformed line is reported and skipped, not fatal. An oversized frame
  ends the connection.
- Pending calls fail with the connection error once the agent goes away.

## Tests

Unit tests with an in-process fake agent over `io.Pipe`: ordering,
concurrency (`-race`), unknown updates, malformed and oversized frames, EOF
and cancellation.

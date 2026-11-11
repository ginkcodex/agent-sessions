# ACP-03 — ACP protocol types and JSON-RPC connection

- **Status:** ✅ Done
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

## Evidence

- **Added:**
  - `internal/acp/types.go`: protocol v1 types.
    - `SessionUpdate` keeps the raw JSON and the `sessionUpdate` kind, and
      decodes known kinds with `AsChunk`, `AsToolCall`, `AsPlan`,
      `AsConfigOptions` and `AsCurrentMode`.
    - `ToolCall` uses pointers, so absent fields stay distinguishable in a
      `tool_call_update`.
    - `mcpServers` is always sent as `[]`.
    - `IsAuthRequired` recognizes code `-32000`.
  - `internal/acp/conn.go`: `Conn`.
    - The agent's request IDs are kept raw and echoed exactly.
    - Notifications run on the read loop, in order and before the reply that
      follows them.
    - The agent's requests run in goroutines; with no handler they get
      method-not-found.
    - A 64 MiB frame cap; an oversized frame ends the connection with
      `ErrFrameTooLarge`.
    - `Close` fails pending calls with `ErrClosed`, cancels the handlers'
      context, closes the writer (the agent's stdin) and drains the rest.
- **Tests:** `internal/acp/conn_test.go` and `types_test.go` cover:
  - round trips and agent errors;
  - request IDs `0` and strings;
  - method-not-found for `fs/*` and `terminal/*`;
  - 200 updates in order before the reply;
  - 50 concurrent calls each way, with replies in reverse order;
  - malformed lines and unknown reply IDs, which are reported and skipped;
  - an oversized frame and a frame exactly at the limit;
  - agent EOF, a last line without a newline, and context cancellation with
    a late reply;
  - `Close` with a permission handler waiting;
  - payload shapes taken from the spike (Claude diff, Codex `_meta` terminal
    output, grouped config options, permission options).
- **Results:**
  - `go test -race -count=30 ./internal/acp` passed, with 92.4 % statement
    coverage.
  - `golangci-lint run ./internal/acp/...` reported 0 issues.
  - All packages compiled (`go test -run 'x^' ./...`).
- **Not checked:** nothing here talks to a real agent; that starts in
  ACP-04. The `internal/rpc` framing was not reused, because importing
  `internal/rpc` would cycle through `engine`. `readLine` is a small copy of
  it.

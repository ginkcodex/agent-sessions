# CDC-03 — The Codex daemon bridge

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CDC-01
- **Plan:** [codex-daemon-chat.md](../../plan/codex-daemon-chat.md)

## Scope

New package `internal/codexbridge`:

- `Main(args []string) (code int, ok bool)`: does nothing (`ok` false) unless
  `AGENT_SESSIONS_CODEX` names the real `codex`.
- Args exactly `app-server`, not turned off by
  `AGENT_SESSIONS_CODEX_DAEMON=off`, and `codex app-server daemon version`
  reports `running` with a `socketPath`: dial the socket, WebSocket
  handshake, stdin lines to text frames, frames to stdout lines. Exit when
  stdin ends or the socket closes.
- Otherwise run the real `codex` with the same arguments and the variable
  removed, and exit with its code.
- `github.com/coder/websocket` becomes a direct dependency.

## Acceptance criteria

- Lines and frames of several MB pass both ways unchanged.
- No daemon, a dead socket, a bad `daemon version` answer, or extra args all
  run the real `codex`.

## Tests

- A fake daemon (WebSocket server on a temp Unix socket) and a fake `codex`
  script printing `daemon version` JSON: round trip of small and large
  messages, close on stdin EOF, exit when the server closes.
- Fallback cases run the fake `codex` and keep its exit code and args.

# CDC-03 — The Codex daemon bridge

- **Status:** ✅ Done
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

## Evidence

- `internal/codexbridge`: `Main`, `DaemonSocket`, `Dial`, `Copy`; the real
  codex runs through `syscall.Exec` on Unix (signals and pipes reach it as
  before) and as a child elsewhere. `github.com/coder/websocket` is now a
  direct dependency.
- Tests (`bridge_unix_test.go`, Unix only; a fake daemon on a Unix socket and
  a fake `codex` script): a 6 MB line both ways, CRLF lines, a pretty-printed
  message kept on one line, exit 0 on stdin EOF and on the daemon's close;
  `daemon version` answers that fail, say stopped, lack a path or are not
  JSON; the test binary run as `Main` bridges plain `app-server`, and runs the
  real codex (exit code and args kept, `AGENT_SESSIONS_CODEX` removed) for
  `-c` overrides, `login`, no daemon, `AGENT_SESSIONS_CODEX_DAEMON=off` and a
  dead socket (with a stderr note).
- Ran `go test -race -count=3 ./internal/codexbridge/`, golangci-lint on the
  package (0 issues), `go vet` for GOOS=windows and darwin.
- Against this computer's real daemon (Codex 0.167.1): `initialize` through
  the bridge answered in 21 ms and the bridge exited 0 at once when stdin
  closed.
- Found: on stdin EOF, replies still in flight are dropped (the WebSocket
  close handshake discards them). codex-acp closes stdin only when it is done
  with the app-server, so nothing waits for them.

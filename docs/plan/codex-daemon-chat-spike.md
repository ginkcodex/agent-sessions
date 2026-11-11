# Codex daemon chat — spike

Supports [codex-daemon-chat.md](codex-daemon-chat.md). Run on 2026-11-12
on Linux, Codex CLI and daemon 0.167.1, codex-acp 2.6.0 (the app's managed
copy).

## What held the session

- `~/.codex/state_5.sqlite` and `~/.codex/logs_2.sqlite` showed the Codex
  CLI resuming the session through the daemon over its control socket
  (`rpc.transport="unix_socket"`), quitting (`thread/unsubscribe`), and the
  daemon shutting the thread down 60 s later:
  `thread … has no subscribers and is idle; shutting down`.
- The chat's adapter resumed over its own `codex app-server`
  (`rpc.transport="stdio"`, `client_name="agent-sessions"`).
- A watcher on `/proc/locks` caught a refused chat start: the session's
  `thread-writer-locks/<id>.lock` had a `FLOCK WRITE` held by the daemon's
  pid, eight seconds after the CLI quit.
- codex-acp's error carries
  `data: {"reason": "thread_active_writer", "threadId": …, "details":
  "thread … already has an active writer"}`.

## The control socket

- `codex app-server daemon version` prints
  `{"status":"running", …, "socketPath": "…/app-server-control.sock", …}`
  in about 20 ms, and exits 1 with "failed to connect" when no daemon runs.
- `codex app-server proxy` copies stdio bytes to the socket, which speaks
  WebSocket: a plain JSON-RPC line gets no answer, an HTTP upgrade gets
  `101 Switching Protocols`. So `proxy` cannot stand in for `app-server`
  under codex-acp, which writes JSON lines.

## The bridge

A throwaway Go program: dial the socket, WebSocket handshake
(`github.com/coder/websocket`), stdin lines to text frames, frames to
stdout lines. With a second client holding the session in the daemon
(`thread/resume` through the same bridge):

| codex-acp started with | `session/resume` |
|---|---|
| `CODEX_PATH=codex` (own app-server) | refused, `thread_active_writer` |
| `CODEX_PATH=<bridge>` | ok: models, modes, configOptions |

Not tried in the spike: a full prompt turn through the bridge, and a host
reached over SSH. CDC-05 covers both.

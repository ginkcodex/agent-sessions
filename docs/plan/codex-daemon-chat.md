# Codex daemon chat

## Goal

A Codex session the user just left in the Codex CLI, app or IDE extension
opens in a chat at once, as a Claude Code session does. Today the chat is
refused for about a minute with a raw error:

```
rpc error -32603: acp: Invalid request: This Codex session is in use by
another Codex client (…). Close the session there or quit that client,
then try again. (code -32600)
```

When the chat is still refused (another client really is using the
session), the message says what is going on and offers to try again.

## Why it happens

Codex 0.167 runs a shared background app-server, the managed daemon
(`codex app-server --managed-daemon`). The CLI, the Codex app and the IDE
extension connect to it over its control socket, so they share every
session it has loaded.

The chat adapter, codex-acp, does not: it starts its own
`codex app-server` on stdio. Codex lets one process write a session at a
time, with a `flock` on `$CODEX_HOME/thread-writer-locks/<thread>.lock`.
While the daemon has the session loaded, the chat's app-server is refused
with `thread <id> already has an active writer`, which codex-acp turns into
the ACP error above (code -32600, `data.reason: "thread_active_writer"`).

The daemon keeps a session loaded for 60 s after its last client leaves
(`thread … has no subscribers and is idle; shutting down`), so every chat
opened in the minute after quitting the CLI is refused. The CLI itself never
waits: it attaches to the daemon that holds the lock. Claude Code has no
daemon and no lock, so its sessions open at once.

The evidence and the spike are in
[codex-daemon-chat-spike.md](codex-daemon-chat-spike.md).

## Design

### A clear "in use" error (CDC-02)

- `engine` recognises the ACP error whose `data.reason` is
  `thread_active_writer` and returns a `SessionInUseError` wrapping a new
  sentinel, `ErrSessionInUse`:
  "Codex has this session open elsewhere: in its CLI, app or IDE extension,
  or for about a minute after one of them closed it. Close it there, or try
  again in a minute."
- `rpc` carries it as `session_in_use`, both ways, so a host's chat gives
  the same text as a local one, without the `rpc error -32603` and
  `(code -32600)` wrapping.
- The chat opener shows it as "Codex is still using this session" with a
  **Try again** action that repeats the same start.

### The chat shares the daemon (CDC-03, CDC-04)

codex-acp starts `$CODEX_PATH app-server [args]`. The app already sets
`CODEX_PATH` to the user's `codex`. On Linux and macOS it now sets it to
the app's own executable, and the real `codex` path in
`AGENT_SESSIONS_CODEX`. Run that way, the executable is a small bridge
(`internal/codexbridge`), not the app:

- **`app-server` with no other argument, and a daemon running:** it asks
  `codex app-server daemon version` for the daemon's `socketPath`, opens a
  WebSocket on that Unix socket (the protocol the CLI uses), and copies
  newline-delimited JSON-RPC from stdin to text frames and frames back to
  stdout lines. codex-acp then talks to the daemon as the CLI does, and
  shares the session instead of fighting for its lock.
- **Anything else** (extra `-c` overrides, any other subcommand such as
  `login`, no daemon, the daemon unreachable): it runs the real `codex`
  with the same arguments, as today.

Both binaries check for `AGENT_SESSIONS_CODEX` first thing in `main`, as the
desktop app already does for its askpass helper: the desktop app for local
chats, `agent-sessions-cli serve` for chats on WSL and SSH hosts.

## Decisions

1. **Bridge only plain `app-server`.** codex-acp adds `-c` overrides for a
   gateway or custom model provider and for hooks. The daemon is shared and
   already configured; it cannot take per-client overrides, so those chats
   keep their own app-server and its lock.
2. **Never start the daemon.** The bridge uses a daemon that runs. Without
   one, nothing holds the lock after the CLI quits, so there is nothing to
   share.
3. **Ask Codex for the socket**, with `codex app-server daemon version`
   (about 20 ms), instead of guessing the path under `$CODEX_HOME`.
4. **Not on Windows.** The daemon listens on a Unix socket; the Windows
   build keeps today's launch.
5. **A way out.** `AGENT_SESSIONS_CODEX_DAEMON=off` in the app's environment
   turns the bridge off.
6. **No automatic retry** of a refused chat: the session may really be in
   use, and only the user knows. One click retries.

## Risks

- **Two writers on one session.** Through the daemon, a chat and a Codex
  CLI can drive the same session at once, as two Codex clients already can.
  The app's own owner lock still keeps a terminal tab and a chat apart; a
  Codex CLI run outside the app was never covered by it
  ([acp-chat](acp-chat.md) risks).
- **The daemon's environment.** Commands the agent runs inherit the
  daemon's environment, not the one the app gave the adapter. This matches
  the CLI.
- **Version skew.** The daemon updates itself and may differ from the
  user's `codex`. The CLI accepts that skew; codex-acp talks the same
  app-server protocol.
- **An undocumented interface.** The control socket's WebSocket protocol and
  `daemon version`'s JSON are what Codex's own CLI uses, not a public API.
  Any failure falls back to the old launch; the spike notes record the
  tested version (Codex 0.167.1, codex-acp 2.6.0).
- **Large messages.** Rollout reads can be many MB; the bridge reads frames
  of any size the daemon sends and lines of any length.

## Release criteria

- Quitting `codex resume <id>` and opening the session in a chat at once
  works, on this computer and on a WSL or SSH host.
- With no daemon running, chats start as before.
- A chat still refused shows "Codex is still using this session" with Try
  again, locally and on a host.
- `AGENT_SESSIONS_CODEX_DAEMON=off` restores the old launch.
- The Windows build launches Codex chats as before.

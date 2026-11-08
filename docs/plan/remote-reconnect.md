# Faster recovery from interrupted remote connections

Planned 2026-11-08. Tasks: [remote-reconnect](../tasks/remote-reconnect/README.md).

## Goal

When the link to an SSH or WSL host breaks, the app notices within about
20 seconds, and once the host is reachable again it is back on the same
session within one retry step plus the time to connect. Today it can take
a minute or more, and some failures are never noticed.

## Today

A remote connection is one `agent-sessions-cli serve --stdio` process,
reached through `ssh` (or `wsl.exe`), speaking JSON-RPC over its stdin and
stdout ([M7](M7.md)). Recovery goes: notice the drop → tear the session down
→ wait a backoff step → probe, maybe deploy, start a new server and
initialize → reload the frontend. Each step has a gap:

| Step | Code | Gap |
|---|---|---|
| Notice the drop | `internal/remote/ssh.go:152` (`ServerAliveInterval=15`, `ServerAliveCountMax=3`) | Left to OpenSSH: about 45 s for a silent network loss (expected from the settings, not measured). |
| | `internal/remote/session.go:265` (`heartbeat`) | `$/heartbeat` is a notification. It keeps the server from idling out but gets no reply, so the client never learns the link is dead. A live SSH link to a stuck server is never noticed. |
| | `internal/remote/wsl.go` | WSL has no keepalive at all. A frozen `wsl.exe` or VM that keeps its pipes open is never noticed. |
| Redial | `internal/app/remote.go:353` | A retry of the same host reuses its ControlMaster (Linux, macOS). After a network loss the master's TCP connection is dead too, so the new attempt hangs on it until the master's own keepalive gives up. |
| | `internal/remote/probe.go:65` | Unix probes have no overall time limit, so that credential prompts can wait for the user. An automatic retry that stalls in the probe stays stuck. Windows and WSL probes stop after 45 s. |
| Backoff | `internal/app/remote.go:38`, `:391`, `:457` | 1, 2, 5, 10, 30 s, then back to 1 s. Not wrong, but not chosen on purpose. |
| Retry by hand | `frontend/src/lib/components/common/ReconnectBanner.svelte` | Retry shows only while disconnected, not while an automatic retry is waiting or stuck. |
| Reload | `frontend/src/lib/stores/appState.svelte.ts:447` | A reconnect to the same host resets like a host switch: the selected session, its group and the back/forward history are cleared, and the first session is opened. |

The existing reconnect test (`internal/app/sshd_test.go:77`) kills the
server with `pkill`, which closes the pipes at once, and shortens the
backoff to 200 ms. No test covers a silent link.

## Design

### 1. RPC liveness watchdog (client)

`rpc.Client` records when it last received any bytes from the server, not
just whole frames: a large transcript that takes 30 s to arrive over a slow
link is progress, not silence. A counting reader wraps the transport below
`FrameReader`.

A watchdog runs for the life of the session, from the preface on:

- When nothing has arrived for `livenessProbeAfter` (5 s), it sends a
  `$/ping` request without waiting for the write to finish (a write can
  block on a full pipe when the network is gone). One ping at a time.
- When nothing has arrived for `livenessDeadline` (20 s), the client closes
  with `ErrLinkSilent`. The session's existing `watchClient` then kills
  `ssh` or `wsl.exe`, and `Session.Done` fires as for any other drop.

Any reply counts, including an error. A server built before this change
answers `$/ping` with "not initialized" or "method not found", which is
still proof of life, so no protocol version bump is needed.

Both durations are variables so tests can shorten them.

### 2. `$/ping` on the server

`rpc.Server` answers `$/ping` itself, before the initialize check and
without the backend or the startup gate, in a handler goroutine so the read
loop never waits on the writer. The reply is `{}`. Like every frame it also
resets the idle timer.

`$/heartbeat` stays: the server's idle timeout counts frames from the
client, and a busy server that streams notifications would otherwise see a
quiet client.

### 3. Fresh connection after a silent drop

When a session ends with `ErrLinkSilent`, the connection loop closes the
host's ControlMaster (`ssh -O exit`) before redialing, so the next attempt
opens a new TCP connection instead of queueing behind a dead one. It leaves
the master alone while a built-in terminal tab uses it (`busy(host)`, as on
host switch): closing it would end that terminal even if the link comes
back. That case falls back to the master's own keepalive.

The banner says why the link was dropped ("no response from the host for
20 s") instead of "connection lost".

### 4. Time limit on automatic attempts

An automatic retry (after the first successful connect, or after a failed
attempt) gets the same 45 s probe limit as Windows and WSL. The limit
pauses while an askpass prompt for this connection is open, so a user
typing a password is not cut off. The first connect the user starts keeps
today's behavior. Deploy keeps its own limits; the serve handshake keeps
its 60 s.

### 5. Backoff and Retry now

- The schedule becomes 1, 2, 5, 10 s, then 10 s for every later attempt.
  A failed attempt against an unreachable host is cheap, and recovery
  speed is the point. No jitter: each window has one connection per host.
- Retry shows while reconnecting too. It calls `Connect(host)` as today,
  which already cancels the running attempt (new generation) before it
  dials, so two attempts never overlap.

### 6. Same-host reconnect keeps the user's place

`appState.reset` gets a same-host path: keep the selected group, the
selected session and the history; reload groups and sessions in the
existing order (groups first, for the stale-group reconciliation); then
reload the selected session. If it no longer exists, fall back to today's
behavior. A host switch or a return to Local still resets everything.

Go to File already keeps editors on a same-host reconnect and reattaches
cleanly (`files.svelte.ts:393`); that stays as it is.

### 7. Measurement

Connection phase changes are logged with the time since the previous phase
(`agent-sessions: connection <host>: reconnecting after 21.3s connected
(no response from host)`), so native runs on Windows and macOS can be timed
without a debugger. The opt-in Docker sshd harness gets two silent-failure
scenarios: the network cut (iptables in the container drops every packet
to and from sshd) and the container frozen (`docker pause`). Both run with
production timings.

## Decisions

- **Watch bytes, not replies.** A ping-reply deadline alone would drop a
  link that is busy delivering one large response. Any received byte shows
  the link and the server are alive.
- **20 s deadline.** Four probe intervals, well under OpenSSH's 45 s, and
  long enough that a short Wi-Fi hiccup or a loaded host does not drop a
  healthy session. Revisit with the RR-01 measurements.
- **Do not lower `ServerAliveInterval`.** The watchdog covers SSH, WSL and
  a stuck server alike; a tighter SSH keepalive would only cover SSH and
  would end terminal tabs on short hiccups too.
- **No session resume.** Every reconnect still starts a fresh server and
  initializes. Resuming RPC state across processes is a much larger change,
  and the slow parts are detection and redial, not initialize.
- **Mutations are never retried**, as in M7. A mutation cut off by the
  watchdog is reported as "outcome unknown", like any other drop.

## Risks

- **False drops.** The client's read loop runs notification handlers
  inline; a handler that blocks stops reads and could trip the watchdog.
  RR-02 checks the handlers and tests a slow handler. A laptop resuming
  from sleep may see one forced reconnect; that is acceptable, since its
  TCP connections are usually dead by then.
- **Terminal tabs on a busy master** still wait for OpenSSH's keepalive
  after a network loss; this plan does not change terminal recovery.
- **Askpass pause** needs the broker to report open prompts per
  connection; Windows has no broker (non-interactive SSH), so the limit is
  never paused there, as today.
- **Docker scenarios approximate** real network loss: container iptables
  drops packets locally rather than across a WAN.
  Native Windows, WSL and macOS checks stay manual.

## Release criteria

- **Met (2026-11-08):** With the Docker sshd harness at production timings: after
  a network cut and after `docker pause`, the app reports
  reconnecting within 25 s, and connects again within 15 s of the fault
  being undone. Before and after numbers are recorded in
  [remote-reconnect-qa.md](remote-reconnect-qa.md).
- **Met (shortened test timers):** A server that stops reading (live transport, stuck process) is detected
  within 25 s in a unit test; a link that delivers a large response slowly
  for longer than the deadline is not dropped.
- **Met (store regressions; native UI not checked):** A same-host reconnect leaves the selected session open.
- **Met (reruns recorded in QA):** `go test -race ./...`, `make test-ssh`, frontend tests and checks pass. Repository-wide lint remains **not met** (135 findings; see QA record).
- **Met (recorded, not executed):** Native Windows (SSH and WSL) and macOS runs are recorded as done or as
  not performed, never assumed.

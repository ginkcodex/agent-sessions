# Remote reconnect — measurements and QA

Supports [remote-reconnect.md](remote-reconnect.md). Each run records the
date, the commit, the scenario, the production timings used, and the time
from the fault to `reconnecting` and from undoing the fault to `connected`.

## Baseline (before the change)

Measured 2026-11-08 on Linux, commit `4aeb573` plus the RR-01 logging and
harness changes, before the liveness changes. Production SSH keepalives,
handshake timeout and reconnect backoff; local Docker sshd, one run each.

| Scenario | Fault → drop reported | Restore → connected | Actual outage | Result |
|---|---|---|---|---|
| Network cut (iptables DROP both directions) | 59.5 s | 0.3 s | 64.5 s | Pass |
| Frozen host (Docker pause) | 59.3 s | 0.2 s | 64.3 s | Pass |

The harness holds the fault for at least 60 s and at least 5 s after the
drop is reported. The fast recovery here is on a local host and is not a
claim about WAN or native-client recovery. Both scenarios completed an RPC
call before and after the fault. Command: `AGENT_SESSIONS_SSH_DOCKER=1 go
test -race -count=1 -timeout 20m -v -run
'TestSSHD_RecoversFrom(NetworkCut|FrozenHost)$' ./internal/app/` (pass).

Phase timings are written to the client's standard logger. A Windows GUI
build needs stderr capture/redirection to collect those lines. Native
checks below have not been performed.

## After the change

Not measured yet ([RR-07](../tasks/remote-reconnect/RR-07.md)).

## Native checks

| Client | Host | Scenario | Result |
|---|---|---|---|
| Windows | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Windows | WSL | `wsl --terminate <distro>` while connected | Not performed |
| macOS | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Linux | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |

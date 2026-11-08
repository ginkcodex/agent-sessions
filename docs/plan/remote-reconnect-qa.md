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

Measured 2026-11-08 on Linux, `377a94b` plus the RR-08 stderr cancellation
fix and tightened RR-07 bounds (the fix is committed as `94b60bb`). Same
local Docker SSH fixture, production timers, 60 s outages, one run each.

| Scenario | Fault → drop reported | Restore → connected | Result |
|---|---|---|---|
| Network cut (iptables DROP both directions) | 21.0 s | 9.1 s | Pass: ≤25 s / ≤15 s |
| Frozen host (Docker pause) | 21.0 s | 0.2 s | Pass: ≤25 s / ≤15 s |

Both scenarios completed an RPC call before and after the fault. Command:
`AGENT_SESSIONS_SSH_DOCKER=1 go test -race -count=1 -timeout 20m -v -run
'TestSSHD_RecoversFrom(NetworkCut|FrozenHost)$' ./internal/app/` — pass.
Detection is about 65% faster than baseline. Restoration latency depends on
where the probe/backoff is when connectivity returns, so the baseline's
subsecond restoration is not a general performance comparison.

The first integrated run failed the 25 s bound: `exec.Cmd` waited its extra
5 s for an inherited stderr pipe after killing SSH (observed drop at
25.1 s). [RR-08](../tasks/remote-reconnect/RR-08.md) owns and closes that pipe
on cancellation; the measurements above are the rerun after that fix.

### Verification

- `go test -race ./...` — pass on rerun. The first run, concurrent with
  Docker builds, timed out in `TestLiveDiagnosticsOversizeAndOwnership`
  (`max escaped text must fit below the frame limit: context deadline
  exceeded`); targeted backend rerun and full-suite rerun passed.
- Frontend tests, `npm run check`, `npm run build` — pass (Vite large-chunk
  warning only).
- `go tool golangci-lint run` — fails with 135 repository-wide findings,
  including existing unused symbols and formatting issues. No reported
  findings in the new liveness/logging code, askpass budget or stderr fix;
  unrelated lint cleanup was not attempted.
- Final full `make test-ssh` rerun — pass with the strict 25 s detection /
  15 s recovery bounds, including both silent-failure scenarios.
- `go vet ./internal/app/... ./internal/remote/... ./internal/rpc/...` — pass.
- Native platform checks below: not performed; Docker is not a substitute.

## Native checks

| Client | Host | Scenario | Result |
|---|---|---|---|
| Windows | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Windows | WSL | `wsl --terminate <distro>` while connected | Not performed |
| macOS | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |
| Linux | SSH (Linux) | Wi-Fi off 60 s, then on | Not performed |

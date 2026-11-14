# CMQ-02 — ACP client: steering

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CMQ-01
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `internal/acp/types.go`: `InitializeResponse.Meta`; the steering method,
  request, reply and outcomes.
- `internal/acp/client.go`: `Client.CanSteer()` (from
  `_meta.steering.supported`) and `Client.Steer`, always with
  `idleBehavior: "promptRequired"`.
- `internal/acp/acptest/agent.go`: advertise steering unless
  `ACPTEST_NO_STEER=1`; answer `_session/steering` with `injected` during a
  turn (reported in that turn as a chunk `steered:<text>`) and
  `promptRequired` when idle. A `hold` prompt waits for a steer or a
  cancel, so tests can steer into a running turn deterministically.

## Acceptance criteria

- `CanSteer` is true only when the agent advertises it.
- `Steer` during a turn returns `injected` and the turn reports the text;
  when idle it returns `promptRequired` and starts nothing.

## Tests

- `go test -race ./internal/acp/...`

## Evidence

- `go test -race -count=1 ./internal/acp/...`: ok (2.9 s). New tests
  `TestCanSteer`, `TestSteerRequestOptsOutOfNewTurns`, `TestSteer` and
  `TestSteerUnsupported` pass.
- The fake agent's `hold` prompt waits for a steer or a cancel; the engine
  and RPC tests steer into it.
- `go vet` on darwin and with `GOOS=windows`: clean.
- Not checked: the real adapters' replies; that is CMQ-08.

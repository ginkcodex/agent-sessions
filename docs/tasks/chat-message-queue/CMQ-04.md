# CMQ-04 — Engine Send now

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CMQ-02, CMQ-03
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `ChatManager.SendNow(ctx, chatID, queuedID)`:
  - idle: start a turn with the message;
  - working, the agent steers: `Steer`; `injected` emits a `steered`
    update; `promptRequired` or an error puts the message back first in
    the queue (the error is returned);
  - working, no steering: `ErrSteerUnsupported`, the message stays.
- The turn's end waits for steering calls in flight.

## Acceptance criteria

- Send now during a `hold` turn: `queue`, `steered`, the turn's
  `steered:<text>` chunk, then `turn_end`.
- Send now on a paused queue starts a turn with that message.
- An agent without steering refuses Send now while working and keeps the
  message.

## Tests

- `go test -race ./internal/engine/...`

## Evidence

- `go test -race -count=1 ./internal/engine/...`: ok. New tests pass:
  `TestChatSendNowSteers`, `TestChatSendNowStartsPausedQueue` and
  `TestChatSendNowWithoutSteering`.
- With the fake agent, the turn's `steered:<text>` chunk can come before
  the `steered` update (the risk the plan names). The test checks the
  turn's story, not that order.
- A steer that reaches the agent before its prompt gets `promptRequired`
  and goes back to the queue. The test repeats Send now until the steer
  joins the turn, so that path runs, though not on every run.
- Not checked: a steering call that fails with an error (the message goes
  back first in the queue). The fake agent has no way to fail one.

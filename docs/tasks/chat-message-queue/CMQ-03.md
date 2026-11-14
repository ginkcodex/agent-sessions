# CMQ-03 — Engine queue

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** CMQ-01
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `internal/engine/chat.go`:
  - `Prompt` returns `ChatPromptResult`; a working chat queues the message
    (attachments checked first) instead of returning `ErrChatBusy`.
  - A turn that ends with `end_turn` and no error starts the next queued
    message; any other end leaves the queue paused.
  - `Unqueue` removes a message and returns it as a `ChatDraft`.
  - `ChatQueuedPrompt`, `ChatAttachmentInfo`; `ChatState.Queue`,
    `CanQueue`, `CanSteer`; update kind `queue`; `turn_start` carries
    `Prompt` when it came from the queue.

## Acceptance criteria

- Two messages queued during a turn run as the next two turns, in order,
  each with `queue` and `turn_start` updates after the previous `turn_end`.
- A cancelled turn leaves the queue; a new prompt then runs at once, and
  the queue goes on after it ends normally.
- `Unqueue` of an unknown ID fails with `ErrQueuedNotFound`.
- `Chats()` shows the queue.

## Tests

- `go test -race ./internal/engine/...`

## Evidence

- `go test -race -count=1 ./internal/engine/...`: ok (184 s, the whole
  package). New tests pass: `TestChatQueueSendsAfterNormalEnd`,
  `TestChatQueuePausesAfterCancel`, `TestChatQueueChecksAttachments` and
  `TestChatUnqueue`. The transcript tests for a cancelled queued turn pass
  too.
- The event order at a turn's end is `turn_end`, `queue`, then
  `turn_start{prompt}`. The tests check it and the update sequence
  numbers.
- `go vet` on darwin and with `GOOS=windows`: clean.

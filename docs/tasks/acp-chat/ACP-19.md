# ACP-19 — The turn's end follows the resolved permission and question updates

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** ACP-06
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

`ChatManager.Prompt` promises that a turn's updates follow as events "then
`ChatTurnEnded`", but the close path raced that order. Closing a chat
(`ChatManager.Close`) wakes a pending permission or question wait by
cancelling its context and tears the connection down, so the wait's
`permission_resolved`/`question_resolved` update and the turn's `turn_end`
were emitted by two goroutines with nothing ordering them. Under CI load
`turn_end` won and landed before `permission_resolved`;
`TestChatCloseDuringPermission` failed on "last update = …
permission_resolved". The same race existed for questions and for the
cancel and crash paths.

The engine now counts the permission and question waits in flight
(`chat.asking`, signalled by a `sync.Cond` on `chat.mu`) and the turn
goroutine waits them out (`awaitAnswers`) before it emits `turn_end`. Each
wait calls `answerDone` after emitting its last update, so that update
lands before the turn's end however the wait ended: answer, turn cancel,
close or the agent's exit.

## Likely files

- `internal/engine/chat.go`

## Acceptance criteria and tests

- Every update of a turn, including the last of a permission or question
  wait, is emitted before `ChatTurnEnded`; the turn's end still precedes
  the next turn's start.
- `TestChatCloseDuringPermission` and `TestChatCloseDuringQuestion` see
  `turn_end` as the last update.
- No deadlock: a wait that never returns cannot hold the close, and the
  chat's goroutine cleanup is unchanged.

## Evidence

- Done: as in the scope; `chat.asking`, `chat.awaitAnswers` and
  `chat.answerDone` in `internal/engine/chat.go`.
- Mutation check: with a 200 ms delay in the wait's last update and
  `awaitAnswers` disabled, `TestChatCloseDuringPermission` fails exactly as
  CI did ("last update = … Seq:6 Kind:permission_resolved", five of five
  runs); with `awaitAnswers` it passes five of five.
- `go test -race -run 'TestChatCloseDuringPermission|TestChatCloseDuringQuestion|TestChatCancelAnswers|TestChatAnswer|TestChatQuestion|TestChatCloseAll|TestChatCloseDuringStart|TestChatNewSession' -count=100 ./internal/engine/`: pass.
- `go test -race ./internal/engine/ ./internal/acp/... ./internal/rpc/...
  ./internal/app/ ./cmd/...`: pass (the engine's 220 s, the rest seconds).
- `go vet ./internal/engine/`, `gofmt -l internal/engine/chat.go`: clean.
- Not checked: a real agent that ends its turn while a permission request
  is still open (the fake agent never does); the turn's end then waits for
  the answer.

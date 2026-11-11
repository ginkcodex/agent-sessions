# ACP-06 — Engine chat manager

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-05
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

`internal/engine/chat.go`, `ChatManager`:

- `Start` (new, or continue an existing session where ACP-02 says the agent
  supports it), `Prompt`, `Cancel`, `Respond`, `Close`, `CloseAll` and
  `Agents`.
- Chat IDs, and an owner registry keyed by `SessionRef`. A second chat for a
  session is refused with a typed error. `Owned(ref)` is used by manage.
- Emit `chat:update` (with a `seq` per chat) and `chat:exit`. Every
  permission request gets an ID, and all pending requests are answered
  `cancelled` on cancel, close or exit.
- Link a new chat to its `SessionRef` once ACP returns the `sessionId`.
- When a turn ends, ask for a targeted rescan of the agent so the saved
  records arrive quickly.

## Likely files

- `internal/engine/chat.go`, `internal/engine/chat_test.go`

## Acceptance criteria

- No goroutine or process outlives `Close` or `CloseAll`.
- Events for one chat are emitted in `seq` order.
- Ownership is released on every exit path.

## Tests

Race tests with the fake agent: concurrent prompts refused while a turn runs,
close during a permission request, agent crash, CloseAll with several chats,
and ownership refusal and release.

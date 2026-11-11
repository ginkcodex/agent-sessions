# ACP-06 — Engine chat manager

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-05
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

`internal/engine/chat.go`, `ChatManager`:

- `Start` (new, or continue an existing session: `session/resume` when
  advertised, otherwise `session/load` with the replay discarded), `Prompt`,
  `Cancel`, `Respond`, `SetOption`, `Close`, `CloseAll` and `Agents`.
- Keep each chat's `configOptions` current from `config_option_update` and
  `current_mode_update`, and include them in the chat state.
- A prompt that fails with an auth error (`-32000` "Authentication
  required") is reported as a typed error carrying the agent's
  `authMethods`.
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

# CPC-03 — Retain the session command catalog

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CPC-02
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Decode and store ACP available commands as per-chat capability state. Retain
capability updates during setup/load replay while still dropping historical
transcript replay. Expose the latest catalog in state/events and the frontend
chat store, with replacement rather than append semantics.

## Likely files

- `internal/acp/types.go`, `internal/acp/acptest/`
- `internal/engine/chat.go`, chat tests
- `frontend/src/lib/types.ts`, `frontend/src/lib/stores/chats.svelte.ts`
- API/RPC mappings and mocks where state types require them

## Acceptance criteria and tests

- Names, descriptions and argument hints survive; only needed metadata is
  exposed. Empty updates clear commands and malformed updates fail safely.
- Setup-before-response, load replay, early events and reattach retain the
  latest catalog; no command notification becomes transcript text.
- Host switch/close cannot leave another session's catalog behind.
- Add focused Go fake-agent and frontend store tests, run the affected suites
  and frontend typecheck, and record outcomes.

## Evidence

What was done:

- `internal/acp/types.go`: `AvailableCommand` (name, description, input
  hint; `_meta` left out) and `SessionUpdate.AsAvailableCommands`.
- `internal/engine/chat.go`: an `available_commands_update` is capability
  state, not history. It replaces `chat.commands` (an empty list clears it),
  is kept during a load's replay, is emitted as a `commands` chat update
  instead of a `session_update`, and is part of `ChatState.commands`, so
  `chatStart`, reattach and the started event all carry the latest catalog.
  A malformed update keeps the last catalog and emits nothing.
- `internal/acp/acptest`: the fake agent sends a two-command catalog during
  `session/new` (before its reply) and inside the `session/load` replay, and
  a `commands N` prompt word replaces it (0 clears).
- Frontend: `ChatCommand`, `ChatState.commands`, the `commands` update kind;
  the chat store keeps `Chat.commands` from state and replaces it on each
  `commands` event. Host switch and close drop the whole chat, catalog
  included.

Verified (2026-11-15):

- `go test ./internal/engine/ ./internal/acp/... ./internal/rpc/
  ./internal/app/... ./internal/remote/...`: pass. New `TestChatCommands`
  covers new and load setup (catalog in state, the replay does not leak it
  into the tail or as a `session_update`), replacement, clearing and seq
  order; the acp, engine and rpc event-order tests now expect the catalog.
- `go vet` on the touched packages: clean.
- `npm test` (1475 pass, 1 skipped), `npm run check` (0 errors), `npm run
  build`: pass. New store test: a `commands` event replaces the catalog and
  an empty one clears it.

Not checked:

- No real adapter run; the catalog shape comes from the CPC-02 contract.
- The mock backend does not send a catalog yet; CPC-05 adds one for the
  picker. The early-event path (catalog at seq 1, before `started`) is the
  store's existing early-event buffer and is covered only on the Go side.

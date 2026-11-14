# CPC-03 — Retain the session command catalog

- **Status:** ⬜ Not started
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

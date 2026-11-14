# CPC-10 — `/plugin` inventory panel

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CPC-04, CPC-05
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Split from CPC-05. Offer `/plugin` in the composer's command list for every
chat, because no adapter advertises it. Submitting exactly `/plugin` does not
send a prompt. It calls `chatPlugins` (CPC-04) and shows the result in a
transient panel by the composer, never as a transcript message. Rows show
name, marketplace, version, scope, "installed" and "enabled". They show
"loaded" only when the chat's command catalog has a command with that
plugin's prefix (see the contract's membership rule).

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`, or a small panel
  component beside it
- `frontend/src/lib/chat.ts` (pure helper), `frontend/src/lib/stores/chats.svelte.ts`
- frontend tests

## Acceptance criteria and tests

- Loading (Codex can take over a minute), empty, unsupported (OpenCode) and
  failure states are shown truthfully. A plugin with no commands is still
  listed.
- There are no install, enable or disable actions, and `/plugin` never
  reaches the agent as a prompt.
- A close or chat switch drops a result in flight. Closing the panel or the
  chat leaves nothing behind.
- Add pure-helper and component tests; run frontend tests, check and build.

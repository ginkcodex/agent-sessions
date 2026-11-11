# ACP-15 — Session option pickers

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

[The spike](../../plan/acp-chat-spike.md) found that an OpenCode chat can
fail on its first prompt because the default model is not logged in, and that
the agent's permission mode decides whether permission requests appear at
all. This task adds:

- Pickers in `ChatComposer` for the chat's `configOptions`, shown for any
  option the agent returns. The spike saw:
  - Claude: `mode`, `model`, `effort`;
  - Codex: `mode`, `collaboration_mode`, `model`, `reasoning_effort`;
  - OpenCode: `model`.
- A change calls `ChatSetOption`, and the picker shows the value the agent
  reports back (`config_option_update`, `current_mode_update`).
- The last choice is remembered per agent and host, and applied when a new
  chat starts. A saved value the agent no longer offers is skipped.
- Modes that skip permission requests (Claude `bypassPermissions`, Codex
  `agent-full-access`) are marked as such. The app never picks them on its
  own.

## Likely files

- `frontend/src/lib/components/chat/ChatOptions.svelte`,
  `ChatComposer.svelte`
- `frontend/src/lib/stores/chats.svelte.ts`, `frontend/src/lib/api.ts`
- `internal/app` binding `ChatSetOption`, and the remote route if ACP-13 is
  done

## Acceptance criteria

- Changing the model of a new OpenCode chat before the first prompt makes the
  prompt use that model.
- The picker always matches the value the agent reports.
- No mode is changed without a user choice, except for a remembered choice
  that the user made earlier.

## Tests

Component tests for picker rendering and updates from the agent, store tests
for remembered choices, and a binding contract entry.

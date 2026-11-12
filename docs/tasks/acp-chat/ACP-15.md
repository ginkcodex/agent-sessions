# ACP-15 — Session option pickers

- **Status:** ✅ Done
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
  own, and never remembers them: each chat starts with permission requests
  unless the user picks such a mode in that chat.

## Likely files

- `frontend/src/lib/components/chat/ChatOptions.svelte`,
  `ChatComposer.svelte`
- `frontend/src/lib/stores/chats.svelte.ts`, `frontend/src/lib/api.ts`
- `internal/app` binding `ChatSetOption` (added locally in ACP-07), and the
  remote route if ACP-13 is done

## Acceptance criteria

- Changing the model of a new OpenCode chat before the first prompt makes the
  prompt use that model.
- The picker always matches the value the agent reports.
- No mode is changed without a user choice, except for a remembered choice
  that the user made earlier. A remembered choice is never a mode that skips
  permission requests.

## Tests

Component tests for picker rendering and updates from the agent, store tests
for remembered choices, and a binding contract entry.

## Evidence

What was done:

- `frontend/src/lib/chatOptions.ts` (pure, apart from storage):
  - `pickers` turns `configOptions` into pickers: `select` options with
    values (groups flattened, in the agent's order) and `boolean` options.
    Options of other types, or without values, get no picker. A current value
    the agent does not list still shows.
  - `skipsPermissions` marks Claude's `bypassPermissions` and Codex's
    `agent-full-access`, on the mode option only.
  - `choicesToApply` returns the remembered choices the agent still offers
    and that differ from the current value, in the agent's order. It never
    returns a mode that skips permission requests.
  - `loadChoices` and `saveChoice` keep the choices in `localStorage` under
    `agent-sessions:chat-options:<targetKey(host)>:<agent>`, so host names
    are hashed as in other per-host keys. Picking a mode that skips
    permission requests forgets the remembered mode, so the next chat starts
    with the agent's own.
- `ChatOptions.svelte`: one picker per option, shown above the composer's
  input while the chat runs. Each picker shows the value the agent reports.
  Pickers are disabled while the agent starts or while a change is pending.
  A refused change resets the picker to the agent's value. Choices that skip
  permission requests say "(no permission requests)", and while one is
  active a note says the agent runs tools without asking.
- `ChatComposer` takes `onOption`. `TranscriptView` passes
  `chatOpen.choose`, which sets the option, remembers it for the agent and
  the shown host, and shows a toast when the agent refuses.
- `chatOpen.startNew`, and Restart of a chat whose session was never saved,
  apply the remembered choices one at a time before the first prompt. The
  options the agent returns are re-read after each change, because a new
  model can change the efforts on offer. A choice the agent refuses is
  skipped with a toast, and the chat still opens. A continued session keeps
  the options the agent loads with it.
- The mock's turns run on the chat's current model.
- Design change from the first draft: the first version remembered and
  re-applied every choice, permission-skipping modes included. The auto-mode
  safety check blocked that edit. Not remembering those modes matches the
  plan's fail-closed rule, so the plan's risk section now says it.

Tests run:

- `npm test`: 1291 tests, 1290 pass, 0 fail, 1 skipped (as before).
  `npx svelte-check`: 0 errors, 0 warnings. `npm run build`: builds.
  - `chat-options.test.mjs` (new): pickers for the spike's Claude, Codex and
    OpenCode options, including groups, booleans, the permission-skip marks
    per agent, and unlisted current values. `choicesToApply` order, values no
    longer offered or of the wrong type, and the rule that a skip mode is
    never applied. Storage per agent and host, hashed host keys, a skip mode
    forgetting the mode, and damaged or missing storage.
  - `chat-ui.test.mjs`: the composer renders a picker per option, with the
    reported value selected and an `optgroup`. It follows a new reported
    value and shows the no-permission note. Pickers are disabled while the
    agent starts, and absent once the chat ended, without `onOption`, or
    without options. Opener: `choose` sets and remembers, and a refused
    change toasts and is not remembered. The next new chat applies the
    choices in the agent's order. A continued session applies none. Full
    access is set when picked but never applied to the next chat. Another
    host has its own memory. A remembered choice the agent refuses toasts,
    and the chat still opens.
  - `chats-store.test.mjs`: a model set before the first prompt is the
    model the mock's saved reply records.
- Mutation checks: without the skip-mode filter in `choicesToApply`, or
  without the forget in `saveChoice`, `chat-options.test.mjs` fails. Without
  `applyChoices` in `startFresh`, the opener test fails.
- Binding contract: `ChatSetOption` was bound in ACP-07 and routed for hosts
  in ACP-13. `TestFrontendBindingMatchesApp` checks every `*App` method by
  reflection, so it needs no new entry.

Not checked here:

- Real agents. Whether a model picked before the first prompt is the one
  OpenCode runs, and how each agent reports option changes, are ACP-14
  checks. The tests use the mock agent and fixtures shaped on the spike's
  options.
- The picker in a real window (focus, keyboard, layout with long model
  lists). The tests render on the server.

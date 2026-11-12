# CDG-02 — Structured grouped warnings

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CDG-01
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

- `frontend/src/lib/stores/filePrompt.svelte.ts`: `FilePromptItem` and
  `FilePromptGroup` beside `FilePromptChoice`; `ask` gains an optional
  `groups` argument (default `[]`) carried on the request.
- `frontend/src/lib/closeConfirm.ts`: `closeWindowWarning` and
  `endingChatsWarning` return `CloseWarning { message, groups } | null`
  — prose counts as today, plus Terminals and Chats groups with rows
  (name, detail, icon kind, agent, "working" badge) and `more` past six
  per group. `confirmClosingWindow` passes the groups to `ask`.
- `frontend/src/lib/stores/connection.svelte.ts`: the host-switch
  prompts compose their messages from `CloseWarning` and pass its
  groups (the combined terminal-tabs question keeps its tabs paragraph
  and gains the Chats card under it).
- Tests: rewrite `close-confirm.test.mjs` for the structured shape,
  update `connection.test.mjs`'s message assertions, extend
  `file-prompt.test.mjs` for the pass-through.

## Likely files

- `frontend/src/lib/closeConfirm.ts`
- `frontend/src/lib/stores/filePrompt.svelte.ts`
- `frontend/src/lib/stores/connection.svelte.ts`
- `frontend/test/close-confirm.test.mjs`
- `frontend/test/connection.test.mjs`
- `frontend/test/file-prompt.test.mjs`

## Acceptance criteria and tests

- The warnings' `message` keeps today's sentences and counts; the
  groups carry the rows with the plan's names, details, icons, agent
  ids and badges, and `more` past six per group.
- Terminal rows name the tab and its host; chat rows name the agent and
  the directory folder, badge "working" while it works; exited chats
  and exited tabs stay out.
- `ask` carries the groups on the request and defaults to `[]`; every
  existing caller is unchanged.
- The host-switch prompts pass the chats group through, both alone and
  in the combined terminal-tabs question.

## Evidence

- `frontend/test/close-confirm.test.mjs` (rewritten for the structured
  shape): the message sentences and counts as before, the Terminals and
  Chats rows (names, host and directory details, icons, agent ids,
  "working" badges), the per-group six-item cap with `more`, no empty
  cards, and `confirmClosingWindow` passing the groups to `ask`.
- `frontend/test/connection.test.mjs`: the host-switch prompts carry the
  Chats card (alone and in the combined terminal-tabs question) beside
  the unchanged titles and action labels.
- `frontend/test/file-prompt.test.mjs`: `ask` carries the groups on the
  request and defaults to `[]`.
- `npm test` 1380 tests, 0 failures (1 pre-existing skip);
  `npm run check` clean; `npm run build` succeeds.

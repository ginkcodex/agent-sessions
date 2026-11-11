# CEG-02 — Quiet exit notice

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** CEG-01
- **Plan:** [chat-end-guard.md](../../plan/chat-end-guard.md)

## Scope

- `LiveTail.svelte`: the stderr tail shows only for `exited` and `crashed`,
  folded under "Show the agent's log"; `closed` and `disconnected` show one
  line.

## Likely files

- `frontend/src/lib/components/chat/LiveTail.svelte`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria

- A chat the user ended shows "The chat ended." and no log.
- A crashed agent shows its reason and the log in a closed disclosure.

## Tests

- SSR tests per exit reason; `npm test`, `npm run check`.

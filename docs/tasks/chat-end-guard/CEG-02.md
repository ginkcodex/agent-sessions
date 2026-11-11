# CEG-02 — Quiet exit notice

- **Status:** ✅ Done
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

## Evidence

- `LiveTail.svelte`: `exitLog` is the stderr tail for `exited` and
  `crashed` only, in a `<details>` "Show the agent's log", closed by
  default. `closed` and `disconnected` show their one line.
- `chat-ui.test.mjs`: a crash shows its reason and the folded log; `exited`
  shows the disclosure; `closed` and `disconnected` show only their line,
  no log, even when the agent wrote one.
- `node --test test/chat-ui.test.mjs`: 22 passed. `npm run check`: 0 errors.

Not checked: the disclosure in the desktop app (CEG-05).

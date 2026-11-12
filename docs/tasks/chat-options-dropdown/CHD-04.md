# CHD-04 — `WorkspaceStart`'s agent select as a dropdown

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** CHD-02
- **Plan:** [chat-options-dropdown.md](../../plan/chat-options-dropdown.md)

## Scope

Found along the way in CHD-01: `frontend/src/lib/components/sessionlist/WorkspaceStart.svelte`
still uses a native `<select>` for the agent to start a workspace with.
Give it the shared `Dropdown` like the chat's pickers (found work from
the plan's risk note; not part of the chat box).

## Acceptance criteria and tests

- No `<select` in the session list's start pane; the agent picker has
  the shared look and the disabled state while a session starts.
- A test pins the markup (`new-session.test.mjs` or
  `session-row.test.mjs`, where the pane is rendered today).
- `npm test` and `npm run check` pass in `frontend/`.

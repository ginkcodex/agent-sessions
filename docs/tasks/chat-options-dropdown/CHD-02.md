# CHD-02 — The chat's pickers as dropdowns

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CHD-01
- **Plan:** [chat-options-dropdown.md](../../plan/chat-options-dropdown.md)

## Scope

- `frontend/src/lib/chatOptions.ts`: `PickerOption` and the pure
  `pickerOptions(picker)` mapping (the choice's bare name, description
  as title, the agent's groups).
- `frontend/src/lib/components/common/Dropdown.svelte`:
  `placement?: 'bottom' | 'top'` ('bottom' by default) and the menu's
  upward styling.
- `frontend/src/lib/components/chat/ChatOptions.svelte`: one `Dropdown`
  per select picker (`align="right"`, `placement="top"`, placeholder
  `Not set`, tooltip `name: description`); the select fix-ups in
  `change` go, the checkbox reset stays; the native select CSS goes.
- Tests: `frontend/test/chat-options.test.mjs` (`pickerOptions`),
  `frontend/test/chat-ui.test.mjs` (the composer renders dropdowns, no
  `<select>`), `frontend/test/dropdown.test.mjs` (`placement`).

## Acceptance criteria and tests

- The four release criteria for the pickers' look, menu content,
  upward placement and change behavior hold.
- The other dropdowns keep opening downward (default `placement`).
- `npm test` and `npm run check` pass in `frontend/`.

## Evidence

- `pickerOptions(picker)` and `PickerOption` in `chatOptions.ts`;
  `placement?: 'bottom' | 'top'` (default `'bottom'`) and the `.menu.up`
  styling in `Dropdown.svelte`; `ChatOptions.svelte` renders a
  `Dropdown` per select picker (`align="right"`, `placement="top"`,
  placeholder `Not set`, tooltip `name: description`) and `change` lost
  its select fix-ups (the checkbox reset stays).
- Tests: `frontend/test/chat-options.test.mjs` (`pickerOptions` labels
  and groups), `frontend/test/chat-ui.test.mjs` (the composer renders
  dropdowns, no `<select>`; values, `Not set`, busy disabling),
  `frontend/test/dropdown.test.mjs` (`placement`, and the chat pickers
  in the convention test).
- Review: option labels keep the agent's bare names (`Bypass
  Permissions`, `Agent (full access)`); the ` (no permission requests)`
  mark the select carried is gone — the unguarded warning says it.
  Tests updated with it.
- `npm test` in `frontend/`: 1403 tests, 1402 pass, 1 skipped
  (pre-existing), 0 fail. `npm run check`: 0 errors, 0 warnings.
  `npm run build`: succeeds (the chunk-size note is pre-existing).
- Not checked: anything only the desktop app shows (CHD-03).

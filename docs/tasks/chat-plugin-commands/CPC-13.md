# CPC-13 — `/plugin` in the same modal

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CPC-10, CPC-12
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Move the `/plugin` listing from the panel above the composer (CPC-10) into
the command result modal from CPC-12, titled "Plugins". It keeps the same
data, states and loaded rule, and adds the name filter. Remove the panel.

## Likely files

- `frontend/src/lib/components/chat/PluginInventory.svelte` (becomes the
  modal's body, or is removed)
- `ChatComposer.svelte`, `CommandResultDialog.svelte`, frontend tests

## Acceptance criteria and tests

- Every CPC-10 state still works in the modal: loading, empty, unsupported,
  error, and listed with enabled and loaded badges.
- A reply after close is dropped.
- The filter works, and nothing about it reaches the agent.
- Update the CPC-10 tests; run frontend tests, check and build.

## Evidence

- `PluginInventory.svelte` now renders inside `CommandResultDialog` (title
  "Plugins"), filtering by name or marketplace. The panel styles are gone.
  Its states, loaded rule and late-reply guard are unchanged. Closing it
  returns focus to the composer.
- `test/chat-end-menu.test.mjs`: the CPC-10 test now finds the modal. It
  checks the title, that the only button is "Close Plugins", that the
  filter has focus, filtering by name, by marketplace and with no match
  (`no-match`) with nothing sent, that Escape closes it and focus returns to
  the textarea. The unsupported, empty, error and late-reply cases still
  pass.
- `npm test` (1491 pass), `npm run check` (0 errors), `npm run build` pass.
- Browser (vite + mock backend, Playwright): `/plugin` opens the modal with
  the filter focused, "pony" narrows to ponytail with enabled and loaded
  badges, and Escape closes it with focus back in the composer.
- Not checked: the Wails desktop build, a real `claude`/`codex plugin list`
  run in the modal.

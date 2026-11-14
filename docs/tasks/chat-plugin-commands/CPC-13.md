# CPC-13 — `/plugin` in the same modal

- **Status:** ⬜ Not started
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

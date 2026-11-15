# Chat inventory dialog polish

## Goal

Improve the Plugins dialog shown in the screenshot with more breathing room and clearer visual hierarchy.

## Design

Use the existing shared `CommandResultDialog` frame: a slightly wider panel, roomier padding, larger title, readable note, and taller rounded search field. Show plugins as separated, subtly filled cards with name and metadata grouped on the left and pill badges on the right. Allow details and badge groups to wrap on narrow screens. Keep full text and a scrollable results body.

## Decisions

- Reuse theme tokens and existing modal, search and close components. No dependencies, new controls or backend changes.
- Shared frame changes intentionally also apply to `McpServersDialog`. Follow-up CID-02 gives its server groups matching padded cards, identity/detail grouping, filled status pills, and padded loading/error/empty messages and notes. Parsing, filtering and read-only behavior stay unchanged.
- Scope: `CommandResultDialog.svelte`, `PluginInventory.svelte`, `McpServersDialog.svelte`, existing plugin/MCP interaction coverage, and plan/task indexes. Composer callers, API contracts, fixtures, configuration and exports need no changes.
- Associate the explanatory note with the dialog for assistive technology. Preserve search autofocus, focus trap, Escape, close button and composer focus restoration.

## Risks

More padding and two-line rows display fewer results before scrolling. Long plugin names/marketplaces must wrap rather than widen the modal. No session data or plugin state is changed; this remains a read-only inventory.

## Release criteria

- Roomier theme-aware dialog, readable plugin cards and clear enabled/disabled/loaded badges.
- Plugin and MCP filtering, empty/error/loading states, Escape and focus restoration continue to pass existing tests.
- Frontend type check, tests and production build pass; unperformed native/visual checks recorded explicitly.

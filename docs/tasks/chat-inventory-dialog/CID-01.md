# CID-01 — Polish the inventory dialog and plugin rows

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** —
- **Plan:** [chat-inventory-dialog.md](../../plan/chat-inventory-dialog.md)

## Scope

Increase shared dialog padding and search/title spacing. Group plugin identity and metadata in roomier cards with wrapped status badges. Reuse theme tokens and preserve complete text, filtering, read-only behavior and modal focus handling. Link the dialog note via ARIA.

## Likely files

- `frontend/src/lib/components/chat/CommandResultDialog.svelte`
- `frontend/src/lib/components/chat/PluginInventory.svelte`
- `frontend/test/chat-end-menu.test.mjs`
- Plan/task indexes

## Acceptance criteria

- Roomier shared frame, clear plugin name/metadata hierarchy, softly filled pill badges.
- Long names/marketplaces and badge groups can wrap; results scroll within the viewport.
- Dialog note is accessible; existing plugin/MCP filtering, states, Escape and focus behavior remain intact.

## Tests

Run frontend check, tests and production build. Extend the existing plugin interaction test to cover grouped content and note association; do not add presentation-only test scaffolding.

## Evidence

- Shared plugin/MCP frame now uses a 680px maximum width, responsive 16–26px padding, 20px corners, a larger heading and roomier search/body spacing. Search/header stay fixed while the body scrolls.
- Plugin rows are softly filled, bordered cards grouping name and metadata separately from wrapped enabled/disabled/loaded pills. Loading/error/empty states use padded surfaces as well.
- Linked the dialog note using `aria-describedby`; extended the existing plugin interaction test to assert that relationship and identity/metadata/badge grouping.
- `npm --prefix frontend run check`: passed, 642 files, zero errors or warnings.
- `npm --prefix frontend test`: passed, 1,492 passed, zero failed, one skipped (1,493 total), including plugin and MCP dialog interactions.
- `npm --prefix frontend run build`: passed; Vite warned about chunks exceeding 500 kB.
- `git diff --check`: passed.
- Not checked: native Windows or live light/dark/narrow-window appearance. No Go changes, so `make test` and Go lint were not run. More padding means fewer rows visible before scrolling.


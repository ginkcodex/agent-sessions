# CID-02 — Polish MCP server groups and cards

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CID-01
- **Plan:** [chat-inventory-dialog.md](../../plan/chat-inventory-dialog.md)

## Scope

Apply the plugin inventory's card treatment to MCP server results: roomier groups, grouped server name/scope/details, filled status pills and padded state messages/notes. Preserve full text, wrapping, Markdown fallback, status parsing, filtering and read-only behavior. Reuse existing theme tokens; no new dependencies or components.

## Likely files

- `frontend/src/lib/components/chat/McpServersDialog.svelte`
- `frontend/test/chat-end-menu.test.mjs`
- Existing inventory plan, task group and progress indexes

## Acceptance criteria

- Server cards and group headings have more breathing room and clear hierarchy.
- Connected, failed, authentication-needed and neutral status pills retain readable text labels and theme-aware colors.
- Long names/scopes/details wrap; status badges wrap within narrow cards.
- Existing filtering, loading/error/end states, Markdown fallback, Escape and focus behavior remain unchanged.

## Tests

Extend the existing MCP interaction test for identity/detail grouping and status pill classes. Run frontend check, tests and production build. Record any unperformed native or visual checks.

## Evidence

- Server groups now have spaced headings and softly filled, bordered 16px-padded cards. Names/scopes are grouped above detail text; status pills use theme-aware green/red/amber or neutral fills and can wrap on narrow cards.
- Loading, failure, cancellation, exit, no-match and notes use padded surfaces; full reply text, Markdown fallback, parsing and filtering are unchanged.
- Extended the MCP interaction test to assert name/detail grouping, failed/connected pill labels and classes, and the accessible dialog note. Existing tests also exercise filter, end states, Escape and focus restoration.
- `npm --prefix frontend run check`: passed, 642 files, zero errors or warnings.
- `npm --prefix frontend test`: passed, 1,492 passed, zero failed, one skipped (1,493 total).
- `npm --prefix frontend run build`: passed; Vite warned about chunks exceeding 500 kB.
- `git diff --check`: passed.
- Not checked: native Windows or live light/dark/narrow-window appearance. No Go changes, so `make test` and Go lint were not run. Roomier server cards show fewer rows before scrolling.

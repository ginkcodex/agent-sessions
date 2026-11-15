# CPC-15 — Route Codex /plugins to installed inventory

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CPC-04, CPC-13
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Offer Codex's native `/plugins` spelling in the composer and open the installed-plugin inventory rather than sending it as a model prompt. Preserve `/plugin` as a Codex compatibility alias and other providers' existing behavior. Verify backend inventory contracts; track any separate backend defect as a task rather than substituting loaded skills for installed plugins.

## Likely files

- `frontend/src/lib/chat.ts`
- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/test/chat-ui.test.mjs`, `frontend/test/chat-end-menu.test.mjs`
- Provider contract and task indexes

## Acceptance criteria

- Codex's picker offers `/plugins`; either bare Codex spelling opens the read-only inventory without sending a prompt, even if advertised by the adapter.
- Commands with arguments remain ordinary turns; other providers' owned commands are not intercepted.
- Inventory is sourced from the native installed listing, including commandless/disabled entries, not the ACP skill catalog.
- Focus/filter/close behavior and existing provider tests remain passing.

## Tests

Add helper and composer regression coverage for Codex plural/singular, arguments, advertised command and complete inventory independent of loaded commands. Run frontend check/tests/build; record backend inspection findings and native checks not performed.

## Evidence

- Root cause: only `/plugin` was intercepted; `/plugins` reached Codex as an ordinary prompt, so its model answered about available integrations rather than the installed inventory.
- Codex now offers bare `/plugins` in the picker and intercepts both `/plugins` and the old `/plugin` alias, even when advertised. Other agents keep their existing provider-owned command behavior; arguments still go through ordinary turns.
- Reused the existing `chatPlugins` backend: Codex runs fixed `codex plugin list --json` in the chat's launch context with a two-minute timeout. Its parser maps every `installed` entry, including disabled and commandless plugins; no backend filtering defect was established, so no backend changes or filesystem scans were added.
- Added helper and composer regression coverage for plural/singular routing, advertised commands with argument hints, loaded vs commandless/disabled inventory rows, focus restoration and argument pass-through. Updated the provider contract.
- `npm --prefix frontend run check`: passed, 642 files, zero errors or warnings.
- `npm --prefix frontend test`: passed, 1,493 passed, zero failed, one skipped (1,494 total).
- `npm --prefix frontend run build`: passed; Vite warned about chunks exceeding 500 kB.
- `go test ./internal/provider/codex ./internal/engine ./internal/rpc`: passed (existing native inventory/parser/host coverage).
- Not checked: actual Codex CLI inventory on the user's local/WSL host, native Windows UI, full `make test` race suite or Go lint. The CLI's installed list remains authoritative and may take up to two minutes; completeness against the user's actual installation was not independently verified.

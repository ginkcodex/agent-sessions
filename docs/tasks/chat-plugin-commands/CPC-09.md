# CPC-09 — O‍penCode MCP and plugin inventory

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CPC-04
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Found in CPC-02: O‍penCode 2.3.0 has no `/mcp` ACP command, and
`opencode mcp list` / `opencode plugin list` print text only, with no JSON.
The first release says "not supported for O‍penCode". This task adds the
listing once a machine-readable interface exists: a `--json` flag, or a
documented server API route. See the
[contract](../../plan/chat-plugin-commands-contract.md#opencode).

## Likely files

- The CPC-04 inventory backend, plus an O‍penCode branch and fixtures

## Acceptance criteria and tests

- Use a stable, machine-readable O‍penCode interface. Don't scrape text, and
  don't use an interface that connects to or changes MCP servers.
- Apply the same safe-field, timeout and unsupported-version rules as CPC-04.
- Fixture tests for the parser and for errors.

## Evidence

- OpenCode 2.3.0's documented `opencode api` operations `plugin.list`,
  `mcp.list` and `skill.list` are read-only GETs that answer JSON for a
  `location[directory]`. They are the machine-readable interface; text output
  is never scraped. The user pointed out OpenCode's native spellings are
  `/mcps`, `/plugins` and `/skills`, so those are what the picker offers and
  intercepts for OpenCode (`/plugin` and `/mcp` are not).
- Backend: `internal/provider/opencode/inventory.go` (`PluginList(dir)`,
  `InventoryList(kind, dir)`, safe-field parsers); `provider.InventoryLister`;
  `Service.ChatInventory` / `ChatManager.Inventory` sharing CPC-04's CLI run,
  timeout, cancellation and unknown-format rules; the `chatInventory` RPC with
  its capability; `App.ChatInventory`.
- Frontend: `pickerCommands`, `inventoryListing`, `mcpStatusFromServers` in
  `chat.ts`; `InventoryDialog.svelte`; `McpServersDialog` takes listed servers.
- Tests: parser fixtures (builtins dropped, local path reduced to its file
  name, unnamed plugin, skill content dropped, bad output rejected), argv,
  engine service/manager tests, helper tests and a composer test that opens
  all three modals with no prompt sent and leaves `/plugin` an ordinary turn.
- `go vet ./...` and `go test ./...`: passed. `make lint`: 155 findings, all
  in files this task did not touch; none in the changed files.
- `npm --prefix frontend run check`: 0 errors, 0 warnings.
  `npm --prefix frontend test`: 1,500 passed, 0 failed, 1 skipped.
  `npm --prefix frontend run build`: passed.
- Not run: the desktop app against a live OpenCode chat, and remote/WSL
  hosts (CPC-08 QA). The live CLI calls were probed by hand on this host.

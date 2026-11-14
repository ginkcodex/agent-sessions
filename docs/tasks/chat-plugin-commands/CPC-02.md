# CPC-02 — Validate provider inventory and invocation contracts

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CPC-01
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Record a provider support matrix and concrete wire/log shapes in a flat
`docs/plan/chat-plugin-commands-contract.md` supporting note. Validate MCP
listing, installed/enabled/loaded plugin inventory, command membership and
canonical names/aliases for the actual supported CLI/adapter versions.
Inspect sanitized existing command records; never run a plugin as discovery.
Resolve how inventory inspection results are displayed without pretending they
are persisted model messages. Split additional provider-specific work into
new <= 4 h tasks if the verified interfaces exceed CPC-04's budget.

## Likely files

- `internal/acp/`, `internal/engine/chat.go`, `internal/provider/`
- `internal/managedtools/` pinned adapter packages
- `docs/plan/chat-plugin-commands-contract.md`

## Acceptance criteria and tests

- Distinguish supported, unsupported and unverified combinations explicitly.
- Validate Claude `plugin list --json` in workspace context, bounded safe
  output fields and the difference between disk and session state.
- Establish plugin-membership metadata without guessing from colon names.
- Record sanitized marker-only, expanded and argument-bearing command log
  shapes and local-output behavior, without copying unrelated private text.
- Use read-only CLI help/list/protocol probes and existing records; document
  exact versions, commands and outcomes. No paid/model/plugin execution.

## Evidence

- Wrote the [contract](../../plan/chat-plugin-commands-contract.md): versions,
  a support matrix, the `/mcp` and `/plugin` interfaces, safe fields, the
  plugin-membership rule, aliases, and saved record shapes.
- Read-only CLI runs on 2026-11-15:
  - `claude plugin list --json`: under 1 s, 3 plugins.
  - `codex plugin list --json`: 68 s, because it queries remote
    marketplaces. A 20 s timeout killed it.
  - `codex mcp list --json`: I looked at key names only, because transport
    fields can carry headers and tokens.
  - `opencode mcp list` and `opencode plugin list`: text only.
  - `claude mcp --help` confirms that `mcp list` health-checks servers, so we
    don't use it.
- ACP `initialize` + `session/new` probes for both pinned adapters ran in an
  empty `/tmp` workspace, with no prompt.
  - Claude advertised 6 `ponytail:*` and 9 `understand-anything:*` commands,
    each with a `(<plugin>)` description prefix, plus `mcp`.
  - Codex advertised its 11 built-ins and `$plugin:skill` names.
  - Neither adapter sent command `_meta`.
  - Neither probe created a session log.
- Adapter source read:
  - Claude: `mcp-command.js`, where bare `/mcp` is a 5 s status read and the
    arguments mutate servers.
  - Codex: the `case "mcp"` handler, which runs in the adapter and starts no
    model turn.
- Native logs: tallied plugin marker variants across CLI logs (args replaced
  with `ARGS`), and printed one invocation's record sequence by keys and
  prefixes only.
  - 0 of 72 ACP (`sdk-ts`) logs contain command markers, so the ACP saved
    shape is still unverified. CPC-08 must confirm it.
- Follow-up found: O‍penCode has no machine-readable inventory, so that work
  is now [CPC-09](CPC-09.md). With `/mcp` handled by the adapters, CPC-04
  covers `/plugin` listing for Claude and Codex only.
- Not checked: remote hosts, macOS, Windows, and older CLIs without `--json`.
  No plugin, model turn or MCP mutation ran. No product code changed, so no
  tests ran.

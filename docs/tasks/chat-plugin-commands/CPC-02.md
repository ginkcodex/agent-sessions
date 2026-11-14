# CPC-02 — Validate provider inventory and invocation contracts

- **Status:** ⬜ Not started
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

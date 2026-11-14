# CPC-09 — O‍penCode MCP and plugin inventory

- **Status:** ⬜ Not started
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

# CPC-12 — MCP servers modal

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** CPC-11
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

A modal titled "MCP servers" in the style of VS Code's, opened by a captured
`/mcp` (CPC-11). It shares one modal shell with `/plugin` (CPC-13): title,
filter box, close button, Escape, and focus trap via `modalFocus`, in the
app's dialog style.

A pure parser reads the adapters' shared Markdown:

- the summary line;
- the status groups, giving each server's name, status and detail;
- the disabled servers;
- the remaining notes.

The modal shows the servers grouped by status, with a status badge
(Connected, Failed, Needs authentication, Connecting, Disabled, or another
title as given) and the detail. A filter narrows the servers by name. While
the turn runs, the modal shows loading. A failed or cancelled turn shows the
reason. A reply the parser cannot read shows as rendered Markdown. The modal
is read-only: no Add, Reconnect or Enable actions.

## Likely files

- `frontend/src/lib/chat.ts` (`parseMcpStatus`)
- `frontend/src/lib/components/chat/CommandResultDialog.svelte` (new)
- `ChatComposer.svelte` or `TranscriptView.svelte` (opening it)
- frontend tests

## Acceptance criteria and tests

- Parser tests use replies in each adapter's format (from their formatters):
  - no servers;
  - every status group, including Codex's extra statuses;
  - a duplicate name with its scope;
  - errors with Markdown escapes;
  - code spans with backticks;
  - the reconnect hint and notes;
  - unparsable text.
- Component tests cover loading, the grouped rows with badges, the filter,
  no matches, failure, the Markdown fallback, Escape and close, and focus
  returning to the composer.
- Run frontend tests, check and build.

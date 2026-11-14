# CPC-12 — MCP servers modal

- **Status:** ✅ Done
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

## Evidence

- **What was done:**
  - **Parser:** `parseMcpStatus` in `chat.ts` reads both adapters' `/mcp` Markdown:
    - the summary, or "No MCP servers are configured.";
    - `**<Status>**` groups, including unknown titles;
    - rows with code spans of any fence, Claude's `(scope)` and Codex's `(unnamed)`, with escapes undone;
    - the `**Disabled:**` line, shown as a last group;
    - any other block, kept as a Markdown note (the reconnect hint, Codex's reload notes).

    A malformed group, a second summary, or no summary gives null.
  - **Modal shell:** `CommandResultDialog.svelte` has the title, a note, the filter, Close and Escape, `modalFocus` and the dialog tokens. It focuses the filter on open, because the composer still holds focus and `autofocus` would not take it.
  - **MCP dialog:** `McpServersDialog.svelte` shows:
    - loading, failed (with the error), cancelled and exited states;
    - `Title (n)` groups of rows with a status badge: Connected is green with ✓, Failed and Cancelled are red, Needs authentication is amber;
    - the name filter, with a "No servers match" state;
    - notes;
    - the reply rendered as Markdown (`TextPart`) when it does not parse.
  - **Composer:** a bare `/mcp` with no files calls `chats.inspect` instead of `prompt`. Close or Escape dismisses the inspection and gives focus back to the textarea. `/mcp` with arguments is still prompted.
  - **Mock fix:** `mockCommands` now checks `claude-code`. It checked `claude`, so Claude chats in the browser mock advertised no commands. The catalog store test now uses OpenCode, which has none.
- **Tests:** `npm test` passes, 1491 tests. New ones:
  - in `test/chat-ui.test.mjs`: parser tests for Claude's shape (scopes, escapes, a fence inside a name, a disabled name with backticks, the hint) and for Codex's notes, statuses, `(unnamed)` and `~` escapes, plus six shapes it must refuse;
  - in `test/chat-end-menu.test.mjs`, a client-mounted test: the bare `/mcp` is captured while the argument form is prompted; loading; the listed rows and badges; read-only (Close only); filter focus, narrowing, no match, and filtering sends nothing; the failed, cancelled, exited and Markdown states; Escape, focus back to the composer, and close.
- **Also run:** `npm run check` (0 errors) and `npm run build`.
- **Browser check:** with the mock backend (vite and Playwright), opening a chat, typing `/mc` and pressing Enter twice showed the loading state, then the grouped list. Typing filtered it, and Escape closed it with nothing added to the transcript. Dark mode was checked too.
- **Not checked:** a real Claude or Codex `/mcp` reply; the Wails desktop build; a screen reader.

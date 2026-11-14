# CPC-05 — Inventory UI and plugin command selection

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** CPC-03, CPC-04
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Support read-only `/mcp` and `/plugin` inspection in the composer and selection
of discovered plugin commands with descriptions and argument hints. Keep the
first release limited to this scope, not all terminal built-ins. Use canonical
adapter command names, with unambiguous short-name lookup and explicit choices
for collisions. Decide inspection/result presentation using CPC-02's contract.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- Small pure command/inventory helpers and chat components
- `frontend/src/lib/stores/chats.svelte.ts`, frontend tests

## Acceptance criteria and tests

- Listings show loading, empty, unsupported and failure states truthfully;
  pluginless-command inventory does not look empty merely because no commands
  exist. No mutation actions are presented.
- Typing/choosing a command executes nothing until submitted; insertion uses
  canonical names and keeps arguments. Ambiguity never silently picks a plugin.
- Keyboard, IME composition, Escape, focus and queue behavior remain correct;
  inventory requests never silently run as model instructions.
- Add pure-helper and component/store regression tests; run frontend tests,
  check and build. Desktop interaction verification belongs to CPC-08.

# CPC-10 — `/plugin` inventory panel

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CPC-04, CPC-05
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Split from CPC-05. Offer `/plugin` in the composer's command list for every
chat, because no adapter advertises it. Submitting exactly `/plugin` does not
send a prompt. It calls `chatPlugins` (CPC-04) and shows the result in a
transient panel by the composer, never as a transcript message. Rows show
name, marketplace, version, scope, "installed" and "enabled". They show
"loaded" only when the chat's command catalog has a command with that
plugin's prefix (see the contract's membership rule).

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`, or a small panel
  component beside it
- `frontend/src/lib/chat.ts` (pure helper), `frontend/src/lib/stores/chats.svelte.ts`
- frontend tests

## Acceptance criteria and tests

- Loading (Codex can take over a minute), empty, unsupported (OpenCode) and
  failure states are shown truthfully. A plugin with no commands is still
  listed.
- There are no install, enable or disable actions, and `/plugin` never
  reaches the agent as a prompt.
- A close or chat switch drops a result in flight. Closing the panel or the
  chat leaves nothing behind.
- Add pure-helper and component tests; run frontend tests, check and build.

## Evidence

Done:

- `chat.ts`:
  - `PLUGIN_COMMAND` is the app's `/plugin` entry. The composer appends it
    to the command list only when the agent advertises no `plugin` of its
    own.
  - `isPluginListing(text, commands)` is true when the text is exactly
    `/plugin` and the agent has no `plugin` command.
  - `pluginRows(plugins, commands)` marks a plugin loaded only when the
    catalog has a `<plugin>:…` or `$<plugin>:…` command. `mcp` is never a
    plugin.
- `ChatComposer.svelte`: sending exactly `/plugin` with no files clears the
  text and opens the panel. No prompt is sent, and this works in any phase
  except starting and ended. Each `/plugin` lists afresh. The panel shows
  only for the chat that asked, and hides when that chat ends.
- `PluginInventory.svelte` calls `api.chatPlugins` on mount. It shows the
  loading ("can take a minute"), unsupported, empty, failure and listed
  states, with `aria-live` on the status. Each row shows name, @marketplace,
  version, scope, enabled or disabled, and loaded. The only button is Close.
  A reply that arrives after the panel closed, or a stale reply, is dropped.

Tests run:

- `test/chat-ui.test.mjs` tests `isPluginListing`, `PLUGIN_COMMAND` in
  `commandChoices`, and `pluginRows`. The cases cover a Claude prefix, a
  Codex `$` prefix, a plugin without commands, the `mcp:` exclusion, and a
  plugin name that is a prefix of another (`pony` against
  `ponytail:ponytail`).
- `test/chat-end-menu.test.mjs` tests the composer and panel in jsdom:
  - `/plu` offers `/plugin`, and Enter inserts it;
  - the next Enter calls `chatPlugins('c1')` and sends no prompt;
  - the loading, listed, unsupported, empty and error states;
  - a commandless plugin is listed, and only a plugin with catalog
    commands is shown as loaded;
  - Close is the only button;
  - a reply after Close shows nothing;
  - an agent with its own `plugin` gets `/plugin` as a prompt. This case
    found a duplicate-key bug in the list, which is fixed.
- `npm run check`: 0 errors. `npm test`: 1480 pass, 0 fail.
  `npm run build`: built.

Not checked:

- Not tried in the desktop app, with a real CLI, or on a remote host
  (CPC-08).
- Claude's `projectEnabled` is not shown; only `enabled` is.


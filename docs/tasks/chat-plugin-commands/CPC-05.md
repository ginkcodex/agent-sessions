# CPC-05 — Command picker in the composer

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** CPC-03, CPC-04
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

List the chat's advertised commands (CPC-03) while the composer's text is one
`/` word, or `$` word for Codex skills, with descriptions and argument hints.
`/mcp` is the adapter's own command (see the
[contract](../../plan/chat-plugin-commands-contract.md)), so the picker offers
it bare and sends it as an ordinary prompt. Use canonical adapter command
names, list every match of a short name, and never pick one by itself. The
app-side `/plugin` listing panel was split into [CPC-10](CPC-10.md), so this
task stays within 4 h.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/src/lib/chat.ts` (pure helper), mock chat, frontend tests

## Acceptance criteria and tests

- Typing or choosing a command runs nothing until it is sent. The inserted
  text is the canonical name, with a space when the command takes arguments.
- A short name that two plugins share lists both; the user chooses.
- Keyboard, IME composition, Escape, focus and the queue keep working.
- Add pure-helper and component regression tests; run frontend tests, check
  and build. Desktop interaction checks belong to CPC-08.

## Evidence

Done:

- `commandChoices(commands, text)` in `frontend/src/lib/chat.ts`. It returns
  matches only while the text is one `/` or `$` word, matching by the full
  advertised name or by the part after the last colon. All matches are listed
  in the advertised order, with an exact name first. `/mcp` is inserted bare
  and its hint is hidden, because its arguments reconnect or turn off
  servers. Other commands keep their hint and description.
- `ChatComposer.svelte` shows the list above the card as an ARIA listbox.
  The text box keeps focus and points at the highlighted option with
  `aria-activedescendant`.
  - Arrow keys move through the list and wrap around.
  - Enter or Tab inserts the highlighted command. Enter sends only when the
    text already is that command.
  - Escape closes the list for that text.
  - A click inserts too.
  - Keys are ignored while an IME composes.
  - No list shows while the chat starts or after it ended.
  - `send()` and the queue are unchanged.
- The mock chat now advertises a catalog: Claude's `/mcp`, `ponytail:ponytail`
  and two `understand-anything` commands; Codex's `/mcp` and `$sites:sites`.

Tests run:

- `test/chat-ui.test.mjs` tests the helper: full and short names, a shared
  short name, an exact name first, case, `$`, text that is not one command
  word, the bare `/mcp`, and an empty catalog.
- `test/chat-end-menu.test.mjs` tests the composer in jsdom:
  - an ambiguous `/understand` lists both commands;
  - arrow keys wrap around, and `aria-activedescendant` follows the
    highlighted option;
  - Enter inserts and sends nothing, then the next Enter sends;
  - Tab completes `/pony` to `/ponytail:ponytail `;
  - a fully typed `/mcp` is sent by Enter, and its hint is not shown;
  - Escape closes the list;
  - Enter does nothing while an IME composes;
  - a click inserts.
- `npm run check`: 0 errors. `npm test`: 1478 pass, 0 fail.
  `npm run build`: built.

Not checked:

- The picker was not tried in the desktop app or with a real adapter catalog
  (CPC-08).
- Plugin labels, such as "loaded" next to commands, depend on the `/plugin`
  inventory (CPC-10).

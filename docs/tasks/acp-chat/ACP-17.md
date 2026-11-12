# ACP-17 — No spellcheck or autofill in the composer

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** ACP-16
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Prompts are full of code, paths and commands. The composer's text box no
longer spellchecks, autofills, autocorrects or capitalizes:
`spellcheck="false"`, `autocomplete="off"`, `autocorrect="off"` and
`autocapitalize="off"`.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria and tests

- The rendered textarea carries all four attributes.
- `npm test`, `npm run check`, `npm run build`.

## Evidence

- Done: the four attributes are on the composer's textarea. Svelte's types
  list `autocorrect` for inputs only, so it is passed through a spread;
  WebKit (macOS, Linux) reads it on a textarea, and other engines ignore it.
- Tests:
  - `chat-ui.test.mjs`: the composer test checks all four attributes on the
    textarea; 16/16 pass.
  - `npm test`: 1303 tests, 1302 pass, 0 fail, 1 skipped (as before).
  - `npm run check`: 0 errors, 0 warnings. `npm run build`: OK.
- Not checked: typing in the app window on macOS, Linux or Windows (no
  display here).

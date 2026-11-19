# AGY-17 — Effort picker says "Extra", agy says `xhigh`

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-06](AGY-06.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). The chat's effort picker
for Antigravity offers Low, Medium, High and Extra. The `agy` TUI's effort
slider says `low`, `medium`, `high` and `xhigh`, and `agy --effort` takes
`low|medium|high|xhigh|max`. "Extra" reads as a different level.

## Cause (to confirm)

The picker shows each choice's ACP `name`, falling back to the value
(`selectValues` in `frontend/src/lib/chatOptions.ts`: `o.name || o.value`).
No app code spells "Extra", so agy's ACP server most likely names
`thought_level` `xhigh` "Extra". The AGY-01 spike recorded only the values
(`low`, `medium`, `high`, `xhigh`), not the names. Confirm with a
`session/new` capture.

Seen in the same screenshots, not a bug on its own: the chat said High and
the terminal's TUI said medium. They were different sessions, and agy's ACP
default is `high` (spike).

Also to check: `max` is an `agy --effort` value, but ACP does not offer it.

## Scope

- Capture the `thought_level` option's names from `agy acp`.
- If agy names `xhigh` "Extra", show agy's own spelling for Antigravity:
  the value (`xhigh`, or "XHigh" to match the other choices' capitals).
  Keep the agent's names for every other agent and option.
- Record the names in the spike's config option table.

## Likely files

- `frontend/src/lib/chatOptions.ts` (`pickers` or `selectValues`).
- `frontend/test/` chat options tests.
- `docs/plan/antigravity-cli-spike.md`.

## Acceptance criteria

- The Antigravity effort picker names the level the CLI calls `xhigh` the
  way the CLI does. Selecting it still sends `xhigh`.
- The other agents' pickers are unchanged.

## Tests

- Frontend: the Antigravity `thought_level` choices map to the CLI's
  spelling. The other agents keep the names they send.
- Manual: the picker in a live Antigravity chat on macOS.

## Evidence

Confirmed with a throwaway `agy acp` session (agy 1.5.12, Linux, workspace
`/tmp/agy-probe-ws`, session `c2b87113-…`):

- `session/new`'s `thought_level` choices are `low`/Low, `medium`/Medium,
  `high`/High and `xhigh`/**Extra**. No `max`: `agy --effort max` exists,
  but ACP does not offer it.
- agy's own model list in the same reply spells the level XHigh:
  `gemini-4-neon-xhigh` is "Gemini 4 Neon (XHigh)".

Done:

- `chatOptions.ts`: a `CHOICE_NAMES` map renames choices an agent's ACP
  spells differently from its CLI. The only entry is Antigravity's
  `thought_level` `xhigh` → "XHigh". The value sent is still `xhigh`, and
  remembered choices are stored by value, so they are unchanged.
- The spike's config option table lists the names.

Tests:

- `frontend/test/chat-options.test.mjs`:
  - Antigravity's `thought_level` choices read Low, High, XHigh, and the
    current value stays `xhigh`;
  - the same option for another agent keeps "Extra".
- `chat-options` tests: 6 pass, 0 fail. `npm run check`: 0 errors,
  0 warnings.

Not checked:

- The picker in the desktop app on macOS.

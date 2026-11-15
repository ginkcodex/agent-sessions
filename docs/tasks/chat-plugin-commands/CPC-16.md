# CPC-16 — Remove the Codex singular inventory alias

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** CPC-15
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Remove `/plugin` as a Codex inventory alias; only `/plugins` opens its installed inventory. Preserve other agents' `/plugin` behavior and the existing Codex picker, which already offers only plural. No backend, configuration, fixture or export changes are needed; session data is unaffected.

## Likely files

- `frontend/src/lib/chat.ts`
- `frontend/test/chat-ui.test.mjs`, `frontend/test/chat-end-menu.test.mjs`
- Provider contract, plan and progress indexes

## Acceptance criteria

- Codex `/plugins` opens inventory; `/plugin` is not intercepted, whether advertised or not.
- Codex's picker still offers only `/plugins`; other providers retain their current behavior.
- Unit and composer tests cover the removed alias and ordinary input routing.

## Tests

Run frontend tests and Svelte check; record actual results and any checks not performed.

## Evidence

- Codex now returns an inventory match only for bare `/plugins`; singular `/plugin` is ordinary input, not a supported inventory alias. Other providers and the already-plural-only picker are unchanged.
- Unit coverage rejects singular Codex input both with and without advertised commands. Composer coverage verifies singular input never calls inventory or opens the modal, while plural still lists all fixture entries.
- `npm --prefix frontend run check`: passed, 642 files, zero errors or warnings.
- `npm --prefix frontend test`: passed, 1,493 passed, zero failed, one skipped (1,494 total).
- `git diff --check`: passed.
- Not run: production build, native app/Windows checks, Go tests or lint; this change touches only frontend command matching and tests. No new runtime risk beyond `/plugin` no longer opening Codex inventory.

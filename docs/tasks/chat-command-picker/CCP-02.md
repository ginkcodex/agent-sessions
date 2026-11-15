# CCP-02 — Reopen suggestions after editing dismissed input

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** CCP-01
- **Plan:** [chat-command-picker.md](../../plan/chat-command-picker.md)

## Scope

Reset command-picker dismissal on user input. Escape must hide the current list, not permanently suppress a previously entered slash word. Preserve completion insertion, keyboard selection, IME handling and message sending.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/test/chat-end-menu.test.mjs`
- Plan/task progress indexes

## Acceptance criteria

- Type `/`, Escape, clear, type `/`: suggestions reopen with the first option active.
- Repeated dismissal and re-entry work for slash commands and Codex `$` skills.
- No prompt is sent by dismissal/editing; existing completion behavior remains passing.

## Tests

Add interaction regression coverage; reproduce failure before the fix, then run frontend tests, Svelte check and production build.

## Evidence

- Root cause: Escape remembered the dismissed text indefinitely; clearing the input did not reset that state, so retyping the same prefix stayed hidden.
- Reset `dismissed` to null alongside `active` in the textarea input handler. Programmatic completion still dismisses its suggestions until an actual user edit.
- Added an interaction regression for repeated Escape → clear → retype cycles with `/` and Codex `$`, first-option selection, ARIA active-descendant restoration and no prompt sends.
- Before the fix, the targeted test failed at `/ reopens after clearing`; after the fix the full frontend suite passed: 1,494 passed, zero failed, one skipped (1,495 total).
- `npm run check`: passed, 642 files, zero errors or warnings.
- `npm run build`: passed; Vite warned about chunks exceeding 500 kB.
- Not checked: native Windows runtime or visual QA. No backend changes; Go tests and lint were not run.

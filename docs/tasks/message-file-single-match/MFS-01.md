# MFS-01 — Open the only found file directly

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [message-file-single-match.md](../../plan/message-file-single-match.md)

## Scope

In the file recovery shared by session messages and the terminal, return the
single candidate instead of asking, unless the listing is truncated. Several
candidates keep the chooser toast.

## Likely files

- `frontend/src/lib/messageFileOpen.ts`
- `frontend/test/message-file-open.test.mjs`

## Acceptance criteria

- A name or partial path with one match in the project opens that file (and
  its line) without a toast.
- Several matches, or one match in a truncated listing, show the chooser.
- The found path is resolved again before opening; a failure is reported.
- Stale requests still open nothing.

## Tests

Run the recovery tests, the frontend suite, Svelte check and production build.

## Evidence

Done:

- `messageFileRecovery` returns `candidates[0]` when there is exactly one and
  `listing.truncated` is not set. The opener re-resolves it, opens it kept and
  reveals the line, as after a toast choice.
- Tests: the old "single matches still require selection" test is replaced by
  one that opens directly, plus truncated single match (toast) and no match
  (error). A new test checks that the found path is resolved again and a
  failure is reported. Two partial-path tests now use two matches for the
  chooser and one match for direct open.

Verified (from `frontend/`):

- `node --loader ./test/resolver.mjs --test test/message-file-open.test.mjs`:
  13 pass, 0 fail.
- `npm test`: 1382 tests, 1381 pass, 0 fail, 1 skipped (already skipped).
- `npm run check`: 0 errors, 0 warnings. `npm run build`: succeeds.

Not checked: clicking a bare file name in the running desktop app.

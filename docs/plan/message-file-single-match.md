# Open a single found file directly

## Goal

When a session message (or terminal) names a file only by its name or by a
partial path such as `stores/terminals.svelte.ts`, and that path does not
exist under the session's working directory, the app already searches the
project listing and offers the matches in a toast. With one match the user
still had to click it. Open that one file straight away; keep the toast for
several matches.

## Design

- `messageFileRecovery` (`frontend/src/lib/messageFileOpen.ts`) returns the
  only candidate instead of showing the chooser. The opener then resolves and
  authorizes that path again, opens it, and reveals any `:line`, exactly as
  after a choice in the toast.
- Several matches: the persistent "Choose a file for …" toast, as before.
- No match: the "Could not find … file" error, as before.

## Decisions

- A truncated listing may hide further matches, so a single match from a
  truncated listing still shows the toast (which says the listing is
  truncated) rather than open a possibly wrong file.
- The terminal shares this recovery, so a bare name clicked in the terminal
  behaves the same way. Splitting the two would make the same path act
  differently in two places.
- Discovery itself is unchanged: it still runs only after the direct path
  fails and a probe proves a path prefix absent.

## Risks

- Opening without a click must not bypass authorization: the selected path
  still goes through `resolveTerminalFile`, and a failure is reported.
- Stale clicks (session, host or epoch changes) must still not open anything;
  the existing `current()` checks run before the path is returned.

## Release criteria

- Tests: one match opens and reveals without a toast; a truncated single match
  and several matches show the chooser; a failing re-resolution is reported.
- Frontend tests, type check and production build pass; record checks not run.

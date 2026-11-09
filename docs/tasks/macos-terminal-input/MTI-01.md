# MTI-01 — Reproduce and fix macOS replacement input

- **Status:** ➖ Superseded by [MTI-03](MTI-03.md) — the failure was the remote shell's POSIX locale, not frontend input routing. No frontend change was needed.
- **Estimate:** 3 h
- **Depends on:** None
- **Plan:** [macos-terminal-input.md](../../plan/macos-terminal-input.md)

## Scope

Trace native Unicode keyboard injection into macOS WebKit and compare with xterm's routing and the existing printable-code assumption. Add the smallest evidence-backed terminal input fix and event-routing regression tests.

## Likely files

- `frontend/src/lib/terminalTextInput.ts`
- `frontend/src/lib/components/terminal/TerminalView.svelte`
- `frontend/test/terminal-text-input.test.mjs`

## Acceptance criteria and tests

- Record native event evidence or clearly state reproduction limitations.
- Replacement text is sent whole once; normal controls, shortcuts and native composition remain unchanged.
- Exercise xterm routing with replacement event data, including single and decomposed Vietnamese text where relevant.
- Run focused/full frontend tests, frontend check/build, `make test`, and diff whitespace checks; record actual results.

## Evidence

- [Native investigation record](../../plan/macos-terminal-input-qa.md): current replacement routing works in the native probe and isolated real desktop app; user's exact failure remains unreproduced.
- Focused terminal/input tests: 58/58 passed. Desktop development build succeeded with linker warnings recorded in the QA note.
- No production code changed, no new regression claiming the issue fixed, and no full frontend/Go suite run.
- The user's later diagnostics placed the failure in a remote plain shell running a POSIX/ASCII locale, not in the frontend input routing. The fix is tracked in [MTI-03](MTI-03.md). No frontend change is planned unless that fix does not resolve the issue.

# CRF-05 — Repair notice for end users

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CRF-04
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

Detect affected sessions, explain the import issue and offer Repair session in the header. Scope asynchronous results to the session and host; show successful refresh and actionable errors.

## Acceptance criteria and tests

Automated tests establish the corresponding in-app recovery contract in the
plan. No real-user files are modified. Record commands, results and remaining
manual UI/native-host limits in Evidence.

## Evidence

- Affected-session header notice explains the old import and backup behavior
  and offers Repair session. Exact agent/UUID/basename detection excludes normal
  sessions; Windows and Unix paths are recognized. No automatic mutation.
- Progress, error/retry, completed status, update-required remote capability
  handling, terminal/chat blockers and success toast with backup location.
  Results are scoped to captured host epoch/session; duplicate writes blocked.
- Store/API/render tests cover these states, old/mock binding errors, refresh
  failure after successful repair and late responses after navigation/host switch.
- npm test: 1362 tests, 1361 pass, 1 skipped. Svelte check: 0 errors, 0 warnings.
  Production build passes (existing large-chunk advisory).
- Headless Chrome local mock: actual notice in light/dark at viewport widths
  1000/390 (card widths 798/340), no horizontal overflow. Repair click invoked
  one mocked mutation, displayed completed state/toast, disabled another repair,
  and the notice disappeared on canonical metadata refresh; no runtime errors.
  Narrow light screenshot visually inspected. Screenshots /tmp/crf-repair-notice-*.png.
  Used a temporary compiled Svelte parent for reactive props, then removed it
  and stopped the preview server/browser.
- Updated end-user instructions: select session, Repair session, then resume;
  Python utility remains optional support recovery.
- Not verified in an installed desktop build or with the user's actual Mac
  data. Desktop and host binaries must include the new feature.

# TTR-03 — Search decoded output values

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** TTR-01
- **Plan:** [transcript-tool-readability.md](../../plan/transcript-tool-readability.md)

## Scope
Include decoded JSON output values in session search, beside the raw output, so visible Windows paths and quoted strings in decoded envelopes remain searchable. Keep the raw corpus for compatibility. Align mock search and add regressions.

## Likely files
`internal/engine/session_search.go`, session search tests, frontend mock search.

## Acceptance criteria and tests
A query matching the displayed decoded path or quote finds the tool part. Existing raw queries still work. Run focused Go and frontend tests.

## Evidence
- Session search retains raw output and adds decoded JSON scalar values with the existing bounded `JSONPlainValues` helper. Mock search includes the displayed decoded output.
- Added real engine SearchSession and mock regressions for Windows paths, raw escaped paths and quoted phrases.
- `go test -race ./internal/engine -run SessionSearch -count=1`: passed.
- Full `make test` (`go test -race ./...`): passed on the longer-timeout run. The first run was stopped by its 120-second tool timeout, not a test failure.
- Focused frontend session-find/output tests: 17 passed. Full frontend suite and check/build passed after the change.
- Review also caught possible loss of sibling metadata and integer rounding: unknown/mixed JSON now stays verbatim, with dedicated preservation tests.


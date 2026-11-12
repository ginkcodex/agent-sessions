# CRF-06 — Repair across provider-root spellings

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CRF-03
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

internal/manage repair path handling. The scan reports session paths under
the symlink-resolved provider root (the Codex provider normalizes its root)
while Codex's thread/session indexes store the root as configured, so the
same file is known in two spellings when the root sits behind a symlink or a
Windows 8.3 short name (macOS temp dirs under /var, CI temp dirs under
C:\Users\RUNNER~1).

## Acceptance criteria and tests

Repair accepts a catalog path in either spelling, works in the configured
form so index reads and writes match what Codex stores, and reports result
paths in the caller's spelling. A regression test creates the fixture root
behind a symlink and asserts the bridging on every OS.

## Evidence

- CI run 43138587214 (ed2aa3b) failed `TestRepairCodexRefreshesCatalogPath`
  on windows-test-build and macos-test with `path is outside the provider
  root`; Linux passed because /tmp is not aliased. Reproduced on Linux by
  running the test with TMPDIR behind a symlink.
- Root cause: `RepairCodexSession` compared the catalog spelling (resolved
  root) against the configured root in `ensureWithinRoot`, `derivePaths`,
  the SQLite `threads` updates and the session-index rewrite, and returned
  result paths in the manager's spelling.
- Fix: `codexManager.repairLocalPath` re-roots the catalog path onto the
  manager root (accepting either spelling) and remembers the caller's root
  spelling; the repair runs entirely in the configured form and reports
  `Path`/`BackupPath` in the caller's spelling. No safety check was
  weakened: symlink and protected-path refusals and the resolved-root
  containment check are unchanged.
- Added `TestRepairCodexSessionBridgesRootSpellings` (fixture root behind a
  symlink, catalog path in the resolved spelling). It fails with the exact
  CI error on the pre-fix code and passes after.
- `go vet ./...` clean. Full `go test -count=1 ./...` passes. The failing
  `TestRepairCodexRefreshesCatalogPath` passes both plain and with TMPDIR
  behind a symlink. Frontend not touched. No real Codex data used.

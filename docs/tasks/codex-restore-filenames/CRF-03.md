# CRF-03 — Native Go repair

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** CRF-02
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

internal/manage native repair. Cover history/index backups, exact targeted repairs, writer/ownership guards, partial recovery and unchanged bytes.

## Acceptance criteria and tests

Automated tests establish the corresponding in-app recovery contract in the
plan. No real-user files are modified. Record commands, results and remaining
manual UI/native-host limits in Evidence.

## Evidence

- Added a native Go targeted repair: catalog-derived paths only, exact legacy
  basename and metadata ID verification, process/chat/native writer-lock guards,
  symlink/ambiguous-source/index-reference validation and no-overwrite checks.
- Backs up the original history and JSON index plus each native SQLite index
  using VACUUM INTO (including WAL data) in a private folder. Canonical hardlink
  precedes exact path updates; the legacy name is removed only after all index
  commits. Partial failure keeps both valid names and supports retry.
- Tests verify history bytes, numeric JSON metadata, backup index paths, WAL
  contents, active chats/writers/terminals, conflicting files, metadata mismatch,
  symlinked indexes, aliased index paths and injected partial DB failure/retry.
- Native test with CODEX_ROLLOUT_TEST_BIN set to installed Codex 0.167.1 passed
  on Linux: synthetic ordinal-0 paginated record in a disposable home reproduced
  the exact filename error; the actual Go repair then allowed thread/resume
  by UUID and retained byte-identical history. No prompt or real user data.
- Full manage suite and targeted repair race checks pass. Darwin/arm64 manage
  test binary cross-compiles with CGO_ENABLED=0; not executed on a Mac.

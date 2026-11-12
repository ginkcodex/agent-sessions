# CRF-02 — Repair already-restored rollouts

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CRF-01
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

scripts/repair-codex-rollouts.py, its tests and repair documentation. Cover index updates, backups, no-overwrite/id safety, held locks and failure recovery.

## Acceptance criteria and tests

Meet the corresponding plan release criteria using synthetic data only. Record
what ran, results and unverified real-client behavior in Evidence.

## Evidence

- Added scripts/repair-codex-rollouts.py, defaulting to preview, with targeted
  --session selection, explicit --apply, private backups, consistent SQLite
  backups including WAL, exact native path updates, unchanged history bytes,
  same-file link recovery, source/ID/path guards and POSIX writer-lock refusal.
- Added [Mac/Linux repair instructions](../../codex-rollout-repair.md). The
  tool was not run against the user's data; no real sessions were modified.
- 15 tests pass in scripts/test_repair_codex_rollouts.py, including preview,
  every state index, JSON references, backups, WAL contents, idempotence,
  aliases/symlinks/conflicts, ID mismatches, active locks, changed sources,
  partial commit failure and successful rerun.
- The opt-in native smoke test ran with CODEX_ROLLOUT_TEST_BIN pointing to
  installed Codex 0.167.1 on Linux, with a disposable Codex home, an ordinal-0
  synthetic paginated session_meta record and no prompt/inference request.
  Native thread/resume.path (experimentalApi enabled) reproduced the exact
  noncanonical filename error; after applying the actual repair utility,
  thread/resume by ID succeeded and the history bytes were identical.
- Command: `CODEX_ROLLOUT_TEST_BIN=/path/to/codex PYTHONDONTWRITEBYTECODE=1
  python3 -m unittest discover -s scripts -p test_repair_codex_rollouts.py -v`.
- `git diff --check` clean. Actual repair/resume on the reported Mac is still
  to be run there. Windows apply and external custom SQLite homes are not
  supported by this standalone utility.

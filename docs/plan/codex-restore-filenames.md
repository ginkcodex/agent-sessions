# Canonical filenames for restored Codex rollouts

## Goal

Restore/import must produce Codex-resumable rollout paths. Repair files created
by the previous rollout-restored-<uuid>.jsonl naming bug without changing history.

## Design and decisions

New restores preserve a canonical native filename timestamp and use the final
session ID (including copy remapping). Older exported restored files derive a
stable timestamp from session_meta (payload timestamp, then record timestamp),
then the bundle creation time if needed. Never use the current clock for the
filename: preview and execution must agree. Keep the existing restore-day
destination directory and collision/path guards.

Provide a standalone Python standard-library repair utility for existing Mac
and Linux installations, with preview by default and --apply for an offline
repair. Limit candidates to the exact legacy basename pattern, verify metadata
IDs, preserve contents, reject symlinks and conflicting targets. Back up
rollouts, session_index.jsonl and SQLite indexes before applying. Create the
canonical hardlinks before updating indexes, and remove legacy names only
after all index updates commit. A partial failure retains both paths so no
committed index points to a missing file. Reruns tolerate the same-file links.
Repair threads.rollout_path in every state_<number>.sqlite with that column,
and exact path references in session_index.jsonl. Do not run against real
user data during implementation. Codex clients and Agent Sessions must be
closed during --apply; refuse a held existing POSIX writer lock.

## Risks

Canonical timestamp format, changing copy IDs, stale previews, old exports,
SQLite WAL state, partial filesystem/database failure, collisions and symlinks.
Repair uses POSIX locks; Windows repair is outside this utility's scope.

## Release criteria

Go restore tests for canonical names, stable previews and copies; Python tests
for preview/no mutation, contents/index updates/backups, idempotence and unsafe
inputs/failure handling. Document exact repair commands for the reported Mac
root. Actual Mac resume remains a manual check after the repair is applied.

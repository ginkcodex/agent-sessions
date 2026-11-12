# Repair Codex sessions imported with noncanonical filenames

An affected session can be read in Agent Sessions but Codex refuses to resume
it with `does not have a canonical rollout filename`. Earlier app versions
restored it as `rollout-restored-<uuid>.jsonl`. Codex's paginated history reader
requires `rollout-YYYY-MM-DDTHH-MM-SS-<uuid>.jsonl`.

The restore fix prevents new occurrences. Existing files need the repair below;
a client rebuild alone does not rename them.

## Preview on the affected Mac or Linux machine

Use Python 3.9+ and the script from this updated checkout. Run from the
Agent Sessions repository directory. Preview does not apply a repair or make
backups. To preview just the reported session:

```sh
python3 scripts/repair-codex-rollouts.py \
  --codex-home /Users/gink/.codex \
  --session 01a02317-c499-77e0-aea1-983624e8aadb
```

Omit `--session` to check all matching legacy names in `sessions/` and
`archived_sessions/`. `--codex-home` defaults to `$CODEX_HOME`, then `~/.codex`.

## Apply offline

Close Agent Sessions and Codex clients (CLI, app and IDE sessions) using this
Codex home before applying. The tool also refuses an existing writer lock
held for an affected session, but it is not a substitute for closing clients:
an open process can cache the old path or reopen a session during repair.
A background daemon may retain a writer lock briefly after clients close;
wait and retry if the tool reports it is still held.

```sh
python3 scripts/repair-codex-rollouts.py \
  --codex-home /Users/gink/.codex \
  --session 01a02317-c499-77e0-aea1-983624e8aadb \
  --apply
```

The tool verifies the rollout's metadata ID and timestamp, backs up its bytes,
backs up every `state_<number>.sqlite` index containing `threads.rollout_path`
using SQLite's backup API (including committed WAL data), and saves
`session_index.jsonl` when present. Backups go in a private
`rollout-repair-backup-*` folder under the selected Codex home. Keep this folder
until you have verified the session resumes.

It creates canonical filenames without rewriting conversation contents,
updates exact `threads.rollout_path` and native JSON-index path references,
and removes legacy names only after the index updates succeed. It never
replaces an unrelated existing target. Symlinks and mismatched session IDs
are refused, as are index paths that conflict with the selected source. The tool covers indexes inside the selected Codex home; custom
external SQLite index locations are not repaired by this command.

If it stops after making a canonical link, both names may remain. Keep the
backups and files, resolve the reported problem, and rerun the same command:
it accepts an existing target only when it is the same file, and finishes
remaining updates. No committed index is intentionally left pointing to a
path removed by the repair.

After success, reopen Agent Sessions and refresh it, then retry chat or:

```sh
codex resume 01a02317-c499-77e0-aea1-983624e8aadb
```

The repair utility's apply mode requires POSIX locks (macOS/Linux). Future
restore naming is fixed on all platforms.

## Verification for maintainers

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts \
  -p 'test_repair_codex_rollouts.py' -v
```

To additionally run the real Codex smoke test, set `CODEX_ROLLOUT_TEST_BIN`
to an absolute Codex executable path. This starts app-server against a temporary
Codex home, reproduces the filename error with a synthetic paginated rollout,
applies the repair, and resumes by session ID. It does not send a prompt or
use the real user's session history or credentials.

See the [plan](plan/codex-restore-filenames.md) and
[tasks](tasks/codex-restore-filenames/README.md) for test evidence and limits.

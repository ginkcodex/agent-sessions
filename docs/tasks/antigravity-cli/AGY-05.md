# AGY-05 — Provider registration, file watcher and internal terminal commands

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** [AGY-04](AGY-04.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Register the Antigravity provider in the central registry, implement its
terminal commands (`agy`, `agy --conversation <id>`), wire filesystem watching
so live edits trigger immediate rescans, and update backend display labels.

## Likely files

- `internal/provider/antigravity/commands.go` — `NewSessionCommand(dir)` →
  `Command{Argv: []string{"agy"}, Dir: dir}`; `ResumeCommand(m)` →
  `Command{Argv: []string{"agy", "--conversation", m.Ref.ID}, Dir: m.CWD}`.
- `internal/provider/all/all.go` — register `AgentAntigravity` (`"Antigravity"`).
- `internal/watch/watch.go` — add `model.AgentAntigravity` cases to
  `rootsFor()` (watch root for `conversation_summaries.db*` and `<root>/brain`
  recursively), `relevantEvent()` (accept `conversation_summaries.db*` and
  `transcript*.jsonl`, ignore `log/`, `cache/`, `crashes/`, `presence/`,
  `conversations/*.db*`, `logs/chunks/`, `brain/<id>/scratch/` and
  `brain/<id>/.system_generated/steps/`). `pollInterval()` keeps the default
  file poll: the transcripts are JSONL, and only the OpenCode poll is special.
- `internal/group/group.go` — add `"Antigravity"` label in `agentNode`.
- `internal/app/panel.go` — add `"Antigravity"` label in `agentLabel`.
- `internal/index/search.go` — add `AgentAntigravity` to the default list in
  `intersectAgents`.

## Acceptance criteria

- `all.Supported()` and `all.Providers()` include `antigravity`; toggling it in
  `[agents] enabled` works identically to existing agents.
- `agent-sessions-cli detect`, `scan`, and `show` list and display Antigravity
  sessions.
- Writing to `conversation_summaries.db-wal` or appending to
  `brain/<id>/.system_generated/logs/transcript_full.jsonl` triggers a debounced
  `fsnotify` refresh for `AgentAntigravity`; writes to `log/`, `cache/`,
  `presence/`, or `conversations/*.db` do not trigger rescans.
- Internal terminal launch produces `agy` for new sessions and
  `agy --conversation <id>` for resume, both locally and over SSH/WSL scripts.
- A search with an `agent:antigravity` filter finds Antigravity sessions.

## Tests

- Unit tests in `internal/provider/all` verifying registration and canonical
  ordering.
- Unit tests in `internal/watch` verifying `rootsFor` and `relevantEvent`
  filtering for `AgentAntigravity` (accepting WAL/JSONL, ignoring noise files
  and `logs/chunks/*.jsonl`).
- Unit tests for `NewSessionCommand` and `ResumeCommand`.
- A unit test in `internal/index` for the `agent:antigravity` search filter.

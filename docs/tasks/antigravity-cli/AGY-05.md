# AGY-05 — Provider registration, file watcher and internal terminal commands

- **Status:** ✅ Done
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

## Evidence

Done:

- `commands.go`: `NewSessionCommand(dir)` is `agy` in `dir`;
  `ResumeCommand(m)` is `agy --conversation <id>` in `m.CWD`. The provider
  now satisfies `provider.Provider`, so the `commandless` test wrapper is
  gone.
- `provider/all`: `antigravity` ("Antigravity") is registered last, so
  `Supported()`, `IDs()`, `Canonical` and `Providers()` include it. A saved
  `[agents] enabled` list that leaves it out keeps it off; with no saved
  list it is on, as the other agents are.
- `watch.go`: `rootsFor` watches `brain/` recursively and the store root
  (the index's folder) flat. `relevantEvent` takes
  `conversation_summaries.db` and `-wal` as a full rescan, and
  `transcript*.jsonl` directly in a `logs/` folder as a path change.
  `logs/chunks/`, `steps/`, `scratch/`, `log/`, `cache/`, `presence/`,
  `conversations/*.db*` and the `-shm` file are ignored. The poll interval is
  the default file poll.
- Labels in `group.go` (`agentNode`) and `panel.go` (`agentLabel`);
  `AgentAntigravity` in the default `intersectAgents` list.
- Tests that counted three agents (`app`, `engine` and `rpc` agent settings)
  now count four.

Tests (all pass; `go test -race` on `watch`, `index`, `provider/...` and
`group`; `go test ./...` before the commit; `go vet ./...` and lint on the
touched packages clean):

- `TestProviders` (`provider/all`): four providers, ID order, `Canonical`.
- `TestAntigravityEvents` (`watch`): `relevantEvent` for the index, both
  transcript names and 11 noise paths; `rootsFor`.
- `TestAntigravityWatch` (`watch`): real fsnotify. Writes to `log/`, a lock
  file and a non-transcript file in `logs/` give no rescan. A `-wal` write
  gives a full rescan, and a transcript write gives a change naming its path.
  Run 3 times with `-race`.
- `TestCommands` (`antigravity`).
- `TestSearchAgentPrefixAntigravity` (`index`): `orbit agent:antigravity`
  returns only the Antigravity session, not the Codex one.

Live, read-only, with cache and config in `/tmp`: `agent-sessions-cli
detect` reports `antigravity true ~/.gemini/antigravity-cli`. `scan --agent
antigravity` lists the 5 top-level sessions with 0 parse errors (the
subagent is hidden, as for other agents). `show antigravity <id>` prints the
prompt, reasoning, the `run_command` and `read_file` calls and the reply.

Not checked: launching `agy` from the internal terminal and over SSH/WSL
(the commands go through the shared launch path; AGY-08 runs them), and
Windows and macOS watchers.

# AGY-24 — Handoff into Antigravity changes `agy`'s workspace to the handoffs directory

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-10](AGY-10.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Reported during desktop use (2026-11-20). After using "Continue in Another
Agent" → Antigravity CLI, `agy` opens and reads the handoff file, but its
workspace changes to `~/.local/share/agent-sessions/handoffs` instead of
staying in the session's project workspace.

## Cause

[AGY-10](AGY-10.md) added `--add-dir <handoff dir>` to Antigravity's handoff
command in `internal/handoff/delivery.go`, assuming it behaved like Claude
Code's `--add-dir`.

In `agy` (verified on `1.5.13`), `--add-dir` adds the directory to
`Active Workspaces` and sorts `workspace_uris` lexicographically, then uses the
first entry as both `Command Working Directory` and `workspace_uris[0]` in
`conversation_summaries.db`. Because `/home/<user>/.local/share/agent-sessions/handoffs`
sorts before `/home/<user>/Workspaces/...` (`.` < `W`), `--add-dir` hijacked
the session's workspace:

- Real handoff sessions in `conversation_summaries.db` (`d2baaa02`, `12bfd0f3`)
  recorded `workspace_uris = ["file:///home/haith/.local/share/agent-sessions/handoffs","file:///home/haith/Workspaces/ginkcode/tools/agent-sessions"]`.
- Reproducing with `cd /tmp/agy-probe-b && agy -p ... --add-dir /tmp/agy-probe-a`
  confirmed `Active Workspaces: [/tmp/agy-probe-a, /tmp/agy-probe-b]`,
  `Command Working Directory: /tmp/agy-probe-a`, and
  `workspace_uris = ["file:///tmp/agy-probe-a","file:///tmp/agy-probe-b"]`.
- `internal/provider/antigravity/discover.go` reads `workspace_uris[0]` as
  `CWD`, so `agent-sessions` also grouped and resumed the new session under the
  handoffs directory.

## Scope

- `internal/handoff/delivery.go`: stop appending `--add-dir` for
  `model.AgentAntigravity` in `LaunchCommand` (keep it for Claude Code only).
  Without `--add-dir`, `agy` keeps the project as its sole workspace and
  `Command Working Directory`, and in interactive mode (`agy -i`) prompts once
  for `read_file` permission on the handoff file (matching OpenCode's
  external-directory permission flow).
- `internal/handoff/handoff_test.go`: assert `LaunchCommand` for Antigravity
  with a prompt file emits `agy -i <pointer prompt>` without `--add-dir`.
- Considered and rejected: passing `--settings <file>` with
  `permissions.allow: ["read_file(<handoff dir>)"]`. While verified on
  `agy 1.5.13` to allow the read without altering `workspace_uris`, `--settings`
  is a recent flag not accepted by older `agy` builds, would require an extra
  managed file in `handoffs/` (inline JSON fails `launch.ValidateArgs` on
  Windows PowerShell 5.1 due to embedded `"`), and its merge vs. replace
  semantics against user `permissions.deny`/`ask` rules in `settings.json` are
  unverified.

## Acceptance criteria

- `handoff.LaunchCommand(model.AgentAntigravity, ..., promptFile, cwd)` runs
  `agy -i <pointer prompt>` in `cwd` with no `--add-dir`.
- `handoff.LaunchCommand(model.AgentClaude, ..., promptFile, cwd)` keeps
  `--add-dir <handoff dir>`.
- Starting `agy` without `--add-dir` keeps `workspace_uris` and
  `Command Working Directory` at the project directory.

## Evidence

- Verified on `agy 1.5.13` (2026-11-20):
  - With `--add-dir /tmp/agy-probe-a` from `/tmp/agy-probe-b`: `agy` reported
    `Command Working Directory: /tmp/agy-probe-a` and stored
    `["file:///tmp/agy-probe-a","file:///tmp/agy-probe-b"]`.
  - Without `--add-dir` from `/tmp/agy-probe-b` reading a non-workspace,
    non-`/tmp` file: `agy` stored `["file:///tmp/agy-probe-b"]` and reported
    that `read_file` requires interactive permission approval (which headless
    `-p` cannot prompt for).
- Tests and lint:
  - `go test -race ./internal/handoff/... ./internal/engine/... ./internal/app/... ./cmd/agent-sessions-cli/...`: all ok (`internal/handoff` 1.0s, `internal/engine` 225.5s, `internal/app` 8.2s, `cmd/agent-sessions-cli` 1.9s).
  - `go tool golangci-lint run ./internal/handoff/...`: 5 pre-existing findings in untouched `build.go`, 0 on changed files (`delivery.go`, `handoff_test.go`).
  - `go tool golangci-lint fmt --diff ./internal/handoff/...`: clean (no diff).
- Not checked: live interactive desktop handoff from the built app into `agy` (and remote host servers will reflect the change once rebuilt).

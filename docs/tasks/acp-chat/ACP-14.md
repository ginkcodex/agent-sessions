# ACP-14 — QA with real agents on every host

- **Status:** 🟡 Partial: the automated checks ran; the real-agent checks on Linux, WSL, SSH and the Windows build did not. See Evidence.
- **Estimate:** 4 h
- **Depends on:** ACP-11, ACP-13, ACP-15
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Run the release criteria by hand with Claude Code, Codex and OpenCode:

- on Linux;
- on a WSL distribution;
- on an SSH host;
- in a Windows test build made as CLAUDE.md describes.

Record each check, its result, the adapter versions and anything not run in
`docs/plan/acp-chat-qa.md`.

## Likely files

- `docs/plan/acp-chat-qa.md`

## Acceptance criteria

Only checks that actually ran are marked passed. Failures become new tasks.

## Tests

The manual QA above, plus `make lint test`, `npm test`, `npm run check` and
`npm run build` on the release commit.

## Evidence

Recorded in [acp-chat-qa.md](../../plan/acp-chat-qa.md), run of 2026-11-11.

What ran, on Linux:

- `make lint`: fails. Two findings came from this feature
  (`--new-from-rev d68793f`): a `goimports` fix in
  `internal/engine/chat_launch.go` and an unused parameter in
  `internal/rpc/chat_test.go`. Both are fixed, and `--new-from-rev d68793f`
  now reports 0 issues. The other 488 findings (counted with the
  per-linter caps off) are older than this feature and stay.
- `make test` (`go test -race ./...`): all 43 packages with tests pass.
- `npm test`: 1291 tests, 1290 pass, 1 skipped.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: built.

What did not run, and is still missing:

- **Real adapters on Linux.** The session's tool sandbox refused to run the
  npm-installed adapters in `/tmp/acp-spike/adapters` without the user's
  explicit approval. No real-agent criterion is marked as passed.
- **The desktop GUI.** This computer has no display.
- **WSL and the Windows test build.** This computer is plain Linux, not
  WSL2, and `/mnt/c/Users/Gink` does not exist.
- **An SSH host.** The configured hosts are the user's own servers. Nothing
  was installed or run on them without the user's go-ahead.

No check failed, so no new task was added.

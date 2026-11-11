# ACP chat mode — QA

Supports [acp-chat.md](acp-chat.md). Task:
[ACP-14](../tasks/acp-chat/ACP-14.md). Only checks that actually ran are
marked as passed. Every other check says why it did not run.

## Run of 2026-11-11

- **Commit:** `e1dd9d8` plus the ACP-14 lint fixes (committed with this file).
- **Host:** Linux (Ubuntu, kernel 7.0.0-43). This is not WSL: `/mnt/c` does
  not exist, and there is no display (`DISPLAY` and `WAYLAND_DISPLAY` are
  unset).
- **Agents installed:** Claude Code at `~/.local/bin/claude`, Codex at
  `~/.local/bin/codex`, OpenCode at `~/.opencode/bin/opencode`.
- **Adapters:** the ones ACP-02 installed into `/tmp/acp-spike/adapters`.
  These versions come from their `package.json` files, because the adapters
  were not run:
  - `@agentclientprotocol/claude-agent-acp` 0.106.0;
  - `@agentclientprotocol/codex-acp` 2.6.0.

  `opencode acp` is built into OpenCode.

### Automated checks

| Check | Result |
|---|---|
| `go tool golangci-lint run` (`make lint`) | ❌ Fails on 488 findings repository-wide, all older than this feature. See below. |
| `go tool golangci-lint run --new-from-rev d68793f` (findings added since the feature began) | ✅ 0 issues, after fixing 2 findings: `goimports` in `internal/engine/chat_launch.go` and `unparam` in `internal/rpc/chat_test.go`. |
| `go test -race ./...` (`make test`) | ✅ All 43 packages with tests pass; 4 packages have no test files. |
| `npm test` | ✅ 1291 tests: 1290 pass, 1 skipped, 0 fail. |
| `npm run check` (svelte-check) | ✅ 616 files, 0 errors, 0 warnings. |
| `npm run build` | ✅ Built. The only warning is Vite's existing one about large chunks. |

`make lint` fails on findings that are older than this feature. With
`--max-issues-per-linter=0 --max-same-issues=0` it reports 488. Its default
output caps them per linter and shows about 135, a different subset on each
run. `--new-from-rev d68793f` reports none of them, so the feature's own
code has no lint findings. This task leaves the old findings alone.
`make lint test` stops at the lint failure, so the tests ran separately as
`make test`.

### Release criteria with real agents

These checks need the real adapters running. Running them was refused in
this session: the tool sandbox blocks running the npm-installed adapters in
`/tmp/acp-spike/adapters` unless the user approves it by name. No check below
ran, so none is marked as passed.

| Criterion | Claude Code | Codex | OpenCode |
|---|---|---|---|
| Streamed text appears before the agent saves it | not run | not run | not run |
| Tool calls and permission requests show | not run | not run | not run |
| Allow and deny both work; cancel stops a turn | not run | not run | not run |
| Once saved, the transcript replaces the live tail without duplicates | not run | not run | not run |
| An existing session continues in chat | not run | not run | not run |
| The chat shows and changes mode and model | not run | not run | not run |
| Closing the chat or quitting ends the agent process and denies pending permissions | not run | not run | not run |

The ACP-02 spike ran the same adapter versions on this computer with a
throwaway client ([acp-chat-spike.md](acp-chat-spike.md)). It showed
streaming before save, permission allow and reject, cancel, load and resume,
and where each agent saves its sessions. That was the protocol, not this
app's chat, so it does not count as a pass here.

The following are covered by automated tests with the fake agent
(`internal/acp/acptest`), the mock agent (`frontend/src/lib/mockChat.ts`)
and synthetic saved transcripts. They are not real-agent checks:

- one owner per session;
- delete and move refusing while a chat owns the session;
- terminal and chat exclusivity;
- fail-closed permissions on close, cancel, exit, quit and disconnect;
- reconciling the live tail with each agent's saved records;
- servers without `ACPChat` hiding the action.

### Hosts

| Host | Result | Why not run |
|---|---|---|
| Linux, desktop app | not run | No display, so the GUI cannot be driven. The real adapters were not run (see above). |
| WSL distribution | not run | This computer is not Windows, so it has no WSL. |
| SSH host | not run | The SSH hosts in `~/.ssh/config` are the user's own servers. Installing adapters or running agents on them needs the user's go-ahead. |
| Windows test build (CLAUDE.md steps) | not run | The destination `/mnt/c/Users/Gink/agent-sessions-launchN` does not exist on this computer. |

## What is left

To finish ACP-14, run the real-agent table above on each host:

1. **Linux:** run the desktop app, or `agent-sessions-cli serve --stdio`
   driven by a client, with `claude-agent-acp`, `codex-acp` and `opencode`
   on PATH.
2. **WSL:** make a Windows test build on the WSL2 development machine as
   CLAUDE.md describes, then open a project in a WSL distribution with the
   adapters installed there.
3. **SSH:** do the same on an SSH host that has the adapters installed.

Record each result in a new dated run section here. Any failure becomes a
new task in [acp-chat](../tasks/acp-chat/README.md).

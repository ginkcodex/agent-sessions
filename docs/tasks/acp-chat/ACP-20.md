# ACP-20 — The chat shows the agent working while it works outside a turn

- **Status:** ✅ Done
- **Estimate:** 1.5 h
- **Depends on:** ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

The live tail's "<agent> is working…" spinner followed `chat.phase`, which
is `working` only while an ACP `session/prompt` runs. Claude's adapter
answers the prompt at Claude Code's result, but a command the agent sent to
the background (`run_in_background` Bash, Monitor) runs on. When it ends,
Claude Code starts a turn of its own with no prompt open. That turn's text
and tool calls streamed into the tail with the chat `idle`, so no spinner
showed while the agent was working.

The adapter does report this work, as AIR `async_task_spawned` and
`async_task_state_update` session updates (a `turn` task for the turn it
starts itself, `local_bash` for a background command). It sends them only to
a client that advertises `asyncTasks` in
`clientCapabilities._meta.jetbrains.air` (version 1). ACP v1 has no session
state, so these tasks are the only sign that the agent is working.

- The client now advertises `asyncTasks` (`acp.AsyncTasksMeta`) to every
  agent. The others ignore the meta.
- The chats store keeps the IDs of the running tasks (`Chat.tasks`): spawned
  or `running` adds one; `completed`, `failed`, `stopped` and `paused` remove
  it. The tasks stay out of the tail. The list is cleared when the chat ends.
- The live tail shows the spinner while the turn runs, or while the chat is
  idle and any task runs.

## Likely files

- `internal/acp/types.go`, `internal/acp/client.go`
- `frontend/src/lib/stores/chats.svelte.ts`
- `frontend/src/lib/components/chat/LiveTail.svelte`

## Acceptance criteria and tests

- `initialize` carries
  `{"_meta":{"jetbrains":{"air":{"capabilities":["asyncTasks"],"version":1}}}}`,
  and claude-agent-acp's `clientSupportsAsyncTasks` accepts it.
- A spawned task keeps the chat working after `turn_end`. Its end, a pause or
  the chat's end clears it. A repeated `running` adds no second copy.

## Evidence

- `TestClientCapabilitiesAskForAsyncTasks` (`internal/acp/types_test.go`)
  pins the wire shape. `go test ./internal/acp/ ./internal/engine/
  ./internal/app/`: pass. `go vet ./internal/acp/` and `gofmt -l`: clean.
- The pinned adapter's own check: `clientSupportsAsyncTasks` from
  claude-agent-acp 0.106.0 `dist/async-tasks.js` returns true for that JSON
  and false without it.
- `frontend/test/chats-store.test.mjs`, "the agent's async tasks keep it
  working…": pass. `npm test`: 1629 pass. `npm run check`: 0 errors.
- Not checked: a real Claude chat in the app (background command, then the
  turn Claude starts by itself). A window opened while a task already runs
  does not know about it until the task's next update, because the backend's
  chat state does not carry tasks. With `asyncTasks` set, the adapter no
  longer stops background workflows when a turn is cancelled
  (`stopsWorkflowsOnCancel`), and it marks a backgrounded Bash result with
  `_meta.jetbrains.air.asyncTasks.backgrounded`, which the app ignores.

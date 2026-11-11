# AAI-03 — Find and launch managed adapters

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** AAI-02
- **Plan:** [acp-adapter-install.md](../../plan/acp-adapter-install.md)

## Scope

- `ACPAdapter` gains the catalog ID of its managed copy (`claude-agent-acp`,
  `codex-acp`; none for OpenCode), and the provider comment drops "the app
  never installs it".
- `ChatAgents` and `ChatLaunch` look up an adapter: config override, then
  PATH and usual locations, then the managed copy. The managed command is
  `<node> <main>` with that Node's directory first on PATH, plus the CLI
  variable as today.
- `ChatAgent` gains `installTool` (set when this platform can install the
  adapter) and `managed`.
- The managed-tools `status` action also reports the adapter IDs.

## Likely files

- `internal/provider/provider.go`, `internal/provider/claude/claude.go`,
  `internal/provider/codex/codex.go`
- `internal/engine/chat_launch.go`, `internal/engine/launch.go`,
  `internal/engine/managed_tools.go`, `internal/engine/managed_tools_contract.go`
- tests beside them; `frontend/src/lib/types.ts` for the new fields

## Acceptance criteria

- With no adapter on PATH and a managed install, `ChatAgents` reports it
  available and `managed`, and `ChatLaunch` returns node and the main script.
- A copy on PATH wins over the managed one; an override wins over both.
- With nothing installed, `installTool` names the entry on macOS and Linux
  and is empty on Windows and for OpenCode.
- `status` lists both adapter IDs, locally and through RPC.

## Tests

Engine tests with a fake managed install and fake executables; an RPC test of
`status`. `go test -race` for engine, rpc and app.

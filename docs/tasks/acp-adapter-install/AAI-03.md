# AAI-03 — Find and launch managed adapters

- **Status:** ✅ Done
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

## Evidence

- `provider.ACPAdapter` gained `Managed`: `claude-agent-acp` for Claude Code,
  `codex-acp` for Codex, none for OpenCode. Its comment no longer says the
  app never installs the adapter.
- `findAdapter` looks up the config override, then PATH and the usual
  locations, then the managed copy. The managed command is
  `<node> <main>`, with the Node.js directory first on PATH. The CLI
  variable (`CLAUDE_CODE_EXECUTABLE`, `CODEX_PATH`) is set as before.
  A damaged install, or one without a Node.js to run it, is reported as
  the problem.
- `managedtools.Open(data)` is a read-only view of the managed installs,
  with no host binding: it cannot propose or install. `Installer.Runtime(tool)`
  picks the host Node.js when it is new enough for the entry (22 for the
  adapters), else the app's own. The engine's `managedAdapters()` uses it
  so that listing chat agents writes nothing (no host identity file). It
  returns nil on Windows and for an injected remote host, so `installTool`
  stays empty there.
- `ChatAgent` gained `installTool` and `managed`, in Go and in
  `frontend/src/lib/types.ts`.
- The managed-tools `status` action reports `claude-agent-acp` and
  `codex-acp` after the language servers.
- The plan's "Finding a managed adapter" section now says the Node.js is
  chosen at launch, and that the lookup is read-only.
- Tests:
  - `TestChatAgentsManagedAdapter`: no managed tools; installable but
    missing; installed (available, `managed`, main path); damaged; no
    Node.js; a copy on PATH wins without the managed copy being looked up;
    an override wins, even an unusable one; no entry means no `installTool`.
  - `TestChatLaunchManagedAdapter`: argv is node then main, the Node.js
    directory is first on PATH, and the CLI variable is set. A missing
    install is refused with the install command.
  - `TestEngineChatAgentsOffersManagedInstall`: the real engine with
    temporary config and data offers `claude-agent-acp` on Linux and
    macOS (empty elsewhere), and writes nothing to either directory.
  - `TestOpenRuntime`: the read-only view cannot propose. A host
    Node.js 20 runs pyright but not the adapters, and 22 runs them.
  - `TestManagedStatusReportsEachTool` and
    `TestManagedContractStatusAndUnavailable`: status includes both adapter IDs.
    `TestManagedToolsForwardedToVerifiedRemoteHost` checks that they cross
    RPC. Its backend is a fake, so this covers the wire, not a remote
    engine.
  - `TestProvidersDeclareACPAdapters` checks the new `Managed` IDs.
- `go test -race ./internal/engine ./internal/rpc ./internal/app ./internal/managedtools ./internal/provider/... ./internal/codediagnostics`:
  all pass. The first run failed the RPC test, because its fake still
  returned only `ruff`. The fake was fixed and `./internal/rpc` re-ran
  and passed.
- `GOOS=windows go build ./...` and `GOOS=darwin go build ./...`: build.
- `go tool golangci-lint run --new-from-rev efd7c3d ./internal/...`:
  0 issues. `npm run check`: 0 errors, 0 warnings.
- Not checked: a launch of a real managed adapter (AAI-05), and the Windows
  behaviour beyond the cross-build (the engine test's Windows branch did not
  run here).

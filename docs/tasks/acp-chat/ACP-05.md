# ACP-05 — Adapter commands and detection per provider

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** ACP-04
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- The optional interface `provider.ACPLauncher`. It became
  `ACPAdapter() ACPAdapter`, a description the engine resolves, instead of
  `ACPCommand(dir)`: lookup, overrides and environment are the same for
  every agent.
- Implement it for Claude (`claude-agent-acp`, `CLAUDE_CODE_EXECUTABLE`),
  Codex (`codex-acp`, `CODEX_PATH`) and OpenCode (`opencode acp`), with lookup
  through `launch.Find`.
- Path overrides in the config file (`[chat.adapters]`, keyed by agent ID).
- An engine `ChatAgents()`: for each agent, whether its adapter is available,
  the resolved path, and the install command to show when it is missing.
- Reuse the `knownDirectory` check from `NewSessionLaunch`.

## Likely files

- `internal/provider/provider.go`, and `claude/`, `codex/` and `opencode/`
- `internal/config/`, `internal/engine/service.go`

## Acceptance criteria

- Detection never runs a network install, and never runs the adapter itself.
- A missing adapter reports the install command for that agent.
- Overrides win over PATH, and an override that does not exist is reported.

## Tests

Unit tests with fake executables on a temporary PATH, override parsing, and
an unknown directory refused.

## Evidence

- **Added:**
  - `provider.ACPAdapter` and `provider.ACPLauncher`, implemented by:
    - Claude: `claude-agent-acp`, `CLAUDE_CODE_EXECUTABLE`;
    - Codex: `codex-acp`, `CODEX_PATH`;
    - OpenCode: `opencode acp`.
    Each declares its npm install command.
  - `launch.Find` looks for the two npm adapters in npm's usual prefixes too
    (`~/.npm-global/bin`, `~/.local/bin`, bun, volta, nvm, and
    `%APPDATA%\npm\*.cmd` on Windows).
  - `internal/engine/chat_launch.go`:
    - `LoadChatAdapters` reads `[chat.adapters]` from `config.toml`; an
      unknown agent key or malformed TOML is an error.
    - `Service.ChatAgents` reports per agent: available, the path, whether it
      is an override, the install command, and the problem when missing.
    - `Service.ChatLaunch` returns an `acp.Command`. It refuses unknown
      providers, agents without an adapter (`ErrChatUnsupported`), unknown or
      removed directories, and a missing adapter (`ErrAdapterNotFound`, with
      the install command).
    - It sets the CLI variable to the CLI new sessions would start, unless
      the user's environment sets it, and puts off-PATH install directories
      first on PATH.
    - `Engine.ChatAgents` and `Engine.ChatLaunch` apply the config file.
  - `Service.checkLaunchDir` is the directory check, shared with
    `NewSessionLaunch`.
- **Tests:**
  - `internal/engine/chat_launch_test.go`:
    - the three providers' adapters;
    - detection with the adapter missing (lookup only, nothing run) and
      found; the WSL "Windows program" hint is kept;
    - overrides: absolute, `~/`, missing, relative, a directory, not
      executable. Each is reported as an override and skips the lookup;
    - launch environment: the CLI variable, the user's own value kept, PATH
      untouched when both are on PATH, install directories prepended when
      not;
    - refusals: missing adapter with its install command, unknown
      directory, removed directory, no adapter, unknown provider, unusable
      override;
    - config parsing: other tables, blank values, an unknown agent,
      malformed TOML;
    - end to end through `Engine`, with a fake executable on a temporary
      PATH, then an override from `config.toml`, then a malformed file.
  - `internal/launch/find_test.go`: adapters found in `%APPDATA%\npm` and
    in `~/.npm-global/bin`.
- **Results:**
  - `go test -race ./internal/engine ./internal/launch ./internal/provider/...`
    passed.
  - `golangci-lint` reported nothing in the changed files for linux, windows
    and darwin. The packages' other reports (for example unused
    `evictTranscript` in `service.go`) were there before.
  - `go vet` passed for windows and darwin.
- **Not checked:**
  - No real adapter was started here; the spike (ACP-02) ran them, and ACP-14
    runs them through the app.
  - Windows lookup is tested with a fake profile only.
  - There is no Settings field for overrides; they are edited in
    `config.toml`, as the plan now says.

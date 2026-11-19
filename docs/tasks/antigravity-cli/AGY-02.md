# AGY-02 — Agent ID, OS roots, binary lookup, cache key and edit protection

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** [AGY-01](AGY-01.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Wire the new `antigravity` agent identity, storage root resolution, executable
discovery, cache-key isolation, editor store protection, and remote env
allowlist across shared backend packages before implementing the provider.

## Likely files

- `internal/model/model.go` — add `AgentAntigravity AgentID = "antigravity"`.
- `internal/paths/paths.go`, `paths_linux.go`, `paths_darwin.go`,
  `paths_windows.go` — add `Roots.Antigravity` defaulting to
  `~/.gemini/antigravity-cli` (`%USERPROFILE%\.gemini\antigravity-cli` on
  Windows, unverified until AGY-08), overridable via `ANTIGRAVITY_CLI_DIR`.
  That variable is this app's own (agy reads none); say so in its comment.
- `internal/engine/engine.go` — include `roots.Antigravity` in empty-roots
  check and `editPolicy()` protected roots.
- `internal/engine/cache.go` — include `e.roots.Antigravity` in `indexKey()`.
- `internal/rpc/server.go` — add `ANTIGRAVITY_CLI_DIR` to `allowlistedEnvKeys`.
- `internal/launch/find.go` — add `~/.local/bin` (verified on Linux and
  macOS) and
  `%LOCALAPPDATA%\agy\bin` (a guess, unverified), `"agy"` label
  (`"Antigravity"`), and install hint.

## Acceptance criteria

- `paths.Resolve()` resolves `Roots.Antigravity` on Linux, macOS, and Windows
  and honors `ANTIGRAVITY_CLI_DIR`.
- `engine.editPolicy()` rejects editing any path under `Roots.Antigravity`.
- Changing `ANTIGRAVITY_CLI_DIR` produces a distinct `indexKey()` hash.
- `launch.Find("agy")` finds `agy` both on `PATH` and in its default install
  directories when `PATH` is minimal (desktop menu launch).
- Remote RPC server forwards `ANTIGRAVITY_CLI_DIR` when configured for a host.

## Tests

- Unit tests for `paths.Resolve()` default + `ANTIGRAVITY_CLI_DIR` override.
- Unit tests for `engine` cache key sensitivity and `editPolicy()` protection.
- Unit tests for `launch.Find("agy")` fallback locations and `rpc` env
  allowlist.

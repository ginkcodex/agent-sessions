# AGY-02 — Agent ID, OS roots, binary lookup, cache key and edit protection

- **Status:** ✅ Done
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

## Evidence

- **Done:**
  - `model.AgentAntigravity` (`"antigravity"`).
  - `paths.Roots.Antigravity`: `~/.gemini/antigravity-cli` on Linux and
    macOS, `%USERPROFILE%\.gemini\antigravity-cli` on Windows, or an absolute
    `ANTIGRAVITY_CLI_DIR`. The comment says agy reads no such variable, and the
    Windows default is marked as a guess.
  - The root is wired into the engine's empty-roots checks, `editPolicy()` and
    `indexKey()`. `ANTIGRAVITY_CLI_DIR` is in `allowlistedEnvKeys`.
  - `launch`: `agy` is looked for in `~/.local/bin` and (Windows, a guess)
    `%LOCALAPPDATA%\agy\bin\agy.exe`, with the label `Antigravity` and an
    install hint.
- **Index key:** the key now hashes one more root, so every existing index
  is rebuilt once on upgrade (registering the agent in AGY-05 changes the
  key anyway). Tests that build the key by hand pass the extra `""`.
- **Tests run (Linux):**
  - `go test ./internal/paths ./internal/launch ./internal/engine
    ./internal/rpc ./internal/app ./internal/index`: pass.
  - New cases: an `ANTIGRAVITY_CLI_DIR` override and an ignored relative value
    per OS (`paths_*_test.go`); `TestIndexKeyIncludesAntigravityRoot`; the
    default Antigravity root in `TestSaveFileProtectsConfiguredAndDefaultRoots`;
    `agy` in `~/.local/bin` and the not-found hint in
    `TestFindPosixInstallDirs`; `%LOCALAPPDATA%\agy\bin\agy.exe` in
    `TestFindWindowsBundledCLIs`; `TestAllowlistedEnvKeys`.
  - `GOOS=windows` and `GOOS=darwin` `go vet` of `paths`, `launch` and
    `engine`: clean.
  - `golangci-lint run --new-from-rev=HEAD ./internal/...`: 0 issues.
- **Not checked:** the macOS and Windows path tests were compiled, not run;
  the Windows paths stay unverified until AGY-08. The allowlist test checks the
  map, not a full `initialize` round trip.

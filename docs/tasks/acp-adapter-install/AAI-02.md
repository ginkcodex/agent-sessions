# AAI-02 — Catalog entries for the two adapters

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** AAI-01
- **Plan:** [acp-adapter-install.md](../../plan/acp-adapter-install.md)

## Scope

- Generate `package.json` and `package-lock.json` for
  `@agentclientprotocol/claude-agent-acp` 0.106.0 and
  `@agentclientprotocol/codex-acp` 2.6.0 with
  `npm install --package-lock-only --ignore-scripts` in a scratch directory,
  and embed them under `internal/managedtools/claude-agent-acp/` and
  `internal/managedtools/codex-acp/`.
- Add the catalog entries `claude-agent-acp` and `codex-acp`: versions, main
  scripts, bins, lock package counts, `npmPackages` constructors, and the
  tool switches in `Propose` and `Discover`.
- A per-entry npm policy: these entries run `npm ci --omit=optional` and record
  `npm-ci-lockfile-ignore-scripts-omit-optional-v1` in the receipt; the
  review card's integrity text says optional packages are left out.
- A per-entry minimum Node.js major (22 for these entries, 18 for the rest),
  used when choosing the host Node.js.
- Keep the adapters out of the code-diagnostics tool lists and the automatic
  install.

## Likely files

- `internal/managedtools/catalog.go`, `npm.go`, `install.go`,
  `discovery.go`
- `internal/managedtools/claude-agent-acp/`, `codex-acp/`
- `internal/managedtools/npm_test.go`, `discovery_test.go`, `install_test.go`

## Acceptance criteria

- `Propose("claude-agent-acp")` and `Propose("codex-acp")` return proposals
  whose source, integrity and note describe the pinned package, the lockfile
  hash, the policy and the Node.js choice.
- An install with a fake npm runs `ci --omit=optional --ignore-scripts`; a
  language server's install is unchanged.
- A host Node.js 20 is not used for the adapters; 22 is.
- `Discover` finds an installed adapter and rejects a receipt with another
  policy.

## Tests

Unit tests in `internal/managedtools` with fake installs and a fake npm, plus
lockfile checks (v3, root package, counts). `go test -race` for the package.

## Evidence

- Lockfiles generated in a scratch directory with
  `npm install --package-lock-only --ignore-scripts` (no package code ran):
  `claude-agent-acp` locks 115 packages (13 optional), `codex-acp` 30
  (7 optional; `@openai/codex` resolves to 0.166.0).
- `npmPackage` gained `omitOptional` and `minNode`. The adapters install with
  `npm ci … --omit=optional`, record
  `npm-ci-lockfile-ignore-scripts-omit-optional-v1`, and use the host Node.js
  only at major 22 or later. Language servers keep `npm-ci-lockfile-ignore-scripts-v1`
  and the 18 minimum.
- The review card's note says when the host Node.js is too old for the
  adapter ("Node.js 20.11.0 on this host is older than 22. …").
- The adapters are proposable but kept out of code diagnostics:
  `managedtools.AdapterTools()` lists them, `ManagedToolNames()` and `Dirs()`
  do not, and `TestLanguagesCoverEveryProviderAndTool` now checks code
  diagnostics covers every proposable tool except the adapters.
- Tests added in `internal/managedtools/npm_test.go`:
  - `TestACPAdapterLockfilesPinOfficialPackages`: lockfile v3, package counts,
    every entry from the npm registry with sha512, the manifest pins the
    adapter exactly, the validated versions match the lockfile, and the bin
    points at the catalog's main script;
  - `TestACPAdapterInstallAndDiscovery`, for both adapters with a host
    Node.js 22, 20 and none: proposal text, fake npm arguments
    (`--omit=optional`), the Node.js chosen, the receipt policy, `Discover`,
    the adapters missing from `Dirs()`, and a receipt with the language
    servers' policy rejected.
  - The existing pyright and vtsls tests still require the old npm
    arguments, so a language server's install is unchanged.
- `go test -race ./internal/managedtools ./internal/engine ./internal/codediagnostics ./internal/rpc ./internal/app`:
  all pass. (The first run failed `TestLanguagesCoverEveryProviderAndTool`;
  fixed as above and re-run.)
- `go tool golangci-lint run --new-from-rev 40ce7cf ./internal/managedtools/... ./internal/codediagnostics/...`:
  0 issues.
- Not checked: a real `npm ci --omit=optional` of either lockfile, and
  whether the adapters start without their optional packages (AAI-05).

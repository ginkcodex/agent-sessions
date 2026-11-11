# AAI-02 — Catalog entries for the two adapters

- **Status:** ⬜ Not started
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

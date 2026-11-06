# Code checks: Ruby (RuboCop) — task briefs

Plan: [code-diagnostics-ruby.md](../plan/code-diagnostics-ruby.md) ·
[Root tracker](../../TASKS.md#follow-up--ruby-code-checks)

Planned 2026-11-07. Each task is at most four developer hours and ends with
its tests passing: `go test ./internal/...`, `GOOS=windows` and `GOOS=linux`
`go vet ./internal/... ./cmd/...`, and from `frontend/` `npm test` and
`npx svelte-check --threshold error`. Native Windows/Linux runs and real remote
hosts are outside these tasks unless a brief says otherwise. Tests built on
fake trees or fake servers are synthetic and are labeled so.

| ID | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [RB-00](#rb-00--rubocop-server-spike) | ✅ Done | Spike: real `rubocop --lsp` behavior, recorded evidence | 3 | — |
| [RB-01](#rb-01--ruby-runtime-resolution) | Not started | Finding the Ruby interpreter and its version | 4 | — |
| [RB-02](#rb-02--gems-and-bundler) | Not started | `Gemfile.lock`, `.bundle/config`, finding RuboCop and Bundler | 4 | RB-01 |
| [RB-03](#rb-03--rubocop-adapter) | Not started | `rubocop_lsp.go`: discovery, launch, approval, notices | 4 | RB-00, RB-02 |
| [RB-04](#rb-04--wiring) | Not started | Language table, bridge, workspace scan, engine environment | 4 | RB-03 |
| [RB-05](#rb-05--frontend) | Not started | Language entry, labels, setup text, install hints | 3 | RB-04 |
| [RB-06](#rb-06--smoke-test-docs-and-gate) | Not started | Real-server smoke test, evidence replay, docs, gate | 3 | RB-05 |
| **Total** | **1/7** | | **25** | |

RB-00 and RB-01 can run in parallel.

## RB-00 — RuboCop server spike

**Goal.** Facts about the real server before the adapter depends on them.

Run the latest RuboCop on a disposable synthetic project with an empty temp
`HOME` and the launch shape from the plan, through a small stdio driver (as
for Ruff), in both Bundler and standalone mode. Record:

- `initialize` result: `serverInfo`, position encoding, `textDocumentSync`
  kind, whether it advertises `diagnosticProvider`.
- Whether pushes follow `didOpen`/`didChange`, and whether they carry
  `version`. Payloads for a syntax error (`Lint/Syntax`), an offense, a fix
  (empty push) and `didClose`.
- Server-to-client requests, if any (`workspace/configuration`,
  registrations).
- Files written to the project, `HOME` or caches (`~/.cache/rubocop_cache`,
  `XDG_CACHE_HOME`); network use with a local `inherit_from`.
- That `<ruby> -rbundler/setup <gem>/exe/rubocop --lsp` starts with only the
  planned environment, and what happens with a non-UTF-8 locale.
- `shutdown`/`exit` behavior and exit code.
- Briefly, for the later ruby-lsp plan: push or pull diagnostics, and what it
  writes to `.ruby-lsp/`.

**Done when.** `internal/codediagnostics/testdata/rubocop/evidence.json` holds
the observed payloads with the RuboCop, Ruby and Bundler versions and the
host platform, and the plan's "Observed behavior" section summarizes them. If
pushes are unversioned, the plan names the chosen freshness rule.

**Outcome (2026-11-07).** RuboCop 1.92.2 on mise Ruby 4.0.9, darwin/arm64,
both modes. Pushes are unversioned, so RB-04 extends the "observed at
receipt" rule to RuboCop. Columns are UTF-16. `didClose` sends nothing.
Nothing is written to the project or `HOME`. Bundler mode needs no
`GEM_HOME`/`GEM_PATH`, and no mode needs a locale. Not checked: URL
`inherit_from`, plugins, a differing `BUNDLED WITH`, and ruby-lsp, which
would have needed another install; that is left to the ruby-lsp plan.

## RB-01 — Ruby runtime resolution

**Goal.** Pick the interpreter a Ruby developer's shell would, without running
anything.

- `ruby_runtime.go`: `RubyEnvironment{Home string}`;
  `resolveRuby(root, workspace, reviewedPath, env)` returning the interpreter's
  real path, its prefix, the version, the ABI version (`3.3.0`), the version
  file used, and a state (`""`, `missing`, `notInstalled`, `tooOld`,
  `foreign`).
- Version request: `.ruby-version`, then the `ruby` entry of `mise.toml`,
  `.mise.toml` or `.tool-versions` (workspace, then ancestors inside the
  root), then `Gemfile.lock` `RUBY VERSION`. Normalize `ruby-` prefixes and
  `p<n>` suffixes; accept partial versions; reject anything that is not a
  plain version. Bounded reads; only the `[tools]` `ruby = "…"` line of the
  TOML files, no general TOML.
- Install directories in the plan's order; exact directory or alias link
  (resolving to a sibling) first, else the newest installed version matching
  a partial request. The interpreter is `<prefix>/bin/ruby` and must be a
  native executable.
- No request: first native `ruby` on the reviewed path, scripts and shims
  skipped; a mise shim (a link to `mise`) is foreign.
- `rbconfig.rb` under `<prefix>/lib/ruby/<abi>/<arch>/`: read
  `RUBY_PROGRAM_VERSION` and `ruby_version`; 2.7 is the minimum.

**Done when.** Synthetic tests cover each version manager, mise alias links
and partial requests (modelled on the observed `4 -> ./4.0.9` layout), an
alias link leaving the installs directory, a requested version
that is not installed (reported, not replaced), rbenv and mise shims on `PATH`, a missing or
malformed `rbconfig.rb`, Ruby 2.6, links leaving the prefix, and oversized
version files.

## RB-02 — Gems and Bundler

**Goal.** Find RuboCop (and Bundler, in Bundler mode) as installed files.

- `ruby_gems.go`: a bounded `Gemfile.lock` reader for the `GEM`/`specs`
  section (top-level `name (version)` lines only), `RUBY VERSION` and
  `BUNDLED WITH`; a reader for `BUNDLE_PATH` in `.bundle/config` (YAML key
  lines only, no general YAML).
- Gem directories in the plan's order; `BUNDLE_PATH` relative to the
  workspace must stay inside the root.
- `findGem(dirs, name, version)` (exact) and `newestGem(dirs, name)`; each
  requires `gems/<name>-<v>/exe/<name>` and
  `specifications/<name>-<v>.gemspec` as regular files within the gem
  directory. Platform-suffixed directories (`-x86_64-linux`) are accepted.
- Mode selection: Bundler when the lockfile lists `rubocop`, else standalone.

**Done when.** Synthetic tests cover locked and standalone selection, a locked
version that is not installed, `vendor/bundle`, a `BUNDLE_PATH` outside the
root, malformed and oversized lockfiles, `BUNDLED WITH` present and missing,
and links leaving the gem directory.

## RB-03 — RuboCop adapter

**Goal.** A Ruff-shaped plan for RuboCop.

- `rubocop_lsp.go`: `rubocopProviderID = "rubocop"`, `RubyLanguageID`,
  `rubyDocumentExtensions`, `rubocopProjectMarkers`, `DiscoverRubocop` and
  `discoverRubocopWorkspace(root, reviewedPath, env, workspace)`.
- `RubocopPlan` with `Availability`, `Version`, `RubyVersion`, `Mode`,
  `Projects`, `ProjectLocal`, `Server`, `Provider` and `covers`.
- `ServerConfig` exactly as in the plan's Launch and Approval sections:
  arguments, environment, policy with the mode, bound tools in `Toolchain`,
  bound configuration in `ConfigPaths`; `Fingerprint` failure becomes
  `unsupported`.
- Notices and install hints from the plan, per state.

**Done when.** Synthetic tests check the full `ServerConfig` for both modes,
every unavailable state and its notice, the policy change between modes, and
that a project without Ruby files is muted `unsupported`.

## RB-04 — Wiring

**Goal.** Ruby discovered, approved and bridged like the other languages.

- `languages.go`: the `ruby` entry and its `languageFiles` rule.
  `TestLanguagesCoverEveryProviderAndTool` still passes with no Ruby tool.
- `push_bridge.go`: the `rubocop` spec.
- `workspace_adapters.go`: the `workspaceProviders` entry; the scan sets a
  `ruby` flag for `Gemfile`, `Gemfile.lock` and `.rubocop.yml`; skips
  `.bundle`, `.ruby-lsp`, `vendor/bundle`, and `tmp`/`log` beside a `Gemfile`,
  without marking partial; `providerRoots` and the markerless-source root for
  Ruby.
- `live_discovery.go`: `LocalLiveEnvironment.Ruby`. Engine
  `code_diagnostics_local_live.go` passes `Home`.
- `live_results.go`: the "observed at receipt" rule covers `rubocop` as well
  as `typescript` (RB-00 found unversioned pushes), with a test that RuboCop
  findings keep their range, record `observedVersion` and stay `uncertain`.

**Done when.** Synthetic tests cover single-root and nested Gemfile projects,
`DiscoverLocalLive` order, the disabled language (no lookup, `disabled`
registration only with Ruby files), a push becoming findings and an empty push
clearing them, and an unowned file ignored. Existing tests pass unchanged.

## RB-05 — Frontend

**Goal.** Ruby shows up wherever the other languages do.

- `codeLanguages.ts`: `{ id: 'ruby', name: 'Ruby', providers: ['rubocop'],
  extensions: ['.rb', '.rake', '.gemspec', '.ru'] }`.
- `codeCheckLabels.ts`: "Ruby linter (RuboCop)".
- `CodeChecks.svelte`: setup text, install hints (`bundle add rubocop
  --group development`, `gem install rubocop`), no managed-tool row, and the
  approval text saying RuboCop gives no navigation.
- `codeNavigation.ts`: Ruby files say no language server provides navigation.

**Done when.** Frontend tests cover the language entry and Settings switch,
the label, the install hints, the absence of an Install button, and the
navigation message. `npm test` and `svelte-check` pass.

## RB-06 — Smoke test, docs and gate

**Goal.** One real run through the Go adapter, and the docs match the code.

- `TestLocalLiveRealRubocopSmoke` in `internal/engine`, opt-in with
  `AGENT_SESSIONS_RUBOCOP_DIR`: approval required before start; syntax error,
  offense and clear, each `current`; nothing written to the project or `HOME`.
- `TestRubocopEvidenceReplay` over `testdata/rubocop/evidence.json`.
- Docs: contract R12 (`ruby` in `disabledLanguages`), the zed-model discovery
  table, the languages table, `README.md`, the plan's status and "Where the
  code differs" notes, `TASKS.md`.

**Done when.** The automated gate passes, the smoke test has run once on this
host with its result recorded in the plan, and the plan states what was not
tested (other platforms, remote hosts, manual editor QA).

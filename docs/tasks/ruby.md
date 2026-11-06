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
| [RB-01](#rb-01--ruby-runtime-resolution) | ✅ Done | Finding the Ruby interpreter and its version | 4 | — |
| [RB-02](#rb-02--gems-and-bundler) | ✅ Done | `Gemfile.lock`, `.bundle/config`, finding RuboCop and Bundler | 4 | RB-01 |
| [RB-03](#rb-03--rubocop-adapter) | ✅ Done | `rubocop_lsp.go`: discovery, launch, approval, notices | 4 | RB-00, RB-02 |
| [RB-04](#rb-04--wiring) | ✅ Done | Language table, bridge, workspace scan, engine environment | 4 | RB-03 |
| [RB-05](#rb-05--frontend) | ✅ Done | Language entry, labels, setup text, install hints | 3 | RB-04 |
| [RB-06](#rb-06--smoke-test-docs-and-gate) | ✅ Done | Real-server smoke test, evidence replay, docs, gate | 3 | RB-05 |
| **Total** | **7/7** | | **25** | |

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

**Outcome (2026-11-07).** `resolveRuby(dirs, lockedRuby, reviewedPath, env)`
takes the canonical directories to search (workspace first, the authorized
boundary last) instead of a root, because a nested workspace's scoped root
cannot see its ancestors; RB-03 binds an ancestor's version file as a
toolchain file. Gemfile.lock's `RUBY VERSION` comes in as a string from the
RB-02 reader. Differences from the brief:

- Requests are read nearest directory first: in each directory
  `.ruby-version`, then `mise.toml`, `.mise.toml`, `.tool-versions`; the
  lockfile only when no directory has one. "system" defers to `PATH`.
- A lockfile request also accepts the newest installed patch release of the
  same minor version, whose gems share the ABI directory. Version files stay
  exact.
- A sixth state, `invalidRequest`: a version file that exists but is not a
  plain MRI version (oversized, `jruby-…`, a TOML array, a link leaving the
  boundary) stops discovery instead of falling back to another Ruby.
- `RUBY_PROGRAM_VERSION` is `"$(MAJOR).$(MINOR).$(TEENY)"` in a real
  `rbconfig.rb`, so the literal `MAJOR`, `MINOR`, `TEENY` and `ruby_version`
  are read. Debian's `lib/<multiarch>/ruby/<abi>/rbconfig.rb` and `ruby3.1`
  binary name are accepted; macOS's `/usr/lib/ruby` link is followed, so its
  2.6 reports `tooOld`.
- On `PATH`, a Ruby that is too old is skipped when a usable one follows,
  and reported only otherwise: a Finder-launched app inherits `/usr/bin`
  (macOS Ruby 2.6) before the login shell's mise directory.
- `RubyEnvironment` also carries `RbenvRoot`, `MiseDataDir` and `AsdfDataDir`
  (absolute values only). `ShimSkipped` records that a shim preceded the
  `PATH` Ruby, for the notice.

A read-only probe of this host found mise Ruby 4.0.9 (ABI 4.0.0) for a `4.0.1`
lockfile request and `/usr/bin/ruby` 2.6.10 as `tooOld` behind the mise shim.

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

**Outcome (2026-11-07).** `readGemfileLock`, `bundleSetting`, `findGem`,
`newestGem`, `defaultBundler` and `findRubocop(workspace, lock, …, rt, env)`
in `ruby_gems.go`. Differences from the brief:

- With a `BUNDLE_PATH`, only `<path>/ruby/<abi>` is searched for RuboCop:
  Bundler installs every gem there. `BUNDLE_DEPLOYMENT: "true"` implies
  `vendor/bundle`; an absolute `BUNDLE_PATH` in `~/.bundle/config` is honored.
  Absolute project paths are allowed; relative ones must stay inside the
  workspace, also through links.
- User gem directories: `~/.gem/ruby/<abi>`, its XDG form
  `~/.local/share/gem/ruby/<abi>`, chruby's `~/.gem/ruby/<version>` and rvm's
  `~/.rvm/gems/ruby-<version>`, then the interpreter's.
- Bundler's entry file is `lib/bundler/setup.rb`. The interpreter's default
  Bundler (`specifications/default/bundler-<v>.gemspec` with
  `lib/ruby/<abi>/bundler/setup.rb`, the layout on this host) counts as
  installed. `BUNDLER_VERSION` is set only when `BUNDLED WITH` names an
  installed Bundler other than the newest; an absent one leaves the newest
  running without it.
- More states: `unsupportedSource` (RuboCop locked from a `GIT` or `PATH`
  source), `invalidLockfile`, `invalidBundleConfig`, `bundlePathOutside`,
  `missingBundler`.
- A release beats any prerelease in standalone mode, as RubyGems activates.

A read-only probe of the RB-00 Bundler project found its vendored RuboCop
1.92.2 and the default Bundler 4.0.20, with no `BUNDLER_VERSION`.

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

**Outcome (2026-11-07).** `rubocop_lsp.go` with `rubocop_lsp_test.go`
(synthetic; each check was confirmed to fail when its rule is removed).
Differences from the brief:

- Only Ruby sources (`.rb`, `.rake`, `.gemspec`, `.ru`) enable RuboCop. A
  `Gemfile` alone, as in iOS (fastlane, CocoaPods) and Jekyll projects, gives
  the muted `unsupported`, never an install request.
- `projectMarkers` gained `skipPath` (`vendor/bundle` at any depth) and
  `skipBeside` (`tmp` and `log` beside a `Gemfile`), both silent, as `skipDir`
  is. `tsWorkspace` gained `ancestors`, set by RB-04 for nested workspaces,
  so their version requests are found. A version file above the scoped root
  is bound as the `rubocop-toolchain-version-file` toolchain file; one inside
  it is in `ConfigPaths`, as are `.bundle/config` and every
  `Gemfile`/`Gemfile.lock`/`.rubocop.yml`/`.rubocop_todo.yml`.
- Toolchain files: RuboCop's `exe/rubocop` and gemspec, and in Bundler mode
  Bundler's `setup.rb` and gemspec, all with policy
  `rubocop-toolchain-v1:loaded-file`.
- Two more `unsupported` states: a `Gemfile.lock` that locks RuboCop without a
  `Gemfile` beside it, and a Ruby `bin` directory (or, in standalone mode, a gem
  directory) containing `:`, which cannot go on `PATH` or `GEM_PATH`.
- With `ShimSkipped`, the too-old notice says shims are not run and suggests
  a `.ruby-version` file.

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

**Outcome (2026-11-07).** Wired as briefed; each new rule was confirmed to
fail its test when removed. A read-only probe of the engine's real discovery
on this host found the RB-00 projects available: Bundler mode with the
vendored RuboCop 1.92.2, and standalone mode with mise Ruby 4.0.9 from the
login shell `PATH`, with no `.ruby-version`. Differences from the brief:

- Only a `Gemfile` marks a nested Ruby workspace. A `.rubocop.yml` in a
  subdirectory is a RuboCop config for that subtree, not a separate Bundler
  context, and a lone `Gemfile.lock` is not a project. Both are still bound
  as configs of the workspace that owns them.
- `.bundle`, `.ruby-lsp` and `vendor/bundle` are skipped by the shared scan for
  every language, so a Ruby project's hidden `.bundle` no longer marks
  TypeScript or Python coverage partial. `tmp` and `log` are skipped only
  beside a `Gemfile`.
- A `Gemfile` whose boundary owns no Ruby source (a Jekyll `docs/`) is
  dropped like other manifest-only boundaries, and its file is bound by the
  enclosing workspace.
- `LocalLiveEnvironment.Ruby.Home` defaults to `Go.Home`, as Rust's and
  Svelte's do. The engine passes `RBENV_ROOT`, `MISE_DATA_DIR` and
  `ASDF_DATA_DIR`. No `PATH` directory was added: mise's Ruby has no `latest`
  link, and the login shell `PATH` already has it.
- Four existing tests that list every live provider count `rubocop` now; no
  other expectation changed.
- Not handled: Homebrew Ruby's own gem directory
  (`$(brew --prefix)/lib/ruby/gems/<abi>`, outside the Cellar prefix). It
  could not be checked on this host, which has no Homebrew Ruby, so a RuboCop
  installed with that Ruby's `gem install` reports `missingTool`.

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

**Outcome (2026-11-07).** Done as briefed; each new rule was confirmed to
fail its test when removed. Differences from the brief:

- The navigation message comes first: a file that only RuboCop declares says
  "No language server provides go to definition, references or hover for Ruby
  files" whether or not RuboCop is installed or the project approved, so it
  never asks for an approval or install that would not help. Only an
  off-in-Settings or host reason comes before it.
- The setup sentence that names the languages with app downloads now lists
  only languages that have a managed install, so Ruby is left out of it.
- A Ruby project (a RuboCop row that is not the muted "no Ruby file" one) gets
  a setup paragraph: RuboCop is never downloaded, how its Ruby is found,
  that Bundler mode loads the Gemfile, and that it gives no navigation.
- The Settings switch needed no code: it lists the language table. Its test
  and the language-table test now include Ruby.

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

**Outcome (2026-11-07).** `TestLocalLiveRealRubocopSmoke` passed once on
this host (darwin/arm64, RuboCop 1.92.2, mise Ruby 4.0.9, standalone mode,
temp dirs under `/private/tmp`); the result is in the plan's status.
`TestRubocopEvidenceReplay` replays the recorded scenarios and rebuilds the
Bundler and UTF-16 summaries into pushes. The gate passes. Differences from
the brief:

- Findings are `uncertain`, not `current`: RuboCop's pushes are unversioned,
  so the smoke test checks the observed version and the kept range instead.
- `AGENT_SESSIONS_RUBOCOP_DIR` names the bin directory of a native Ruby whose
  own gem directory has RuboCop. `HOME` is an empty temp directory, so the run
  is standalone mode, and only Ruby is discovered (the other languages need
  tools of their own).
- Bundler mode was not run through the Go adapter; the RB-00 spike and the
  RB-04 discovery probe cover it.

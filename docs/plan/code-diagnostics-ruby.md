# Code checks: Ruby (RuboCop) (2026-11-07)

**Status: implemented 2026-11-07** ([tasks RB-00…RB-06](../tasks/code-diagnostics-ruby/README.md);
wire details in the [contract, R12](code-diagnostics-contract.md#r12-additive-ruby-language-2026-11-07)).
Parent plans: [code-diagnostics.md](code-diagnostics.md), the
[editor (Zed) model](code-diagnostics-zed-model.md) and
[language settings](code-diagnostics-languages.md). Closest precedents: Ruff
([code-diagnostics-python.md](code-diagnostics-python.md)) for the provider
shape and Pyright (`pyright_lsp.go`) for an interpreter bound into approval.

Where the code differs from the plan below (details in each task's Outcome):

- Ruby resolution takes the workspace's directories up to the opened folder,
  nearest first; a version file that exists but names no plain CRuby version
  (`invalidRequest`) stops discovery instead of falling back. A lockfile
  request also accepts the newest installed patch of the same minor version.
  On `PATH`, a too-old Ruby (macOS's 2.6) is skipped when a usable one follows.
- Only Ruby sources enable RuboCop; a `Gemfile` alone (fastlane, CocoaPods,
  Jekyll) gives the muted `unsupported` row. Only a `Gemfile` marks a nested
  Ruby workspace. `.bundle`, `.ruby-lsp` and `vendor/bundle` are skipped by
  the shared scan for every language.
- More `unsupported` states than planned: RuboCop from a `git`/`path` source,
  an unreadable `Gemfile.lock` or `.bundle/config`, a `BUNDLE_PATH` outside
  the project, a locked RuboCop without a `Gemfile`, and a Ruby or gem
  directory containing `:`.
- Findings are `uncertain`, not `current`: the pushes carry no version, so
  each records the version observed at receipt and keeps its range.
- The frontend says "No language server provides go to definition, references
  or hover for Ruby files" before any approval or install reason, and leaves
  Ruby out of the languages with app downloads.
- Not handled: Homebrew Ruby's own gem directory
  (`$(brew --prefix)/lib/ruby/gems/<abi>`), which lies outside its Cellar
  prefix; a RuboCop installed there reports `missingTool`.

Tested: synthetic Go tests for every rule (each confirmed to fail when its rule
is removed), the evidence replay, frontend tests, and one real run of
`TestLocalLiveRealRubocopSmoke` on darwin/arm64 (RuboCop 1.92.2, mise Ruby
4.0.9, standalone mode): approval required before open, `Lint/Syntax` as an
error, three convention offenses as information, an empty push for the fixed
draft, each finding `uncertain` with its range and observed version, and
nothing written to the project or `HOME`. Not tested: Linux, native Windows
(unsupported anyway), real SSH/WSL hosts, Bundler mode through the Go adapter
(only in the RB-00 spike and a read-only discovery probe), and manual editor
QA.

## Goal

Ruby becomes the sixth language in Settings › Code checks. Opened `.rb` files
get lint and syntax findings from RuboCop's built-in language server
(`rubocop --lsp`), on the local route and on SSH/WSL hosts, under the same
project approval as every other language.

## Decisions

- **RuboCop first.** It is the linter almost every Ruby project already uses,
  the project's `Gemfile.lock` pins its version and plugins, and its server
  pushes diagnostics. It reports lint and syntax problems, not type errors, and
  offers no navigation; the UI says both.
- **No app download in this phase.** Gems need a Ruby runtime and some compile
  native extensions at install time, so there is no equivalent of a pinned
  archive or `npm ci --ignore-scripts`. A missing RuboCop is `missingTool` with
  an install hint; `Language.Tools` for Ruby is empty.
- **Later, separate plans** (not part of these tasks):
  - **ruby-lsp** for Go to Definition, hover and syntax errors. It creates
    `.ruby-lsp/` in the project and runs `bundle install` to compose its
    bundle, so it would need an approved preparation step like Rust's, and it
    may use pull diagnostics, which the live bridge does not support. RB-00
    records what we observe so that plan starts from evidence. Now planned in
    [code-diagnostics-ruby-lsp.md](code-diagnostics-ruby-lsp.md), which
    avoids the composed bundle and add-ons (spike RL-00).
  - **Sorbet** (`srb tc --lsp`) only for projects with `sorbet/config`. A
    native binary and the only real type checker.
  - **A managed RuboCop**, installed with Bundler from an embedded `Gemfile`
    and a `Gemfile.lock` carrying `CHECKSUMS`.
  - A saved whole-project `rubocop --format json` checker for Check Project,
    like `go vet` and `cargo check`.

## What makes Ruby different

| Issue | Consequence |
|---|---|
| Ruby is usually installed by a version manager (rbenv, asdf, mise, chruby, rvm) whose `ruby` on `PATH` is a `#!` shim | Rejecting scripts, as for Ruff, would turn Ruby off for most users. Discovery resolves the real interpreter from the project's version file and the managers' install directories instead, without running anything. |
| Bundler decides RuboCop's version and plugins | Bundler mode loads the project's `Gemfile`, which is Ruby code. Approval already covers running project code (as for Cargo build scripts); the notice says so. |
| RuboCop configuration can load code and use the network | `require:`/`plugins:` load gem code; `inherit_from`/`inherit_gem` can name URLs or other gems. They still apply but are not bound; the notice says so. |
| Gems are many Ruby files | Like Pyright, only the entry points and the interpreter are bound, plus the project files that select them. The rest of the gem tree is a documented limit. |
| Push versions | RuboCop's pushes carry no version (RB-00). The bridge keeps ranges of unversioned pushes only for TypeScript (`live_results.go`), so the rule is extended to RuboCop. |

## Behavior

### Ownership and project detection

- **Ruby files**: `.rb`, `.rake`, `.gemspec`, `.ru` → language `ruby`.
  Ownership is by extension, so `Gemfile`, `Rakefile` and other extensionless
  Ruby files are not checked in this phase. ERB templates are not owned.
- **Project markers**: `Gemfile`, `Gemfile.lock`, `.rubocop.yml`. Any owned Ruby
  file also enables Ruby for its project, as for Python.
- **Nested projects**: each directory with a `Gemfile` is a workspace, one
  RuboCop server per workspace, through the nested-workspace scan that
  TypeScript and Python use (`workspace_adapters.go`, at most 16 instances).
- **Skipped silently**: `.bundle`, `.ruby-lsp`, and `vendor/bundle`; `tmp` and
  `log` only in a directory holding a `Gemfile` (Rails projects fill them and
  would exhaust the scan's entry budget). Other hidden directories still mark
  coverage partial.

### Finding Ruby (metadata only)

1. The project's requested version, from the workspace or an ancestor inside
   the root: `.ruby-version`, else the `ruby` entry of `mise.toml`,
   `.mise.toml` or `.tool-versions`, else `RUBY VERSION` in `Gemfile.lock`. A
   `ruby-` prefix and patch-level suffix are normalized. A request may be
   partial (`4`, `3.3`).
2. That version under the known install directories, in this order:
   `~/.rbenv/versions/<v>`, `~/.local/share/mise/installs/ruby/<v>`
   (`$MISE_DATA_DIR` when set in the reviewed environment),
   `~/.asdf/installs/ruby/<v>`, `~/.rubies/ruby-<v>`, `/opt/rubies/ruby-<v>`,
   `~/.rvm/rubies/ruby-<v>`. An exact directory or link wins; mise keeps
   links for partial versions (`4 -> ./4.0.9`, `latest`, `global`), which must
   resolve to a sibling directory. Otherwise a partial request matches the
   newest installed version with that prefix. A requested version that is not
   installed is reported ("Ruby 3.3.5, requested by .ruby-version, is not
   installed"), never replaced by another Ruby.
3. With no project request, the first native `ruby` on the reviewed path
   (login-shell `PATH` and the reviewed executable directories), Homebrew's
   included. A shell with `mise activate` puts
   `~/.local/share/mise/installs/ruby/<v>/bin` there directly; that is how a
   global mise version (`~/.config/mise/config.toml`) is honored without
   parsing user configuration. Shims are skipped as for Ruff: rbenv/asdf shims
   are scripts, and mise shims are links to the `mise` binary, so the resolved
   file must be named `ruby`.
4. The version is read from the interpreter prefix's `rbconfig.rb`
   (`<prefix>/lib/ruby/<abi>/<arch>/rbconfig.rb`, `RUBY_PROGRAM_VERSION`),
   never by running Ruby. Ruby older than 2.7 is reported as too old; this
   covers macOS's `/usr/bin/ruby` (2.6).

Observed on the development host (2026-11-07): mise Ruby 4.0.9 at
`~/.local/share/mise/installs/ruby/4.0.9` (links `4`, `4.0`, `latest`,
`global`), requested only by the global mise config, ABI directory `4.0.0`,
arch `arm64-darwin25`, Bundler 4.0.20 as a regular gem in the interpreter's
gem directory, no RuboCop installed.

### Finding RuboCop (metadata only)

- **Bundler mode**, when the workspace's `Gemfile.lock` lists `rubocop` under
  `specs`. The gem directory `gems/rubocop-<locked version>` is looked up in
  `BUNDLE_PATH` from the project's `.bundle/config` (as
  `<path>/ruby/<ABI version>`), then the user gem directory
  (`~/.gem/ruby/<ABI version>`), then the interpreter's own gem directory
  (`<prefix>/lib/ruby/gems/<ABI version>`). Bundler itself is found the same
  way, at the `BUNDLED WITH` version, falling back to the interpreter's
  default bundler. A locked RuboCop that is not installed reads "RuboCop
  <v> from Gemfile.lock is not installed. Run bundle install, then refresh."
  The app never runs it.
- **Standalone mode**, otherwise: the newest `rubocop-<v>` gem in the user or
  interpreter gem directory. Plugins named in `.rubocop.yml` must then be
  installed globally too; RuboCop reports it if not.
- Each candidate needs `gems/rubocop-<v>/exe/rubocop` and
  `specifications/rubocop-<v>.gemspec` as regular files inside its gem
  directory. Links leaving the gem directory are rejected.
- None found: `missingTool`, "RuboCop is not installed. Add it to your Gemfile
  and run bundle install, or run gem install rubocop, then refresh."
- No Ruby file in the project: muted `unsupported`, no install hint.

### Launch

No shell, no shim, no `bundle exec` looked up on `PATH`. Arguments:

- Bundler mode: `<ruby> -rbundler/setup <gem>/exe/rubocop --lsp`.
- Standalone: `<ruby> <gem>/exe/rubocop --lsp`.

Environment, exactly (confirmed by RB-00):

- Both modes: `HOME` and `PATH=<ruby bin>:/usr/bin:/bin`. No locale variables.
- Standalone: `GEM_HOME`/`GEM_PATH` naming the gem directories found.
- Bundler: `BUNDLE_GEMFILE=<workspace>/Gemfile` and `BUNDLE_FROZEN=true`, no
  `GEM_HOME`/`GEM_PATH`; `bundler/setup` finds the gems from the Gemfile and
  `.bundle/config`. `BUNDLER_VERSION` is set to the lockfile's `BUNDLED WITH`
  only when that differs from the interpreter's Bundler (not yet observed).

The server runs in the canonical workspace directory.

### Approval

- **Tools bound**: the resolved interpreter, `exe/rubocop`, the RuboCop
  gemspec, and in Bundler mode Bundler's entry file and gemspec.
- **Configuration bound**: `Gemfile`, `Gemfile.lock`, `.bundle/config`,
  `.rubocop.yml`, `.rubocop_todo.yml` and the version file used, for each
  workspace.
- **Policy**: `rubocop-v1:any-version:<os>/<arch>:ruby-direct:<bundler|standalone>:push-only:no-edits:opened-documents`.
  Switching mode, Ruby or RuboCop version changes the fingerprint; the
  project's grant carries over as for other tool upgrades.
- **Notice**: RuboCop reports lint and syntax problems, not type errors.
  Bundler mode loads the project's Gemfile. Plugins, `inherit_from` and
  `inherit_gem` targets and your user RuboCop configuration still apply but
  are not part of approval. Opened documents only.

### Results

- One push bridge entry (`push_bridge.go`) with the shared normalizer.
  Severities map as RuboCop sends them; codes are cop names
  (`Style/StringLiterals`, `Lint/Syntax`).
- RuboCop's pushes carry no version (RB-00). The TypeScript "observed at
  receipt" rule in `normalizeLiveDiagnostic` is extended to `rubocop`: each
  finding records the mirror's version when the push arrives
  (`observedVersion`), keeps its range, and stays `uncertain`, never
  `current`. Without the rule the ranges would be dropped and nothing drawn
  inline. The provider list for the rule becomes a small set with its own
  test.
- RuboCop sends nothing on `didClose`; `CloseDocument` already removes a
  closed document's results.
- Go to Definition in a Ruby file says no language server provides navigation
  for Ruby; the panel's approval text says RuboCop gives no navigation.

### Platforms

macOS and Linux, amd64 and arm64, local and remote (SSH, WSL), like the other
languages. Windows projects stay unsupported. Version-manager directories are
looked up under the executing host's `HOME`.

### Compatibility

Additive. `ruby` joins the `disabledLanguages` IDs (contract R12, contract
version stays 1). Older servers ignore `ruby` and have no Ruby provider. An
older desktop talking to a newer server shows the `rubocop` provider by its
raw ID and has no switch for it.

## Observed behavior (RB-00, 2026-11-07)

RuboCop 1.92.2 on mise Ruby 4.0.9, darwin/arm64, driven by a Python stdio
script (not the Go adapter) on a disposable project in a canonical temp path,
with an empty temp `HOME` and no workspace folders. Payloads:
`internal/codediagnostics/testdata/rubocop/evidence.json`.

**Standalone mode** (`ruby <gem>/exe/rubocop --lsp`, env `HOME`, `PATH`,
`GEM_HOME`, `GEM_PATH`):

- `initialize` returns no `serverInfo` and no `positionEncoding` (UTF-16 by
  default); `textDocumentSync {openClose, change: 2}`; it also advertises code
  actions, formatting and two `executeCommand` commands, which the app does not
  use.
- Every `didOpen`/`didChange` got one push within about 3 ms, **without a
  `version`**, although the client declared `versionSupport`.
- `Lint/Syntax` is severity 1; convention offenses are severity 3
  (information). Messages begin with the cop name; syntax messages end with
  "This offense is not autocorrectable." Diagnostics carry quick-fix `data`.
- A clean draft got an empty push; `didClose` got none.
- No server-to-client requests. Nothing was written to the project or `HOME`,
  and no `rubocop_cache`. stderr has three `[server]` lines. `shutdown` ⇒
  `null`, exit 0. Startup to `initialize` result took about 2.3 s.
- Installing RuboCop compiled native extensions for prism, racc and json,
  which supports leaving app downloads out.

**Bundler mode**: a project with `gem "rubocop", "1.92.2"`, `.bundle/config`
`BUNDLE_PATH: "vendor/bundle"`, installed offline with `bundle install
--local`. Bundler 4.0.20 wrote `PLATFORMS arm64-darwin-25 / ruby` and a
`CHECKSUMS` section into `Gemfile.lock` by default; gems landed in
`vendor/bundle/ruby/4.0.0/gems/rubocop-1.92.2`. Then
`ruby -rbundler/setup <vendor gem>/exe/rubocop --lsp` with only `HOME`,
`PATH`, `BUNDLE_GEMFILE` and `BUNDLE_FROZEN=true`:

- Started without `GEM_HOME`, `GEM_PATH` or `BUNDLER_VERSION` (the lockfile's
  Bundler matched the interpreter's). `initialize` result after about 1.6 s.
- The project's `.rubocop.yml` applied (`EnforcedStyle: double_quotes` changed
  which literals were flagged). Pushes as in standalone mode, unversioned.
- Nothing written: `Gemfile.lock` unchanged, `HOME` empty.

**Encoding**: columns are UTF-16 code units (a semicolon after two emoji is at
character 10, not 8), so ranges need no conversion. Every run had no `LANG` or
`LC_*` set and handled emoji and accented text, so the launch adds no locale.

**Not checked** in RB-00: `inherit_from` with a URL (network use is taken as
given from RuboCop's documentation; the notice covers it), plugins
(`plugins:`/`require:`), a lockfile `BUNDLED WITH` that differs from the
interpreter's Bundler, and ruby-lsp (left to its own plan).

## Changes

| Area | Change |
|---|---|
| `internal/codediagnostics/ruby_runtime.go` (new) | Version request, install directories, `rbconfig.rb` version, gem directories |
| `internal/codediagnostics/ruby_gems.go` (new) | `Gemfile.lock` and `.bundle/config` readers, RuboCop and Bundler lookup |
| `internal/codediagnostics/rubocop_lsp.go` (new) | `DiscoverRubocop`, `RubocopPlan`, `RubyLanguageID`, server config, notices |
| `languages.go` | `{ID: "ruby", Name: "Ruby", Live: ["rubocop"]}` and its file rule |
| `push_bridge.go` | `rubocop` spec |
| `workspace_adapters.go` | `workspaceProviders` entry; scan marks `ruby` directories and skips Ruby dependency trees; `providerRoots` for Ruby |
| `live_discovery.go` | `LocalLiveEnvironment.Ruby` |
| `live_results.go` | The "observed at receipt" rule covers `rubocop` too |
| `internal/engine/code_diagnostics_local_live.go` | `Ruby: cd.RubyEnvironment{Home: home}` |
| `frontend/src/lib/codeLanguages.ts` | Ruby entry |
| `codeCheckLabels.ts`, `CodeChecks.svelte`, `codeNavigation.ts` | Label, setup text and install hints, no-navigation wording |
| Docs | Contract R12, zed-model discovery table, languages table, `README.md` |

## Tests

- Go, synthetic trees: every Ruby resolution path (each version manager,
  requested-but-missing, `PATH` with shims skipped, too-old Ruby), lockfile and
  `.bundle/config` parsing including malformed and oversized input, Bundler
  and standalone selection, links leaving the gem directory, nested Gemfiles,
  the skip lists, the disabled language, approval inputs and policy.
- Fake server: push to findings, clear on empty push, unowned files ignored.
- Evidence replay of real RuboCop pushes (`testdata/rubocop/evidence.json`).
- Opt-in real-server smoke test (`AGENT_SESSIONS_RUBOCOP_DIR`), skipped by
  default.
- Frontend: language entry, labels, the Settings switch, install hints.
- `GOOS=windows` / `GOOS=linux` vet, as for every code-checks change.

## Not included

- ruby-lsp, Sorbet, Steep, Solargraph; type checking and navigation for Ruby.
- App downloads of Ruby, Bundler or RuboCop; running `bundle install`.
- Extensionless Ruby files (`Gemfile`, `Rakefile`), ERB, RBS.
- Code actions and autocorrect.
- Native Windows, real SSH/WSL runs and manual editor QA, unless a task says
  otherwise.

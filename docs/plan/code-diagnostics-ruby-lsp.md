# Ruby Go to definition (ruby-lsp)

**Status:** planned 2026-11-07; spike [RL-00](../tasks/code-diagnostics-ruby-lsp/RL-00.md)
done, implementation not started ([tasks](../tasks/code-diagnostics-ruby-lsp/README.md)).
Parent plans: [code-diagnostics-ruby.md](code-diagnostics-ruby.md) (RuboCop,
Ruby resolution, gems and Bundler) and the
[editor (Zed) model](code-diagnostics-zed-model.md). Closest precedent:
Pyright beside Ruff for Python, where the language server answers navigation
and the linter keeps its own diagnostics.

## Goal

Go to Definition, Find References and hover in opened Ruby files, from
Shopify's [ruby-lsp](https://github.com/Shopify/ruby-lsp), on the local route
and on SSH/WSL hosts, under the same project approval as every other
language. RuboCop stays the source of Ruby diagnostics.

## Observed behavior (RL-00, 2026-11-07)

ruby-lsp 0.26.11 on mise Ruby 4.0.9 (Bundler 4.0.20), darwin/arm64, driven by
a Python stdio script that sends this app's `initialize` parameters, on
disposable projects with an empty temp `HOME`. Payloads:
`internal/codediagnostics/testdata/ruby-lsp/evidence.json`.

- **The default launch is unusable here.** `exe/ruby-lsp` without
  `BUNDLE_GEMFILE` composes a bundle in `<project>/.ruby-lsp/` (`Gemfile`,
  `Gemfile.lock`, `.gitignore`), runs `bundle install` against rubygems.org
  (debug, irb, rdoc, prism with native extensions, 248 cache files under
  `HOME/.bundle`) and then re-execs itself with `bundle exec`. `initialize`
  answered after 44 s.
- **`BUNDLE_GEMFILE` set skips all of that.** With
  `BUNDLE_GEMFILE=<workspace>/Gemfile` (present or not) the exe starts the
  server in-process: `initialize` in 0.15–0.7 s, nothing written, no network.
- **Add-ons cannot be turned off by option.** On `initialized` the server
  always loads add-ons: every `ruby_lsp/**/addon.rb` on the gem path (RuboCop
  1.92.2 ships one, which activated) and every `ruby_lsp/**/addon.rb` under
  the workspace. A marker add-on in the project ran and wrote a file: the exe
  runs project code. ruby-lsp-rails, a common add-on, boots the Rails app.
- **An app-owned wrapper avoids add-ons.** `ruby -e <wrapper> <gem>/lib`
  loads `ruby_lsp/internal`, prepends a no-op `RubyLsp::Server#load_addons`
  and starts the server as the exe does. Result: no add-on, no RuboCop
  activation, empty stderr, nothing written, `initialize` in 0.27 s, and the
  same navigation answers as the exe.
- **Features turn off as asked.** `enabledFeatures` with every non-navigation
  feature `false`, `linters: []` and `formatter: "none"` leave
  `definitionProvider`, `referencesProvider`, `hoverProvider`, plus `rename`
  and `documentRangeFormatting`, which the app never calls. No
  `diagnosticProvider`; ruby-lsp diagnostics are pull-only anyway, and the
  live bridge never pulls.
- **Protocol.** `serverInfo {name: "Ruby LSP", version}`, `positionEncoding
  utf-16`, `textDocumentSync {openClose, change: 2}` (incremental; a change
  without a range broke the next request). No server-to-client requests from
  this client's capabilities; five `window/logMessage` notifications.
  `shutdown` ⇒ `null`, exit 0.
- **Answers.** Within a file and across project files (definition, references,
  hover with a markdown "Definitions" link), into Ruby's standard library
  (`<prefix>/lib/ruby/4.0.0/json/common.rb`), into a gem from the lockfile
  (`gems/parallel-2.3.0/lib/parallel.rb`), and into core RBS inside the `rbs`
  gem (`rbs-4.2.0/core/string.rbs`). Each answer took under 50 ms. A bare
  method name with no receiver type matches every method of that name.
- **Index.** Indexing runs in the background after `initialize`. Before it
  ends, definition answers `[]` at once. Without `window.workDoneProgress`
  in the client there is no signal when it ends; on the small fixture it
  ended within the 8 s wait. The index covers project `*.rb`, Ruby's default
  gems and the lockfile's gems except the `development` group and its
  dependencies (so a locked `json` displaced by RuboCop's dependency resolved
  to nothing).
- **Drafts.** A method defined only in an unsaved draft is not found. ruby-lsp
  re-indexes an open document only while answering `documentSymbol`,
  `foldingRange`, `documentLink` or `codeLens` (its "combined requests"), and
  ignores watched-file changes for open documents. Saved changes to closed
  files arrive through `workspace/didChangeWatchedFiles`, which it handles
  whether or not the client advertised watching.
- **Gemfile evaluation.** ruby-lsp calls `Bundler.definition` for the index
  configuration, which evaluates the project's `Gemfile` (Ruby code), as
  RuboCop's Bundler mode does.

Not checked: indexing time on a real project and the `$/progress` sequence;
the wrapper on a project without a `Gemfile`; whether forwarding saved
changes and a `documentSymbol` request re-index as the source says; ruby-lsp
from a project bundle; Linux, SSH/WSL and Rails projects. The tasks below
cover them.

## Decisions

- **A second Ruby live provider**, `ruby-lsp`, next to `rubocop`:
  `{ID: "ruby", Live: ["rubocop", "ruby-lsp"]}`. `navigationRoute` already
  picks the owner offering the most navigation answers, so RuboCop keeps
  diagnostics and ruby-lsp answers navigation, as Pyright beside Ruff.
- **Launch through the app's wrapper, never `exe/ruby-lsp`.** The wrapper is a
  constant in the Go code: binary mode on the standard streams, prepend the gem's
  `lib`, `require "ruby_lsp/internal"`, check that
  `RubyLsp::Server` defines `load_addons` (otherwise print a fixed message and
  exit 3, which discovery reports as an unsupported ruby-lsp version), prepend
  the no-op, `$> = $stderr`, `RubyLsp::Server.new.start`. Its text is bound
  into the policy.
- **Supported versions:** ruby-lsp 0.26 and newer (the observed version), on
  Ruby 3.0 or newer (its gemspec). Older versions are `unsupported` with
  "ruby-lsp <v> is older than 0.26; run gem update ruby-lsp".
- **Environment**, exactly: `HOME`, `PATH=<ruby bin>:/usr/bin:/bin`,
  `GEM_HOME`/`GEM_PATH` naming the gem directories found,
  `BUNDLE_GEMFILE=<workspace>/Gemfile` (also when it does not exist) and
  `BUNDLE_FROZEN=true`. No `-rbundler/setup`: ruby-lsp and its dependencies
  load from the gem directories, not the project bundle, so the server works
  in projects whose bundle lacks it. The lockfile only tells the index which
  gems to read.
- **Finding ruby-lsp (metadata only)**, with the Ruby from `ruby_runtime.go`:
  the version locked in `Gemfile.lock` if it lists `ruby-lsp`, looked up in
  `BUNDLE_PATH`, the user gem directory and the interpreter's gem directory;
  otherwise the newest `ruby-lsp` in the user or interpreter gem directory.
  It needs `gems/ruby-lsp-<v>/lib/ruby_lsp/internal.rb` and its gemspec as
  regular files inside the gem directory. Its runtime dependencies
  (`language_server-protocol`, `prism`, `rbs`) must exist by name in those
  directories or as default gems; versions are not checked. None found:
  `missingTool`, "ruby-lsp is not installed. Run gem install ruby-lsp, then
  refresh." No app download, for the reasons in the RuboCop plan.
- **`initializationOptions`**: `enabledFeatures` with every feature except
  definition, references and hover `false`, `linters: []`,
  `formatter: "none"`. Bound into the policy like other adapter settings.
- **Client unchanged** for the first release: no `window.workDoneProgress`,
  server requests still answered with MethodNotFound. An empty Ruby answer
  says the index may still be building.
- **Index freshness.** Saved `.rb` transitions from the approved project
  watcher are forwarded to ruby-lsp as `workspace/didChangeWatchedFiles`, as
  they are for gopls. Before a navigation request, each open Ruby document
  changed since its last re-index gets one `textDocument/documentSymbol`
  request (result discarded), so drafts and open saved files are indexed.
- **External locations**: Ruby's library directory, the gem directories and
  `.rbs` files in the `rbs` gem open read-only through `grantExternal`.
- **Approval.** Tools bound: the interpreter, the ruby-lsp gemspec and
  `lib/ruby_lsp/internal.rb`, the wrapper text. Configuration bound:
  `Gemfile`, `Gemfile.lock`, `.bundle/config` and the version file, per
  workspace. Policy:
  `ruby-lsp-v1:min-0.26:<os>/<arch>:ruby-wrapper:no-addons:navigation-only:no-edits:opened-documents`.
  Notice: "ruby-lsp reads the project's Ruby files, Ruby's standard library
  and the gems in Gemfile.lock to answer navigation. It evaluates the project's
  Gemfile. Add-ons are not loaded. Opened documents only."
- **Workspaces** as for RuboCop: each directory with a `Gemfile` is a
  workspace, one server each, through the nested-workspace scan.

## Risks

| Risk | Mitigation |
|---|---|
| The wrapper relies on ruby-lsp internals (`ruby_lsp/internal`, `Server#load_addons`, `start`) | The wrapper checks for the method and exits 3 with a fixed message, reported as an unsupported version instead of a crash; the real-server smoke test runs it on each new ruby-lsp release we try. |
| Without add-ons, Rails-specific navigation (routes, associations) is missing | Documented limit; the approval notice says add-ons are not loaded. A later option could run named add-ons with their own approval. |
| Empty answers while indexing look like "no definition" | The wording for an empty Ruby answer mentions indexing; RL-08 can add a real progress state. |
| The `documentSymbol` re-index is unofficial behavior | Verified by the real-server test in RL-07; if it stops working, drafts fall back to saved-file freshness. |
| Dependencies resolve from the gem directories, not the bundle; a locked `prism` older than ruby-lsp needs is not a conflict here, but a missing dependency only shows at start | Discovery checks dependency names; a start failure shows stderr as for other servers. |
| `Bundler.definition` runs the project's Gemfile | Same exposure as RuboCop's Bundler mode; the notice says so and the Gemfile is bound into approval. |
| RuboCop may still be required inside ruby-lsp when a `Gemfile` exists (its runner checks for one), even with `linters: []` | Observed nothing on stderr and no activation; it loads RuboCop's library only, not project configuration. Rechecked in RL-07. |

## Release criteria

- Go to Definition, references and hover work in a Ruby file on darwin and
  Linux (local), with RuboCop's findings unchanged beside them.
- Nothing is written to the project or `HOME`, no network is used and no
  add-on runs (the opt-in real-server test checks all three).
- A missing, too-old or incompatible ruby-lsp gives the right state and hint.
- Go tests for discovery, launch, policy, routing and freshness; frontend
  tests; `GOOS=windows`/`GOOS=linux` vet.

## Not in scope

- Sorbet, Steep, Solargraph; type checking.
- A managed (downloaded) ruby-lsp, or running `bundle install`.
- ruby-lsp add-ons, its diagnostics, formatting, code actions, completion and
  rename.
- ERB and extensionless Ruby files (`Gemfile`, `Rakefile`), as for RuboCop.
- Native Windows projects.

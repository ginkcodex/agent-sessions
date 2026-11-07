# Code checks: Git ignore rules in project discovery

Planned 2026-11-07. Status: **implemented**; the desktop app check (IS-04)
and repository filter programs (IS-06) are open.
Tasks: [code-diagnostics-ignore-scope](../tasks/code-diagnostics-ignore-scope/README.md).

Request: Go files in this repository showed *Go language server (gopls) —
Unsupported — No contained Go module was found*, although `go.mod` is at the
root. Discovery should leave out what Git ignores (`dist/`, build output),
but still look into a nested repository that the outer one ignores, because
that is related source. Files, Changes and History already tell these apart.

## Cause

Each checker's discovery walks the project with its own bounded, rooted walk
(`internal/codediagnostics`): at most 4096 entries and depth 32, depth-first,
entries sorted by name, skipping only fixed names (`vendor`, `node_modules`,
dot folders, …). None of them reads `.gitignore`.

In this checkout `build/`, `cmd/`, `dist/`, `docs/` and `frontend/` sort before
`go.mod`. `frontend/dist` (1755 entries) and `frontend/public` (1498) use up
the budget inside `frontend/`, so the walk stops before it reads `go.mod`. The
gopls walk (`go_lsp.go`) and the go vet walk (`go_vet.go`) are the same and
both report "No contained Go module was found".

## How Files, Changes and History decide

`internal/files`:

- `gitIgnored` runs `git ls-files -z --others --ignored --exclude-standard
  --directory`: Git reports a wholly ignored folder once (`dist/`) and never
  reads it.
- `nestedRepositoryFiles`: an ignored folder that has its own `.git` marker
  (a directory or a regular file, never a symlink) is a repository of its
  own, so it is source: its files are listed with its own ignore rules.
- `gitHistoryNestedCandidates` offers untracked or ignored folders with a
  marker, and submodules, as repositories.

In this checkout Git ignores `build/`, `build/bin/`, `build/remote/`,
`dist/`, `frontend/dist/`, `frontend/node_modules/` and `web/`; `web/` has its
own `.git`.

## Design

1. **`files.IgnoredDirectories(ctx, dir)`** runs the `ls-files` call above in
   `dir` and returns only the folder entries (`…/`), slash-separated and
   relative to `dir`. It reports "unknown" (not an empty set) when Git is
   missing, `dir` is in no worktree, Git fails or times out, or Git answers
   `./` (the whole folder is ignored by an outer repository, as a dotfiles
   repository in the home folder with `*` would do). Git runs with
   `-c core.fsmonitor=false` (a repository's own config could otherwise make
   `ls-files` start a program before the project is approved),
   `--literal-pathspecs`, and History's environment (`GIT_OPTIONAL_LOCKS=0`
   and the no-fetch settings).
2. **A Git scope per discovery walk** (`codediagnostics`, new file). It asks
   Git once at the project root. When the walk reaches a folder that has its
   own `.git` marker, it asks that folder's Git too and adds its answer
   under that prefix. So an ignored nested repository is walked with its own
   rules, and so are untracked nested repositories and submodules. A folder
   Git ignores, with no marker of its own, is skipped. It is not counted as
   partial coverage: it is out of the project's source on purpose. At most
   16 Git calls per walk and 5 s in total; after that the walk carries on
   with the name rules only. When Git is unknown, everything behaves as
   today.
3. **Files before folders.** The affected walks visit a folder's files
   before its subfolders. A project's root marker (`go.mod`, `package.json`,
   `svelte.config.js`, …) is then always seen, whatever sits beside it. When
   the budget runs out, the folder being read still records what it found,
   and the result is partial, not "no module". A walk never reads a folder
   once the budget is spent (`ReadDir(n ≤ 0)` would read it whole).

The scope only removes folders a walk would otherwise enter. Folders the
name rules already handle stay as they are, including the `.venv`/`venv`
check for `pyvenv.cfg` in the nested workspace scan.

### Walks changed

| Walk | File | Change |
|---|---|---|
| gopls modules | `go_lsp.go` `discoverGoWorkspace` | scope, files first |
| go vet modules | `go_vet.go` `discover` | scope, files first |
| TypeScript / Pyright / Ruff / Ruby roots | `ts_lsp.go` `discoverProjectDirs` | scope, files first, budget guard |
| Nested workspaces | `workspace_adapters.go` `scanLiveWorkspaces` | scope, files first |
| Svelte check | `svelte.go` `walkContained` | scope, files first |
| Svelte language server | `svelte_lsp.go` `discoverSveltePackages` | scope, files first, budget guard |

### Walks not changed

- **Rust** (`rust_discovery.go`): its walk is a fail-closed review of every
  Cargo and rust-analyzer config that Cargo may load, not a search for
  coverage. Skipping ignored folders would leave some of that unreviewed.
- **Project watcher** (`project_watch.go`) and **Go syntax**
  (`go_syntax.go`): they decide which saved files are checked. Ignored
  folders can hold generated source that is still compiled; leaving them out
  would hide changes there. Go syntax already walks breadth-first.
- **Files, Changes, History**: unchanged.

## Decisions

- Ask Git rather than parse `.gitignore` in Go. Git applies every rule
  (nested `.gitignore`, `.git/info/exclude`, the user's global ignore file,
  negation) and matches what the Files tree shows dimmed. Discovery so far
  ran no process; this adds one hardened, read-only `git ls-files`. The file
  tree already runs Git in every opened project.
- Only folders are skipped, never single ignored files: a Go file that Git
  ignores is still a Go file of the module.
- A folder Git ignores that has its own `.git` is walked; a repository
  inside an ignored folder that is not one itself (`build/x`) is not, as in
  Files and History.
- Skipping an ignored folder does not mark coverage partial. Skips by name
  keep their current partial rules.

## Risks

- **Renewed approval.** The approval fingerprint binds the config files
  discovery finds (`go.mod`, `go.sum`, `package.json`, …). Where the found
  set changes (more found at the root, fewer in ignored folders), the
  project asks for approval again once.
- **`.gitignore` edits** change what the next discovery finds. Discovery
  already reruns on its usual triggers; nothing new is watched.
- **Cost.** One `git ls-files` per walk, and one per nested repository
  reached: about 5 ms in this checkout, six walks per discovery. Bounded by
  the 16-call and 5 s limits.
- **Remote hosts** run the same code on the server; without Git there,
  discovery behaves as before.
- **Found along the way:** the Files, Changes and History Git calls did not
  set `core.fsmonitor=false`, so a repository's own config could start a
  program when the file tree was opened; fixed in IS-05. `git status` also
  runs a clean filter the repository's config sets; tracked as IS-06.

## Release criteria

- In this checkout, gopls and go vet discovery find the root module, and
  TypeScript and Svelte discovery find `frontend/` and `web/`.
- Tests with real temporary repositories cover: an ignored folder skipped
  without partial; an ignored nested repository walked with its own rules;
  an untracked nested repository's ignored folder skipped; a root ignored by
  an outer repository and a non-repository root behave as today; Git
  missing; `core.fsmonitor` not run; a root marker found when a large folder
  sorts before it.
- `go test ./internal/files ./internal/codediagnostics ./internal/engine`
  and `golangci-lint` pass.

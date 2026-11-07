# Git ignore rules in repositories below a non-Git folder

Planned 2026-11-07. Tasks: [nested-repo-gitignore](../tasks/nested-repo-gitignore/README.md).

Report: in Go to File, clicking a line of some files shows the blame bar for a
moment, then it closes. Seen on the remote host `plgl`, project
`/home/haitran/Workspaces/v2`, file
`cadence/devops/staging/init-scripts/postgres/01-init-extensions.sql`.

## Cause

`v2` is not a Git repository; `cadence`, `cadence-admin`, `dtp4a` and others
inside it are. A project root that is not in a work tree is listed by walking
it (`walkPaths`, `internal/files/files.go`). The walk skips only fixed folder
names (`skipDirs`: `node_modules`, `.venv`, `.mypy_cache`, …) and never reads
the repositories' ignore rules. `cadence/.gitignore` has `devops/staging/`
(line 220), so Git reports the file as `!!`, but the tree shows it as source.

Blame then:

1. The listing does not say the file is ignored, so the viewer shows the
   blame bar ("Loading blame…").
2. `git blame` fails on the untracked file; `BlameFile` asks `git status
   --ignored`, gets `!!` and answers `{ignored: true}`.
3. The viewer records the file as ignored (`ignoredFiles` in
   `FileViewer.svelte`), `blameShown` turns false, and the bar disappears,
   for every line of that file.

A project that is itself a repository does not do this: its listing comes
from Git, ignored files are listed apart and dimmed, ignored folders are not
read until opened, and the blame bar never shows for them.

The same walk feeds project search for a non-Git root (`walk` in
`search.go`), so search reads ignored files there too, and ignored caches and
generated folders count toward the listing's file cap.

## Goal

A repository found below a non-Git project root is listed with its own Git
ignore rules, as it would be if it were opened as the project: ignored files
are in `Ignored` (dimmed), wholly ignored folders are in `IgnoredDirectories`
(dimmed, read when opened), and neither is in `Files`. When blame still meets
an ignored file, the bar says so instead of closing.

## Design

### Git scope per repository (`internal/files`)

When the walk enters a folder that has its own `.git` marker (a directory or
a regular file, never a symlink, as `nestedRepositoryFiles` and
`codediagnostics.hasGitMarker` decide), it asks that folder's Git once:

```
git -C <repo> --literal-pathspecs -c core.fsmonitor=false -c core.quotepath=off \
    ls-files -z --others --ignored --exclude-standard --directory
```

with `GIT_CEILING_DIRECTORIES=<parent of repo>` (as `repositoryStatus` does,
so an invalid `.git` never answers with an outer repository's paths) and
History's environment (`GIT_OPTIONAL_LOCKS=0`, no fetch, no replace objects).
`--directory` reports a wholly ignored folder once (`dist/`) without reading
it; a folder that holds tracked files is never collapsed, its ignored files
are reported one by one. Entries are prefixed with the repository's path
relative to the root.

The answer is unknown (not empty) when Git is missing, fails, times out,
answers `./` (the repository is ignored by an outer one), or the budget is
spent. A repository whose scope is unknown is walked with the name rules, as
today.

Budget per listing: at most 64 scope queries and 5 s in total. On `plgl` one
query takes 15–26 ms and `v2` holds 15 repositories, 13 of them reachable.

### Walk

For each entry, the innermost repository the walk is in decides:

| Entry | Known scope | Unknown scope / outside any repository |
|---|---|---|
| `.git`, `.hg`, `.svn` folder | left out | left out |
| Folder with its own `.git` marker | new repository: added to `Repositories`, status started, its own scope asked, walked | same |
| Folder Git reports wholly ignored | `IgnoredDirectories`, not walked | — |
| Folder in `skipDirs` | walked unless Git ignores it | `IgnoredDirectories`, not walked (today) |
| File Git reports ignored | `Ignored` | — |
| Other file | `Files` | `Files` |

Ignored files count toward the file cap with `Files` and `Directories`, as a
Git-rooted listing limits `Ignored` to what is left of the cap.

Search for a non-Git root reads `Files` only, so it leaves ignored files and
folders out, as search in a Git-rooted project does.

### Viewer

When `BlameFile` answers `ignored`, the bar stays and reads "Ignored by Git —
no blame", and the file is not asked again. A file the listing already lists
as ignored still shows no bar. Files opened from an ignored folder are not in
the listing; their first click asks the host, which answers ignored, or blame
when the file is in a repository of its own inside that folder.

## Decisions

- **Ask Git, do not parse `.gitignore`.** Git applies nested `.gitignore`
  files, `.git/info/exclude`, the user's global ignore file and negations,
  and its answer is what a Git-rooted listing shows.
- **Inside a repository, Git decides, not names.** A committed `vendor/` or
  `build/` is listed, a non-ignored `.venv` is walked, exactly as when the
  repository is the project. Names decide only where Git cannot.
- **A repository is always source.** A folder with its own marker is walked
  with its own rules even when the outer repository ignores it (for example
  an ignored `plugins/x` that is a clone). This keeps what the walk does
  today for such repositories (status, blame, Changes). A Git-rooted listing
  dims those files instead; that difference is kept, not widened.
- **Repositories inside an ignored folder are not found.** `cadence` ignores
  `system_plugins/`, which holds clones two levels down. They are not walked
  (as in Files for a Git-rooted project and in code-check discovery), so they
  lose tree status tints; blame still works when such a file is opened,
  because `BlameFile` finds Git from the file's own folder.
- **Fail open to today's behavior.** Any Git failure for a repository falls
  back to the name rules for that repository; the listing never fails because
  of a scope query.

## Risks

- **Listing time.** One extra Git call per repository the walk reaches, run
  before the walk enters it. Measured cost goes in the task evidence; the
  budget caps the worst case at 5 s.
- **Walked repositories that do not ignore their dependencies** now have
  `node_modules` or `.venv` listed, as a Git-rooted listing of the same
  repository does, and may reach the file cap sooner.
- **Fewer repositories** (those inside ignored folders) in `Repositories`, so
  fewer status tints and fewer entries offered by the Changes pane picker.
- **`.gitignore` edits** change the next listing, which already reruns on its
  usual triggers; nothing new is watched.
- **Older remote servers** keep the old walk until the server bundle is
  rebuilt and deployed; the viewer change works with either.

## Release criteria

- On `plgl`, `v2`: `cadence/devops/staging/` is dimmed and not walked; a file
  inside it, opened, shows "Ignored by Git — no blame" instead of the bar
  closing; tracked files in `cadence` still blame; status tints in `cadence`,
  `cadence/ui` and `dtp4a` still show.
- Real-Git tests for the cases in the Walk table, Git failure, the budget,
  the file cap and search; Go and frontend tests pass.
- Listing time for a walked root with several repositories measured before
  and after.

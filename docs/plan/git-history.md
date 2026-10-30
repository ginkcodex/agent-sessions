# Go to File — Git commit history

- Status: planned; implementation not started.
- Agreed layout: IntelliJ-inspired history browser inside Go to File.
- Task breakdown: [GH-01…GH-18](../tasks/git-history.md).

## Goal and scope

Inspect past work without leaving Go to File. Support a session directory that
is a Git repository, a directory inside one, or a non-Git parent containing
multiple Git repositories. Browse one repository at a time.

History is read-only. Selecting a branch filters history; it never checks out
that branch. No checkout, reset, revert, cherry-pick, fetch, push, or other Git
write actions are part of this feature. Remote branches are already-fetched
refs only. Existing Changes actions remain separate.

First version includes repository selection, local/remote branch selection,
commit search and pagination, commit metadata, changed files, and immutable
historical diffs. Commit graph lanes, tags browsing, combined repository
history, arbitrary revision expressions, and advanced User/Date/Paths filters
are deferred.

## Agreed layout

History is a dedicated dialog-body layout, **not** another narrow picker pane.
Files, Search, and Changes retain their current layouts. Keep the mode switch
accessible in both layouts without resetting working-file tabs or drafts.

```text
 Go to File                                  project / host       ×
 Files   Search   Changes   History
────────────────────────────────────────────────────────────────────
 Repository: [ agent-sessions ▾ ]       [ Search text or hash… ]  ↻
────────────────┬─────────────────────────────┬─────────────────────
 BRANCHES       │ COMMITS                     │ CHANGED FILES
 HEAD           │ Subject       Author  Date  │ ▾ frontend
 ▾ Local        │ fix: …        Gink    Today │   M FilePicker…
   develop      │ feat: …       Gink    Oct 25│   A HistoryPane…
   release      │ chore: …      Gink    Oct 24│ ▾ internal
 ▾ Remote       │                             │   M git_history.go
   ▾ origin     │                             ├─────────────────────
     develop    │                             │ COMMIT DETAILS
     release    │                             │ Full message
                │                             │ Author · date
                │ Load more                   │ a1b2c3d [Copy]
────────────────┴─────────────────────────────┴─────────────────────
```

- Initial horizontal proportions: branches 18%, commits 52%, files/details
  30%. Splitters are resizable; remember sizes within sensible minimums.
- Changed files and commit details have independently scrollable areas and
  an adjustable split. Show full message, author/committer metadata, hash,
  parent hashes, and comparison basis.
- Selecting a changed file opens a resizable **bottom diff section**, initially
  about half the body height. Keep branches, selected commit, and changed
  files visible above it. Closing the section restores the browser height.
- Reuse existing diff rendering, syntax highlighting, and preview/retained-tab
  conventions. Double-click retains a historical tab. Historical tabs identify
  the revision and repository and never replace a dirty working-file tab.
- History owns the bottom section's historical tab selection; switching back
  to Files restores the working-file viewer/selection. Retained history tabs
  survive mode switches within the same workspace/host context. Do not mount
  two viewer instances sharing one mutable active selection.
- At narrow widths collapse the branch tree into a selector and adapt the
  files/details area to a disclosure rather than crushing the commit list.
  Final breakpoints and minimum sizes are established in GH-12 and verified
  in a real WebView, not inferred from SSR tests.
- Preserve dialog close/lock shortcuts, keyboard access, focus restoration,
  host display, and presentation behavior. History must not offer misleading
  live-file Problems/code-check actions for a historical snapshot.

## Repository and branch behavior

Automatically select the containing repository for a Git workspace. For a
non-Git parent, show validated discovered repositories with relative-path
labels; automatically choose the only repository when there is one. With
multiple repositories, require a selection unless a still-valid selection
already exists for that workspace/host.

Reuse bounded filesystem-walk discovery and its skipped-directory conventions.
Recognize `.git` directories and files (worktrees and submodules), do not
follow directory symlinks, and expose discovery truncation. An invalid marker
must not fall through to an outer repository. No new exhaustive scan of nested
repositories inside an already Git-backed workspace is required in v1.

Default to HEAD, including detached HEAD. Group branches under Local and
Remote, with remotes grouped by name. Keep remote branches even when a matching
local branch exists. Unborn repositories show an empty history state.

For a workspace inside a repository subdirectory, retain that authorization
boundary: show commits affecting the scope and only changed content inside
it. Label scoped history. Cross-boundary renames must not expose outside-scope
paths or blobs.

## Backend contract and safeguards

Names below are provisional until GH-01 establishes the shared contract:

| Operation | Purpose |
|---|---|
| `GitHistoryRepositories(dir)` | Valid repository choices and truncation |
| `GitHistoryBranches(dir, repo)` | HEAD state and full local/remote refs |
| `GitHistoryList(target, ref, query, cursor, limit)` | Pinned tip, summaries, continuation |
| `GitHistoryCommit(target, tip, commit)` | Metadata, parents, bounded changed files |
| `ReadGitHistoryDiff(target, tip, commit, change)` | Immutable content, comparison basis, hunks |

Every operation preserves the exactly authorized workspace `dir`; `repo` is a
separate relative selector. Keep `Service.knownDirectory()` unchanged. Resolve
and revalidate selected child repositories server-side on every request,
including real-path containment and actual worktree-root identity. Cached
listings are hints, never access grants. Reject absolute/traversal/metadata
selectors and symlink escapes.

Accept HEAD or validated refs under `refs/heads/` and `refs/remotes/`, resolving
them to full commit OIDs. Pin subsequent pages to the tip, scope, query, and
deterministic traversal order. Validate cursor parameters, OID format/type,
and commit membership; do not pass unrestricted revisions or options to Git.
Account for SHA-1 and SHA-256 repositories where the installed Git supports
them.

Search backend history, not just loaded rows: literal case-insensitive
message/author matching and hash-prefix lookup. Bound query length, page size,
retained results, traversal, output, rename detection, and subprocess duration.
GH-01 fixes numeric limits and explicit limit/timeout response semantics.

Diff ordinary commits against their parent, root commits against an empty tree
without writing objects, and merge commits against first parent, clearly
labeled. Parse NUL-delimited change records. Read verified tree/blob objects;
never depend on the working tree, index, or present-day path existence. Bound
both blob sides using existing text/binary limits. Historical symlinks and
gitlinks are explanatory metadata, not navigable filesystem targets.

Reuse the bounded Git runner and short-lived common-directory locking, with a
history-specific timeout. Disable external diffs, textconv, paging, optional
locks, and lazy fetch. Missing objects/partial or shallow history must produce
honest coverage or error states, not network access.

## Transport, state, and viewer architecture

Follow App → optional backend → Engine/RPC for local, WSL, and SSH. Add an
independent `gitHistory` capability; missing capabilities/bindings show an
unsupported/update message. Offline remotes must never fall back to local Git.

Use a dedicated history store with workspace/host/context identity and separate
request generations for discovery, refs, pages, details, and diff reads. Reject
late responses after selection, query, workspace, or connection changes.
Relevant Changes writes invalidate matching lists/refs; refresh repins history
explicitly without mutating already-open snapshots.

Current tabs are keyed by path. Introduce stable identity separate from display
path, plus explicit immutable history source metadata. Audit all path-keyed tab
operations: selection, close/keep, previews, context menus, draft recovery,
refresh, navigation, rename/delete, and document lifecycle. Historical identity
includes context, repository, commit, comparison parent, and changed path.
History tabs must never save, run live-file diagnostics/blame, emit live-document
operations, reload from disk, or offer working-file mutation actions.

## Reuse and primary integration points

- `internal/files/files.go`: `ListPaths`, bounded walk, containment helpers.
- `internal/files/nested_status.go`: invalid-marker/outer-repo safeguards.
- `internal/files/git_branches.go`: branch enumeration/parsing concepts;
  switch-oriented remote deduplication must not carry into history.
- `internal/files/git_write.go`: bounded runner and common-directory locking.
- `internal/files/git_diff.go`: `readGitBlob`, content limits and hunk model;
  working-file preflight is unsuitable for deleted historical paths.
- New `internal/files/git_history*.go`, engine/app/RPC counterparts;
  `internal/engine/backend.go`, `internal/rpc/types.go` and `server.go`.
- `frontend/src/lib/api.ts`, `types.ts`, mocks and generated desktop bindings.
- `frontend/src/lib/stores/files.svelte.ts`, `fileActions.svelte.ts`,
  `gitChanges.svelte.ts`, and new `gitHistory.svelte.ts`.
- `GoToFileDialog.svelte`, `go-to-file/FilePicker.svelte`, `FileTabs.svelte`,
  `FileViewer.svelte`, `contentSearch.svelte.ts`, and new history components.

Recheck APIs and ownership before implementation; these are reuse points, not
instructions to duplicate infrastructure.

## Delivery and verification

See the [task backlog](../tasks/git-history.md) for dependencies and acceptance
criteria. All tasks start unimplemented. Contract/backend work precedes
transport; revision-safe identity precedes viewer integration. Layout work can
use contract fixtures while backend integration proceeds.

Run focused affected tests, repeated tests and race detection, then broad checks:

```sh
go test -race ./internal/files ./internal/engine ./internal/rpc ./internal/app ./internal/remote
npm --prefix frontend test
npm --prefix frontend run check
npm --prefix frontend run build
make test
```

Use the project run workflow for real-app verification. Test repository roots,
scoped subdirectories and non-Git parents, branch browsing without checkout,
root/merge/rename/deletion diffs, multiple historical revisions beside a dirty
file, splitters and narrow layouts, mode switches, and disconnects during reads.
Exercise SSH integration (`make test-ssh`) when available and record local,
SSH, WSL and Windows/macOS validation separately. Cross-build/vet is not native
Windows runtime testing. Use isolated Git config, Windows-legal fixtures,
explicit async barriers, and bounded waits. Planned or blocked QA is not a pass.

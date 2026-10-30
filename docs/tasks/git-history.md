# Git history — implementation tasks

- Plan: [Git commit history](../plan/git-history.md).
- Progress: **17/18 complete**. GH-01…GH-17 are complete; GH-18 is blocked on native UI access.
- Scope: IntelliJ-inspired History layout inside Go to File. Selection is read-only;
  the 2026-10-27 extension adds tags, explicit confirmed ref actions and saved-disk
  working-tree comparison (see [contract](../plan/git-history-contract.md)).
- Extension automated coverage includes temporary-repository mutation safety,
  original-workspace authorization, transport/capability gating, comparison
  staleness, frontend orchestration and SSR. Native menu/keyboard/theme/narrow
  layout and Windows/SSH/WSL interaction QA remain pending under GH-18.
- Estimates include focused tests, not elapsed-time commitments. Each package
  is at most four developer hours; split before starting if it grows beyond
  that, rather than dropping acceptance criteria.
- Follow repository Definition of Done. Record implementation, automated tests,
  native UI QA, and unavailable platform coverage separately.

## Task map

| Task | Deliverable | Dependencies | Estimate |
|---|---|---|---|
| GH-01 | ✅ Shared contracts, limits, and fixtures | — | 3 h |
| GH-02 | ✅ Authorized repository resolution and discovery | GH-01 | 4 h |
| GH-03 | ✅ Read-only runner and history branch enumeration | GH-01, GH-02 | 3 h |
| GH-04 | ✅ Pinned commit pagination and search | GH-03 | 4 h |
| GH-05 | ✅ Commit metadata and scoped changed files | GH-04 | 4 h |
| GH-06 | ✅ Immutable historical content and diffs | GH-05 | 4 h |
| GH-07 | ✅ Engine authorization and app host routing | GH-02…GH-06 | 3 h |
| GH-08 | ✅ RPC capability, methods, and compatibility | GH-07 | 4 h |
| GH-09 | ✅ Frontend API, types, mocks, desktop bindings | GH-01, GH-08 | 3 h |
| GH-10 | ✅ Stable tab identity separate from path | GH-01 | 4 h |
| GH-11 | Immutable history sources and viewer safeguards | GH-06, GH-09, GH-10 | 4 h |
| GH-12 | Dedicated History layout, splitters, mode lifecycle | GH-01, GH-10 | 4 h |
| GH-13 | History store, pagination, and stale-response guards | GH-09 | 4 h |
| GH-14 | Repository/branch navigation and commit list UI | GH-12, GH-13 | 4 h |
| GH-15 | Details/files panel and bottom diff integration | GH-11, GH-14 | 4 h |
| GH-16 | Refresh/invalidation and lifecycle regressions | GH-15 | 3 h |
| GH-17 | Security, limits, and automated integration gate | GH-16 | 4 h |
| GH-18 | Native UI and cross-platform/remote QA gate | GH-17 | 4 h |
| **Total** | **18 tasks** | | **67 h** |

Dependency ranges include all tasks between their endpoints. Do not treat the
map as authorization for multi-agent implementation. GH-10 and backend work can
proceed independently after GH-01, but shared viewer/store edits need a single
owner. GH-12 can use fixtures; production integration must wait for its listed
dependencies. Do not claim remote support validated merely from mocks.

## GH-01 — Shared contracts, limits, and fixtures

**Status:** complete (2026-10-26) · **Estimate:** 3 h · **Dependencies:** none

Establish the source-of-truth request/response contract before implementation.

- Define workspace/repository target, full refs, HEAD/unborn states, commit
  summaries, pinned continuation, detail/change identities, scope metadata,
  historical diff source, errors, and capability semantics.
- Fix numeric limits for query length, page size, discovery, commits scanned,
  retained rows, changed files, rename detection, output and timeouts.
- Specify literal search/hash-prefix behavior, deterministic traversal,
  continuation validation, merge/root comparisons, and cross-scope renames.
- Define snapshot-expired/missing-object/unsupported/limit states, avoiding a
  false “no results” on incomplete traversal.
- Add sanitized contract fixtures for direct/parent/scoped repositories and
  text, deletion, rename, merge, binary and unsupported responses.

**Likely files:** new `docs/plan/git-history-contract.md`, fixture locations used
by existing backend/frontend Git tests; update the design when names settle.

**Done when:** contracts and fixtures agree, round-trip tests cover representative
payloads, and later tasks do not have to invent incompatible field semantics.

## GH-02 — Authorized repository resolution and discovery

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-01

- Preserve the original known workspace and resolve a separate relative child
  selector. Revalidate real-path containment and actual worktree identity.
- Reuse bounded non-Git walk discovery and skip conventions. Return sorted
  choices and truncation; recognize `.git` directories/files.
- Resolve a containing repo plus authorized scope for subdirectory workspaces.
- Refuse forged absolute/traversal/metadata targets, symlink escapes, and
  invalid markers falling through to an outer repository. No recursive scan
  of nested repos inside an already Git-backed workspace is needed in v1.

**Likely files:** new `internal/files/git_history_repositories.go` and tests;
reuse `files.go` and `nested_status.go` helpers where suitable.

**Done when:** tests cover repo roots, scoped directories, multiple deep child
repos, worktrees, submodules, no repos, invalid markers, truncation, and escapes.
Discovery is never an authorization grant.

## GH-03 — Read-only runner and history branches

**Status:** complete (2026-10-26) · **Estimate:** 3 h · **Dependencies:** GH-01, GH-02

- Reuse bounded Git execution/common-dir locking with a short history timeout.
- Disable lazy fetch, optional locks, prompts, paging, external diffs/textconv.
  No network or object writes to compute an empty tree.
- Enumerate HEAD, local and already-fetched remote refs without the Changes
  branch-switch parser's remote deduplication; retain remote grouping identity.
- Validate full refs and object format; expose unborn/detached states and limits.

**Likely files:** new `internal/files/git_history.go`, targeted runner/branch
helper extraction only if needed.

**Done when:** tests cover detached/unborn, matching local/remote names, malformed
refs, timeout/cancellation and runner environment; existing write behavior stays
unchanged and history produces no Git state changes.

## GH-04 — Pinned commit pagination and search

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-03

- Resolve the selected ref once; paginate deterministically from its full tip.
- Validate continuation identity against repository/scope/query/order/limits.
- Search backend history using the agreed literal message/author semantics and
  hash-prefix lookup. Do not filter only the rows already loaded in the UI.
- Bound traversal, output and runtime; surface incomplete searches honestly.
- Keep scoped history inside the workspace and handle missing/shallow objects.

**Likely files:** `internal/files/git_history*.go` and tests.

**Done when:** fixtures prove no skipped/duplicate pages for a stable snapshot,
ref movement cannot mix snapshots, and special queries, forged cursors,
missing objects, no matches, scope and limits behave as contracted.

## GH-05 — Commit details and scoped changed files

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-04

- Validate commit type/membership in the selected history.
- Return full message, author/committer metadata, parents and comparison basis.
- Compare ordinary commits to parent, merges to first parent, roots to empty
  tree. Parse NUL-delimited changes with bounded rename detection.
- Enforce scope on both rename sides; never reveal outside-scope paths.
- Return modes/status and explicit change-list truncation.

**Likely files:** `internal/files/git_history_commit.go` and tests.

**Done when:** tests cover root/merge commits, rename/copy policy, deletes,
unusual filenames, cross-boundary renames, invalid OIDs, membership and limits.

## GH-06 — Immutable historical content and diffs

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-05

- Resolve requested change entries from verified trees; read blobs by verified
  identity rather than using index/disk paths or unrestricted revision syntax.
- Reuse content/binary/5 MiB limits and hunk rendering contracts; inspect both
  sides before generating a bounded patch.
- Handle deleted/renamed paths whose disk ancestors no longer exist.
- Return non-navigable symlink/gitlink states and correct binary/large notices.

**Likely files:** new `internal/files/git_history_diff.go`; reuse appropriate
parts of `git_diff.go` without working-file preflight.

**Done when:** root/merge/deletion/rename diffs are correct, working-tree edits
do not alter results, unsafe change identities are rejected, both-side limits
hold, and neither external diff nor textconv executes.

## GH-07 — Engine authorization and app routing

**Status:** complete (2026-10-26) · **Estimate:** 3 h · **Dependencies:** GH-02…GH-06

- Add an optional history backend interface and Engine/Service wrappers.
- Keep `knownDirectory` unchanged and require it on every operation.
- Add app bindings using one consistent route snapshot per call.
- Preserve remote-offline errors with no local filesystem/Git fallback.

**Likely files:** `internal/engine/backend.go`, new engine/app
`git_history.go` and matching tests.

**Done when:** tests cover every method's unknown-directory rejection, allowed
child selection under an authorized parent, local/remote forwarding, and
host-switch/offline behavior without authorization widening.

## GH-08 — RPC and compatibility

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-07

- Add independent `gitHistory` capability, client methods, server dispatch,
  validated payloads and typed error propagation.
- Follow existing cancellation/timeout conventions and bounded wire replies.
- Older peers reject unsupported history cleanly; existing Git APIs still work.

**Likely files:** `internal/rpc/types.go`, `server.go`, new `git_history.go`
and RPC tests; capability normalization/forwarders as required.

**Done when:** round-trip tests cover discovery through diff, capability-off and
old-peer cases, malformed requests, authorization errors, cancellation and
missing objects. No silent local fallback is introduced.

## GH-09 — Frontend API and bindings

**Status:** complete (2026-10-26) · **Estimate:** 3 h · **Dependencies:** GH-01, GH-08

- Add shared TypeScript types, desktop API forwarding, mocks, and history
  capability handling. Regenerate tracked bindings through the project process.
- Keep full target/snapshot identity in requests and existing epoch guards.
- Map missing old bindings to a clear unsupported/update state.

**Likely files:** `frontend/src/lib/api.ts`, `types.ts`, existing mock and
binding locations, new frontend API tests.

**Done when:** tests prove argument/result forwarding, old bindings, unsupported
peers, stale epochs, and host paths are not rewritten in the frontend.

## GH-10 — Stable tab identity

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-01

- Separate stable tab ID from actual/display path; preserve live-file behavior.
- Audit select/open/preview/close/keep, context menus, draft persistence,
  navigation, reload, rename/delete and document observers for path-only keys.
- Establish explicit working versus history selection ownership so layout
  switches do not overwrite the working-file active tab.

**Likely files:** `files.svelte.ts`, `fileActions.svelte.ts`, `FileTabs.svelte`,
viewer/navigation helpers and affected tests.

**Done when:** all current tab/draft tests pass; synthetic same-path revisions
can coexist; closing/selecting one ID cannot act on another; observers still
receive real paths, never synthetic IDs. Split this task before coding if the
audited caller set exceeds the estimate.

## GH-11 — Immutable sources and viewer safeguards

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-06, GH-09, GH-10

- Add explicit history source identity with repo/commit/base/old-new paths.
- Load immutable diffs through the history API and reuse viewer hunks.
- Prevent editing/save, live-document/diagnostics/blame activity, disk reload,
  working-file actions and misleading diff-basis switches for history tabs.
- Preserve dirty working tabs and mark revision/repository clearly in labels.

**Likely files:** `files.svelte.ts`, `FileViewer.svelte`, `FileTabs.svelte`,
action/document lifecycle helpers and tests.

**Done when:** a dirty live file plus two revisions of the same path coexist;
late reads cannot cross tabs; history invokes no save or live-document APIs;
all text/deleted/binary/large/symlink/gitlink states render correctly.

## GH-12 — Dedicated layout and splitters

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-01, GH-10

- Make History a dedicated body mode with persistent mode navigation, rather
  than squeezing it into the existing left picker.
- Build branch/list/right-detail regions and a closable bottom diff region.
- Add adjustable splits with remembered sizes, minimums and keyboard access;
  start at 18/52/30 horizontally and roughly half-height diff when opened.
- Define narrow-window collapse behavior and ensure one viewer owns each
  active selection. Keep working tabs/drafts mounted or safely preserved.

**Likely files:** `GoToFileDialog.svelte`, `FilePicker.svelte`,
`contentSearch.svelte.ts`, new `HistoryLayout.svelte`, shared splitter helpers.

**Done when:** fixture-driven component tests cover mode switches, diff collapse,
size bounds and responsive states; Files/Search/Changes remain unchanged.
Native splitter/focus/layout verification remains GH-18, not an SSR claim.

## GH-13 — History store and request lifecycle

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-09

- Add independent state for repository choices, refs, pinned pages, selected
  commit/details, errors and continuation limits.
- Bind requests to workspace/host/context and per-operation generations.
- Debounce search; clear dependent state on repo/ref/query changes; reject
  obsolete pages/details and cap retained results.
- Support refresh/repin without modifying retained immutable snapshots.

**Likely files:** new `frontend/src/lib/stores/gitHistory.svelte.ts` and store
tests, reusing `gitChanges.svelte.ts` stale-request patterns.

**Done when:** deterministic tests cover out-of-order replies, pagination,
query changes, host/repo/ref switches, reconnect, old capability, empty/error
states and retained-result limits without arbitrary sleeps.

## GH-14 — Repository/branch navigation and commit list

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-12, GH-13

- Add repository selector and scoped-history label; require selection for
  multiple repos unless a prior valid selection exists.
- Render HEAD/Local/Remote branch tree and collapsed narrow-width selector.
- Render search, aligned subject/author/date rows, selection and Load more.
- Support keyboard navigation and honest loading/empty/error/truncation states.

**Likely files:** new history components under `components/common/go-to-file/`
and their component tests.

**Done when:** browsing refs never invokes switch/fetch; commit rows stay usable
with long text, multiple remotes, pagination, empty/unborn and unsupported states.

## GH-15 — Details, changed files, and bottom diff

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-11, GH-14

- Add changed-file tree above full commit details, with independent scrolling.
- Show rename origins, change status, copy hash action, parent hashes and
  explicit first-parent comparison labels for merges.
- Wire click to bottom diff preview, double-click to retain, and close to
  restore browser height. Maintain separate working/history tab selection.
- Keep commit context visible and provide accessible file-tree navigation.

**Likely files:** history detail/file components, history layout/store,
`FileTabs.svelte`/`FileViewer.svelte` integration and tests.

**Done when:** end-to-end component tests select repo → ref → commit → file,
retain multiple revisions, close/reopen the bottom region, switch modes with
a dirty file, and handle binary/large/deleted entries without mutation actions.

## GH-16 — Refresh and lifecycle integration

**Status:** complete (2026-10-26) · **Estimate:** 3 h · **Dependencies:** GH-15

- Invalidate matching refs/history after commit/amend/branch switch/fetch/pull
  from Changes, following existing invalidation mechanisms.
- Refresh explicitly repins pages; already-open commit diffs remain immutable.
- Define dialog reopen, focus refresh, context replacement, disconnect and
  reconnect behavior without request storms or stale selection restoration.
- Keep Problems/code-check UI tied to working files, not historical snapshots.

**Likely files:** `gitHistory.svelte.ts`, `gitChanges.svelte.ts`, dialog and
file/document lifecycle integration points.

**Done when:** tests cover writes while history is open, ref movement during
pagination, mode switches, dialog reopen and delayed replies after disconnect;
working-file lifecycle and snapshot lifecycle remain distinct.

## GH-17 — Automated security and integration gate

**Status:** complete (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-16

- Close cross-layer gaps in authorization, scope, hostile selectors, refs/OIDs,
  cursors, unusual paths, output limits and missing-object handling.
- Verify no network/lazy fetch, external diff/textconv, or refs/index/worktree
  writes; verify history never enters edit/live-document paths.
- Cover discovery-to-diff transport and UI integration, legacy servers, remote
  offline and stale replies. Repeat focused tests with race detection.
- Run full backend/frontend checks listed in the plan; record actual outcomes.

**Done when:** focused and broad automated checks pass, or remaining failures
are explicitly recorded as blockers. Isolate Git config and checkout line
endings; use Windows-legal fixtures, proper path encoding and async barriers.
Static/cross-build checks are labeled as such, not native runtime validation.

## GH-18 — Native UI and platform/remote QA

**Status:** blocked / partial (2026-10-26) · **Estimate:** 4 h · **Dependencies:** GH-17

- Use the project run workflow to exercise the real app and record a QA matrix.
- Check layout proportions, dragging/keyboard splitters, narrow windows,
  scrolling, focus, dialog shortcuts and mode switching.
- Browse a repo, scoped subdir and multi-repo parent; inspect root/merge,
  rename/deletion, binary/large entries and multiple revisions next to a draft.
- Verify branch selection leaves checkout unchanged; refresh and disconnect
  during loads, then reconnect without stale local/remote data mixing.
- Exercise available native macOS/Windows and local/SSH/WSL routes; run opt-in
  SSH integration where available. Record unsupported environments as untested.

**Likely files:** new `docs/plan/git-history-qa.md`, task progress updates.

**Done when:** actual evidence and remaining blockers are recorded per platform
and route. Do not mark this task complete solely from SSR/mocks or cross-vet.
If environment setup or follow-up fixes exceed this package, split additional
work and keep the corresponding gate pending.

### GH-01 evidence

Shared Go types/constants and typed JSON round-trip fixtures are implemented.
The authoritative contract is [v1](../plan/git-history-contract.md). Focused
validation: `go test ./internal/files -run TestGitHistoryContractRoundTrip -count=1`.
No history operations or UI are implemented by this task.

### GH-02 evidence

Implemented bounded discovery and canonical target/scope validation with direct,
parent, worktree/submodule, invalid-marker, escape and limit tests. Invalid child
candidates are omitted with `truncated=true`, preserving healthy siblings.
Repeated history tests passed under race detection (`-count=2`); Windows vet
passed (static coverage only, not native runtime).

### GH-03 evidence

Implemented bounded coordinated history reads, complete local/remote ref
enumeration, unborn/detached/shallow states and SHA-1/SHA-256 validation.
Focused branch/runner/object-format tests passed twice, full files tests passed,
and Windows vet passed after an intermediate in-progress unused-import failure
was resolved. No native Windows execution is claimed.

### GH-04 evidence

Pinned pagination, strict bound cursors, literal search/hash lookup, framed raw
metadata parsing and merge-aware scoped traversal are implemented. Tests cover
ref movement, invalid continuation, searches across chunks and empty final
continuations. Full files regression and native vet passed.

### GH-05 evidence

Validated commit membership, full metadata, first-parent/root comparisons and
bounded raw changed-file parsing are implemented. Tests cover merge metadata,
renames/deletions, scope-crossing renames, unusual paths and entry truncation.
History race suite and Windows static vet passed; native Windows untested.

### GH-06 evidence

Immutable tree/blob reads and bounded Git blob diffs are implemented, independent
of the index and working files. Tests cover root additions, modifications,
renames, deletions with missing disk ancestors, binary blobs, oversized old
sides, forged changes and disabled external diff/textconv. History race suite
and Windows static vet passed; no native Windows claim.

### GH-07 evidence

Optional history backend, exact known-directory checks on every Engine/Service
operation and single-snapshot app forwarding are implemented. Focused engine/app
GitHistory tests pass, covering unauthorized requests, child repositories,
unsupported backends and offline routes without local fallback.

### GH-10 evidence

Stable tab IDs, working/history selection ownership, path-safe actions and
observer/navigation integration are implemented. Synthetic same-path revision
tests cover dirty drafts, independent previews/selections and close/rename/delete
isolation. Frontend suite: 777 passing; check: zero errors, existing tsconfig
warning. Implemented independently after GH-01 while transport tasks proceeded.

### GH-08 evidence

All five RPC operations, independent capability, local/CLI advertisement,
validated payloads and typed authorization errors are implemented. Real-engine
framed RPC tests cover discovery through immutable diff and unauthorized reads.
Package tests passed, repeated focused/race tests passed and Windows static
vet passed. Native Windows and manual UI remain untested.

### GH-09 evidence

Frontend history types/API, missing-binding compatibility, capability typing and
read-only mock history are implemented; forwarding/compatibility/mock tests and
full frontend regression/check pass. Desktop bindings regenerated successfully
with `go build -tags desktop,bindings` and the resulting binding executable
from the repository root; generated wailsjs files are ignored by this project.
The stock generator was tried first but cannot locate the configured entrypoint.

### GH-12 evidence

History is a fourth Go to File mode with its own body: persistent mode
navigation, toolbar, branch/commit/detail regions and a closable bottom diff
hosting the existing viewer. Keyboard-accessible splitters persist bounded
sizes (branches 12–30%, details 22–42%, diff 25–70%; defaults 18/30/50).
Container queries hide branches below 850 px and stack details below 580 px.
Selection ownership follows the mode; working drafts stay in the store and the
Files/Search/Changes body is unchanged. SSR tests cover regions, bounds and
diff collapse; full frontend suite and check pass. Native splitter, focus and
layout behaviour remain GH-18.

### GH-11 evidence

History tabs carry a frozen source (dir, repo, tip, commit, base, old/new path)
whose identity is the tab ID, load only through `readGitHistoryDiff`, and drop
late, mismatched or reconnect-interrupted replies. They are excluded from
drafts, edit/save/discard, refresh, live documents, blame, diagnostics, code
navigation and working diff-basis controls. Viewer and tabs label repository,
short commit and comparison, and render deleted, binary, too-large, symlink and
submodule states. Tests cover a dirty working tab beside two revisions of the
same path; full frontend suite and check pass.

### GH-13 evidence

`gitHistory.svelte.ts` owns repository, ref, page and detail state with separate
request generations plus a workspace/host/connection/epoch identity, so late
replies cannot overwrite newer selections. Repository choice is remembered per
workspace and host but only reused when rediscovered; one repository is chosen
automatically. Load-more never repins, refuses moved tips or non-advancing
cursors, dedupes rows and stops at 2,000. Over-long queries, offline hosts and
hosts without the `gitHistory` capability make no calls; local capabilities now
advertise history and an old desktop binding fails explicitly in the API.
Connection changes currently clear loaded history; preserving the view across
reconnects is GH-16 scope. 18 store tests plus full suite and check pass.

### GH-14 evidence

The History toolbar offers an explicit repository picker (with scope label),
debounced message/author/OID search and Refresh; the branch pane shows HEAD,
Local and per-remote groups and collapses to a selector below 850 px. Commit
rows show subject, author, date and short OID with roving keyboard focus and
Load more. Empty, loading, error, unborn, shallow, traversal/display limit,
truncated and unsupported states are explicit; no switch/fetch action exists.
SSR component tests, full suite, check and production build pass;
keyboard and debounce behaviour in a real WebView remain GH-18.

### GH-15 evidence

The independently scrolling changed-file tree groups full directory paths with
foldable headings, rename origins, status labels and keyboard walking/folding.
Commit metadata includes author/committer timestamps, full escaped message,
parents, copy hash and explicit first-parent merge comparison. Click previews,
double-click or Ctrl/Cmd+Enter retains the pinned diff through GH-11; closing
restores browser height and Show diff reopens retained tabs without changing
working selection/drafts. The current change highlight requires matching repo
and commit. SSR details tests and existing immutable multi-revision/store/viewer
tests pass, as do the frontend suite/check/build. Real WebView end-to-end clicks,
clipboard, focus and tree/splitter interaction remain GH-18 rather than being
claimed as SSR coverage.

### GH-16 evidence

Commit, amend, switch and every sync (including failed/cancelled operations
and push tracking refs) lazily invalidate history for the write's host beside
existing listing invalidation; stage/unstage/discard preserve the selection.
Invalidation retires all pending generations and bumps a revision observed by
the visible History effect, so the next first page repins. Opening the dialog
invalidates once per opening (not per mode switch, and not during a load);
window focus refreshes only loaded idle history, avoiding storms against
offline or failing hosts. Disconnect clears pinned cursors/details; reconnect
rediscovers once and never restores a stale selected commit. Open history tabs
are untouched. Problems stays hidden in History and historical viewers
suppress diagnostics (GH-11/12). Store/Changes tests, full suite (814),
check and build pass; real focus/reopen behaviour remains GH-18.

### GH-17 evidence

Audit fixes: non-UTF-8 changed paths are omitted and disclosed instead of being
aliased by JSON replacement characters; historical blob diffs force standard
indicators and no word diff; every history Git process denies all transports;
unborn detection no longer needs Git 2.43 (`show-ref --exists`) and ignores
nested refs below the branch name. New security tests snapshot every repository
file (HEAD, refs, index, objects, dirty/staged/untracked worktree) across a full
discovery→refs→pages→detail→diff run, and cover missing promisor blobs offline,
hostile refs, unusual round-trip paths, shallow missing parents and output caps
with isolated Git config. Results on macOS (Git 2.54):
`go test ./internal/files ./internal/engine ./internal/app ./internal/rpc ./cmd/...`
passed; `go test -race -count=3 … -run History` passed; frontend suite (814) and
check passed. `GOOS=windows go vet` passed as static validation only; the
non-UTF-8 filesystem fixture runs on Linux only (macOS rejects it), with a raw
parser test on every platform. No native Windows, WSL or SSH execution yet.

### GH-18 partial evidence and blocker

See [QA matrix](../plan/git-history-qa.md). Native macOS backend race gate and
Docker SSH integration pass (SSH needs `TMPDIR=/tmp` for macOS socket length).
Frontend build/tests/check pass. Desktop compile succeeds with the SDK's
UniformTypeIdentifiers framework linked; isolated QA instances launched, but
computer-use tooling is unavailable and no screenshot or interactive acceptance
was possible. Windows/WSL and real-host history UI remain untested. Broad lint
fails on unrelated existing findings; history-specific lint findings are fixed.
GH-18 stays pending rather than treating process launch or SSR as native QA.

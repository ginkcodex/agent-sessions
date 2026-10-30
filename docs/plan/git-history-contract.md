# Git history contract v1

Source types: `internal/files/git_history_types.go`. Capability: `gitHistory`.
Browsing and comparison operations are read-only; explicit ref actions are separately
negotiated through `gitHistoryActions` (writes) and `gitHistoryCompare` (comparison).
All operations carry an exactly authorized workspace `dir`
and relative `repo` (`.` means its containing repository). Paths in returned
changes/content are relative to that workspace, or the selected child repo.
`scope` is a display-only repository-relative prefix for a scoped workspace.
No operation accepts an absolute repository selector or arbitrary revision.

## Methods

- `GitHistoryRepositories(dir)` → `GitHistoryRepositoryList`.
- `GitHistoryBranches(target)` → `GitHistoryRefList` (despite method name, full
  refs are returned; symbolic remote HEAD aliases are omitted).
- `GitHistoryList(request)` → `GitHistoryPage`.
- `GitHistoryCommit(request)` → `GitHistoryCommitDetail`.
- `ReadGitHistoryDiff(request)` → `GitHistoryDiff`.

Go, desktop and RPC wrappers retain these names subject to existing transport
casing conventions. Optional backend support is advertised independently.

## Explicit ref actions and saved-disk comparison

- `GitHistoryRefAction(request, actionID, host)` supports checkout, merge, push
  and delete. Original `dir` authorization, GitWrite/SaveFile (plus GitSync for
  push), selected raw OID and symbolic/OID HEAD are revalidated under the shared
  repository lock. Cancellation uses `CancelGitSync(actionID)`.
- Checkout is nonforced; tags detach HEAD. Merge requires a clean attached HEAD
  and preserves conflicts. Push sends only the selected local branch/tag to
  origin without force or upstream changes. Branch deletion uses safe `-d`;
  tag/remote-tracking deletion is local-only with expected-OID protection.
- Scoped checkout/merge requires whole-repository acknowledgement without
  relaxing protected-path policy. Drafts and in-progress operations are guarded.
- `GitHistoryCompare(request, host)` and `ReadGitHistoryCompareDiff(request,
  host)` compare a pinned commit to saved disk, including nonignored untracked
  additions. An opaque token binds the displayed list to bounded disk state;
  incompatible changes require Refresh. Unsaved editor buffers are excluded.
- Comparison tabs use a separate mutable identity from immutable commit-parent
  tabs. Selection alone never checks out, merges, pushes or deletes anything.
- Older servers omit the optional capabilities; ordinary history stays usable.

## Limits

| Bound | Value |
|---|---|
| Default / maximum page size | 50 / 100 |
| Search UTF-8 bytes | 256 |
| Scanned commits per pinned traversal | 100,000 |
| Repository choices / refs | 500 / 500 |
| Retained frontend commit rows | 2,000 |
| Changed entries / rename detection candidates | 5,000 / 500 |
| Blob bytes per side | existing `MaxReadBytes` (5 MiB) |
| Git stdout / stderr | existing runner 16 MiB / bounded error output |
| Operation including lock wait | 20 seconds |
| Cursor encoded bytes | 4,096 |

Reject negative or excessive explicit page limits; zero uses the default.
Large messages/output fail explicitly rather than silently altering content.
Discovery, refs and changed files expose `truncated`. Discovery also sets it
when broken or stale child candidates are omitted, preserving healthy choices;
selecting an invalid target directly remains an error. A traversal reaching the
scan cap exposes `limited`; this is not equivalent to an exhaustive no-match.
Timeout/cancellation is an error, not an empty result.

## Refs, objects and continuation

An initial ref is HEAD or a checked full `refs/heads/…`, `refs/remotes/…` or
`refs/tags/…`. Ref listings return `kind`, raw direct-object `oid`, and peeled
`commit`; a non-commit tag has an empty `commit` and cannot be browsed. `headRef`
retains symbolic HEAD identity, including unborn HEAD.
Resolve it to a full commit OID. Object IDs must have the repository's expected
SHA-1 or SHA-256 length and hexadecimal syntax, and refer to the required type.
Commit detail/diff requests include a pinned tip and commit; prove ancestry
before returning objects. No option/revision expression supplied by a caller
is passed to Git unchecked.

Traverse `--topo-order` from the pinned tip. A versioned base64url JSON cursor
binds the canonical target, scope, tip, literal query, ref, page size and next
scan offset. It is a validated continuation, not an authorization token. The
server revalidates target and objects on every call, even with a cursor. Ref
movement does not affect a continuation. Reject malformed, oversized,
mismatched or out-of-range cursors. Missing pinned objects require refreshing
history; do not substitute the current tip.

Search is a case-insensitive literal substring in full message or author name/
email, OR an ASCII hexadecimal prefix of the full commit OID (minimum four
characters). No regular expressions. Empty query matches all. Scan in bounded
chunks so selective searches cannot require unbounded output. Scope filtering
uses the same traversal and first-parent comparison semantics as details;
merges affecting the scoped path must not disappear due to Git path-history
simplification. `shallow` means the available history may stop at a shallow
boundary; missing objects never trigger lazy fetch.

## Details, paths and diffs

Summaries include OID, subject, author timestamp and parent OIDs. Details add
committer, full message, comparison base, and changes. Base is first parent,
or empty for a true root. Merges explicitly use first parent, not combined diff.
At a shallow boundary with unavailable parents, details may return a missing-
object error instead of inventing an empty-tree diff.

Change statuses are A/M/D/R/T. Copies are treated as additions; bounded rename
detection may present unmatched renames as delete/add. NUL-delimited records
preserve tabs, newlines and non-ASCII paths. Validate literal paths and tree
membership; never interpolate them into revisions or shell commands. Returned
`path` identifies the new path, or old path for deletion. `oldPath` is present
only for an in-scope rename. Crossing the scope boundary becomes A or D with
no outside path/blob leakage. Outside changes are omitted. A diff request is
identified by the returned path; recompute/validate its matching change.

Text diffs return new-side `Content` and old-side removed lines using existing
`LineChange` semantics. Deletions have empty new content and deletion hunks.
Historical `Content` has no save revision and always has read-only edit info.
`kind` is text/binary/tooLarge/symlink/submodule; neither symlinks nor gitlinks
are followed. Check both sides before diffing. No current working-file or index
state determines historical content.

## Errors and compatibility

Reuse existing authorization, outside-root, unsupported, cancellation and
bounded Git error mechanisms across transports. Invalid target/ref/OID/cursor,
missing object and safety-limit failures must remain errors, never fabricated
empty results. Preserve useful bounded messages. Missing capability/binding is
an update/unsupported state. Offline remotes never route to local Git.

Git execution disables optional locks, lazy fetch, paging, external diff and
textconv, denies every transport (`GIT_ALLOW_PROTOCOL=`) and forces standard
diff indicators. It does not write objects to compute an empty tree, change
refs/index/worktree, invoke hooks or use the network. History requires Git
2.31 or newer (`rev-parse --path-format`); avoid newer-only plumbing such as
`show-ref --exists` so LTS SSH hosts keep working. Paths that are not valid
UTF-8 cannot round-trip through JSON, so they are omitted and the commit
details are marked truncated rather than aliased. Immutable tabs remain pinned when
lists refresh. Typed fixture coverage lives in `internal/files/git_history_types_test.go`;
unsupported is represented as a separate capability fixture, not a successful
history reply.

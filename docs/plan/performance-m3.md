# Performance Milestone 3 — Shared project index

Status: automated gate complete 2026-10-25, PI-01…10. Native platform/GUI checks remain unperformed. Prerequisites PF-10A/B
complete. [Roadmap](performance.md), [≤4h briefs](../tasks/performance-m3.md).

## Goal and evidence

Reduce repeated project enumeration and concurrent duplicate Git work without
weakening authorization or pretending cached paths/status are fresh. PF-03's
20,001-file listing took ~152 ms and six Git processes; the non-Git walk took
~14 ms. PF-11/12 retained frontend snapshots improve reopen latency but always
request another backend listing. PF-13/14 bounded directory/ignore work; they do
not retain a shared index. Historical evidence is in [perf.md](../perf.md).

## Contract and selected scope

Keep `files.List` as the uncached compatibility/oracle entry point. Internally
split immutable path enumeration from status acquisition, preserving `Listing`
fields, caps, relative paths, ignored paths, nested repositories and truncation.
A process-local manager owned by `engine.Service` shares bounded path snapshots
between listing callers. Remote hosts own their own manager; no local/remote
state is shared. Search retains its uncapped enumeration until a separate tested
uncapped index exists: a 20k listing must never cap search silently.

Keys are exact cleaned authorized roots plus enumeration options/schema. Do not
merge symlink/case aliases or derive subdirectory views from a truncated parent.
Each caller is authorized before lookup and again after waiting; catalog removal
cannot turn a cache into continued access. Cached data never replaces fresh
presence probes, file reads, save revisions or draft/conflict checks.

### Freshness and wire compatibility

Retained paths are **unverified**, not fresh. Existing `ListFiles` continues to
await a reconciliation and fresh status, preserving old clients and servers.
Add an optional capability-negotiated listing snapshot method for immediate
unverified paths, followed by the existing authoritative list request. The
snapshot envelope identifies root and generation and explicitly says unverified;
it never carries cached status as current. A server without the capability uses
PF-11's existing frontend snapshot and normal ListFiles. Do not infer support
from a version string or silently change old Listing semantics.

Paths and Git status have independent lifetimes. Every authoritative list gets
current status (including deleted paths) even if its path work is coalesced.
Within an overlapping flight status can be shared, but never reused across
completed requests merely because HEAD/index mtimes match. Non-Git nested status
retains current bounded/grace behavior and is not promoted to complete coverage.

### Reconciliation, not an unproven watcher promise

Every authoritative request reconciles paths. Concurrent same-key requests share
work; sequential requests still re-enumerate to detect terminal/agent edits,
untracked paths and ignore changes. This deliberately does not claim zero Git
processes on sequential refresh. Retention buys immediate unverified paths;
coalescing removes duplicate work. Full recursive project watchers are deferred:
they add substantial descriptor/lifecycle complexity and cannot prove absence of
missed events. No new watcher is necessary for correctness because reconciliation
is mandatory. A later watcher may accelerate invalidation but may never replace
reconciliation after overflow, degraded coverage or uncertainty.

### Coalescing and generations

A flight owns detached cancellation and bounded lifetime, not its first caller's
context. Waiters cancel independently; the last departing waiter cancels work.
Bound active enumeration and queued keys; reject or fall back without unbounded
goroutines. Reuse the proven transcript-flight lifecycle where practical, not
its data model. Service shutdown cancels flights and drops snapshots.

Every mutation bumps a generation **before** starting and invalidates again on
completion, including failures/uncertain outcomes. A flight started before a
bump cannot publish or satisfy a later request as current. A caller encountering
a superseded shared run retries once against the latest generation. Continued
churn falls back to one private, unretained reconciliation under the same admission
and lifetime bounds. Like uncached `files.List`, it is not an atomic view of
concurrent writes, but it avoids a retry error during ordinary sustained editing.
Generation bookkeeping is bounded with manager state; a global epoch handles unknown roots
and eviction without retaining an unbounded tombstone map.

Invalidate overlapping roots for file create/add/upload/save/replace/rename/
delete, including old and new names. Git writes, branch changes and sync invalidate
all retained roots on that service conservatively (worktree/common-dir precision
is an optimization, not a correctness dependency). Authorization is rechecked on
all hits, so Forget/Retire cannot preserve access. No dependency on diagnostics
approval or its separate watcher is introduced.

## Memory and ownership

Initial limits: 4 retained roots, 16 MiB total estimated owned path data, 4 active
flights and 16 queued keys. Account strings, slice storage and bookkeeping; record
estimate and live-heap evidence separately. Oversized results can be returned
without admission. Successful truncated results retain their Truncated flag and
may be shown only as unverified snapshots, never as a complete index. Errors and
cancellation publish nothing. Caller mutation must not affect manager entries;
clone public slices/maps and keep internal snapshots immutable.

## Frontend behavior

Preserve PF-11 host/root/sequence/epoch guards and editor state. An optional
backend snapshot fills the same stale/unverified presentation, never overriding
a newer authoritative list. Always follow it with ListFiles; explicit refresh
awaits reconciliation. Preserve deleted-status paths in FilePicker, status tabs,
blame gating and honest nested-status coverage. Host changes and disconnects
cancel requests and isolate cache keys. Failed refresh keeps unverified data only
with visible error/stale state; no false freshness.

## Acceptance and limits

- Differential Git/non-Git/subdirectory/ignored/nested/truncated fixtures preserve
  the existing authoritative Listing contract.
- Concurrent same-key calls perform one path enumeration/status flight, while
  different roots/options stay isolated. Cancellation, shutdown, generation races,
  failures and denied/revoked roots have deterministic barrier tests.
- External create/delete/ignore edits between sequential requests are discovered.
  Tests must not substitute HEAD/index mtime checks for these cases.
- Mutation invalidation covers every engine route, including uncertain results.
- Snapshot retention and work admission are bounded; reads/probes/save and uncapped
  search remain unchanged. RPC capability and old-peer fallback are tested.
- Repeat PF-03 and wide-tree benchmarks with work counts, concurrency, retained
  estimates and heap evidence. Record remaining sequential reconciliation cost.
- Run backend/race/build/native and Windows cross-vet, frontend test/check/build.
  Native GUI/SSH/WSL/Windows execution remains separately unperformed unless tested.

No installs, private session stores, app launch/replacement, process killing,
Windows packaging or push. Use generated fixtures and explicit temporary roots;
never run `scripts/qa-perf-bench.sh`.

# Performance Milestone 3 — Implementation briefs

Designed 2026-10-25. Status: **10/10 automated gate complete** (PI-01…10). Implement in order, commit each
finished task; do not push. [Design and contracts](../plan/performance-m3.md).
Estimates include focused tests. Split before coding if any brief exceeds 4 h;
never omit acceptance to fit the estimate. Native GUI QA is not an automated gate.

| ID | Scope | Estimate | Depends on |
|---|---|---:|---|
| PI-01 | Separate paths and status without behavior changes | 3 h | PF-03, PF-14 |
| PI-02 | Coalesced bounded listing flights | 4 h | PI-01 |
| PI-03 | Bounded immutable path snapshots | 3 h | PI-02 |
| PI-04 | File mutation generation invalidation | 4 h | PI-03 |
| PI-05 | Git/catalog/lifecycle invalidation | 3 h | PI-04 |
| PI-06 | Optional snapshot wire capability | 4 h | PI-05 |
| PI-07 | Frontend unverified snapshot integration | 4 h | PI-06 |
| PI-08 | External change and concurrency integration matrix | 4 h | PI-07 |
| PI-09 | Synthetic timing/work/retention evidence | 4 h | PI-08 |
| PI-10 | Integrated validation and milestone gate | 3 h | PI-09 |

Total: **36 h** planning estimate. Project watchers and uncapped search-index
reuse are explicitly deferred, not acceptance requirements quietly dropped.
Mandatory request-time reconciliation covers external changes and missed events.

## PI-01 — Separate path enumeration and status (3 h)

**Status:** complete 2026-10-25; [evidence](../perf.md#pi-01--separate-path-enumeration-and-status-2026-10-25).

Likely files: `internal/files/files.go`, `directories.go`, `nested_status.go`,
`listing_bench_test.go` and new focused listing tests.

Extract path-only and status-only operations, composing them through unchanged
`files.List`. Preserve Git and non-Git behavior, relative subdirectory scope,
ignored limits, deleted-status entries, nested repository coverage and truncation.
Do not change uncapped Search or path security checks.

Done when: differential fixture tests preserve all Listing fields; component Git
invocation counts explain exactly which work is path and which is status. Existing
listing/wide-tree regressions pass. No cache or protocol changes in this task.

## PI-02 — Coalesced bounded listing flights (4 h)

**Status:** complete 2026-10-25; [evidence](../perf.md#pi-02--bounded-coalesced-listing-flights-2026-10-25).

Likely files: new engine project-index manager, `service.go`, lifecycle tests.

Exact cleaned root/options keys; authorize every caller before and after waiting.
Detached flight context, independent waiter cancellation, last-waiter cancellation,
4-active/16-queued admission and shutdown cleanup. Share overlapping requests,
not completed status results. Bound retry on superseded generations as designed.

Done when: barrier tests prove same-key single enumeration/status, distinct keys,
leader/waiter cancellation, last departure, failure cleanup, queue saturation,
shutdown and revoked authorization. Existing ListFiles remains authoritative.

## PI-03 — Bounded immutable path snapshots (3 h)

**Status:** complete 2026-10-25; [evidence](../perf.md#pi-03--bounded-immutable-path-snapshots-2026-10-25).

Add 4-entry/16 MiB estimated-byte LRU, immutable ownership and cloned public
results. Oversized data bypasses admission. Truncated snapshots retain the flag;
errors/cancellation do not publish. Every authoritative call still reconciles.
Use bounded generation/global epoch state, including eviction ABA tests.

Done when: bytes/entries/eviction, mutation isolation, reset, failure, cancellation,
truncation and concurrent publication tests pass. Sequential create/delete requests
reconcile rather than returning retained data as fresh.

## PI-04 — File mutation invalidation (4 h)

**Status:** complete 2026-10-25; PI-08 success, conflict, cancellation and uncertain-outcome wrapper coverage passes.

Likely files: engine create/add/upload/save/replace/rename/delete routes and tests.

Invalidate overlapping scopes before and after every operation, including failed
and uncertain outcomes and both names in rename. A pre-mutation flight cannot
publish after the bump. Preserve existing mutation locks and revision/conflict
semantics. Do not invalidate editor/draft state with listing data.

Done when: a route coverage table and fake/barrier tests cover all mutations,
in-flight publication, overlapping roots, non-overlapping isolation, errors and
uncertainty. Fresh reads/probes and save revision guards remain unchanged.

## PI-05 — Git, catalog and lifecycle invalidation (3 h)

**Status:** complete 2026-10-25; successful linked-worktree/local-remote Git operations, withdrawal, cancellation and lifecycle coverage passes.

Cover stage/unstage/discard/commit, branch switch, sync, catalog authorization
withdrawal and service restart/stop. Conservatively invalidate all service roots
for Git operations before/after, including errors. Avoid unbounded repo alias maps.

Done when: Git changes cannot reuse old paths/status as current; Forget/Retire
revokes cache access; stop cancels flights and restart starts empty. Include
subdirectory/worktree fixtures, race tests and no weakened trust checks.

## PI-06 — Optional snapshot wire capability (4 h)

**Status:** complete 2026-10-25; automated compatibility/freshness tests pass.

Likely files: engine/app interfaces, RPC client/server/capabilities, bindings and
mock API. Add explicit optional snapshot envelope (root, generation, unverified
paths) while leaving ListFiles authoritative. Status never appears current in a
retained snapshot. Use established capability negotiation, not version guesses.

Done when: local/in-process RPC parity, unsupported old-peer fallback, rejected
root, disconnect/cancellation and immutable envelope tests pass. Existing clients
and servers still work without the optional method. Regenerate bindings with
installed tooling only, or record a blocker without fabricating generated files.

## PI-07 — Frontend snapshot integration (4 h)

**Status:** complete 2026-10-25; automated compatibility/freshness tests pass.

Likely files: files store, API/mock, FilePicker and frontend tests.

Use optional backend snapshots as unverified paths with mandatory authoritative
refresh. Preserve PF-11 LRU, host/root/epoch/sequence guards, error/stale UI,
explicit refresh, deleted-status merge, blame gating and all editor/draft state.
Late snapshot responses cannot replace an authoritative list. Old peers follow
the current behavior.

Done when: real-store/mock tests cover both response orders, stale errors,
host/root changes, disconnect, explicit refresh, fallback and draft preservation.
Frontend test/check/build pass; native focus/paint/scroll QA remains unperformed.

## PI-08 — Integrated external-change and race matrix (4 h)

**Status:** complete 2026-10-25; [matrix and limits](../perf.md#pi-08--external-change-and-concurrency-matrix-2026-10-25).

Generated Git/non-Git roots only. Exercise external untracked creation/deletion,
tracked deletion, nested ignore changes, ignore exclusions, subdirectory caps,
worktrees and nested repositories. These must reconcile even without watcher
signals or HEAD/index changes. Include concurrent different-root requests and
mutation during flight through local and in-process RPC routes.

Done when: authoritative results match uncached List, fresh status is independent
of snapshot age, race tests pass, and unknown/overflow/missed-event equivalence is
proved by mandatory enumeration (no project watcher is claimed).

## PI-09 — Repeatable performance evidence (4 h)

**Status:** complete 2026-10-25; [distributions, work and heap](../perf.md#pi-09--repeated-workload-and-retention-evidence-2026-10-25).

Repeat PF-03 listing, non-Git and PF-13/14 wide-tree workloads with prebuilt
before/after binaries and alternating order. Add concurrent same-key versus
different-key cases, immediate snapshot work/latency, request-time reconciliation
cost, status counts and retained estimate/live-heap measurements with reachable
manager. Exclude fixture construction; keep generated temporary roots explicit.

Done when: repeated distributions, allocations, Git/work counts, caps and memory
tradeoffs are recorded in `docs/perf.md`. Explain unchanged sequential Git work;
do not present stale snapshot latency as authoritative refresh latency.

## PI-10 — Integrated milestone gate (3 h)

**Status:** complete 2026-10-25; [automated gate and unperformed native checks](../perf.md#pi-10--integrated-milestone-gate-2026-10-25).

Run full backend tests/build/vet, affected race tests, Windows cross-vet and
frontend test/check/build. Review all PI acceptance against evidence, synchronize
trackers and record limits/deferred items. Do not mark a task complete solely
because its code landed. Native GUI/SSH/WSL/Windows execution is separate; do not
claim it, install tooling, launch apps, package Windows or run private-store QA.

Done when: all automated acceptance has evidence, failures/unavailable checks are
reported, every retained buffer/work queue has a bound, and next milestone still
requires its own design and ≤4h briefs before implementation.

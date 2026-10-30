# Performance improvement roadmap

Status: in progress, updated 2026-10-25. Milestone 1 now 13/15, the
startup package is complete (6/6 plus ST-06A/B) and Milestone 2 is complete
(12/12). PF-10A/B are complete. **Milestone 3 automated gate complete (10/10); next: Milestone 4 design.**

## Context and goal

Large session histories, long transcripts, and large project directories can feel
slow. The app already has a persistent SQLite metadata/FTS index and an in-memory
transcript LRU. Improve reuse and incremental data flow rather than replace the
Go providers → index/catalog → service → Wails/RPC → Svelte architecture.

Measure synthetic workloads first and preserve correctness while removing work.
Evidence is in [../perf.md](../perf.md); historical sections there describe the
code of their date, not the current code.

## Delivery sequence

| Performance milestone | Scope | Status |
|---|---|---|
| 1 | Synthetic baselines; correct/coalesced transcript loads; validated Codex source fast paths; unchanged checkpoint reuse; frontend listing retention; directory ignore-check batching; [briefs](../tasks/performance-m1.md) | Gate closed at 12/15; now 13/15 after PF-10. PF-07 partial (production opt-out, no further work planned); PF-08 superseded by M2; **PF-10A/B complete** |
| Startup priority | Startup responsiveness slice pulled forward from M4; [plan](performance-startup.md), [briefs](../tasks/performance-startup.md) | Complete 6/6, plus the ST-06A/B large-index follow-up |
| 2 | Incremental transcripts: provider-owned resumable state, immutable snapshots, byte-bounded cache, shared FTS loading, transcript deltas; [design](performance-m2.md), [briefs](../tasks/performance-m2.md) | Complete 12/12 ([gate](../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25)) |
| 3 | Shared per-root project index, separate Git-status freshness, coalesced refresh and safe invalidation | Complete 10/10 automated gate ([evidence](../perf.md#pi-10--integrated-milestone-gate-2026-10-25)); native checks unperformed |
| 4 | Per-source persistent checkpoints, targeted scans, progressive cold discovery, catalog deltas/paging, bounded fair FTS job batches | Roadmap only |
| 5 | Lazy expensive content, bounded rendering memoization, transcript/tree/source virtualization | Roadmap only |

These numbers name this performance effort, not the original M1…M8 milestones.

**Before implementing each milestone:** write its design and ≤4h task briefs in
`docs`, link them from `TASKS.md`, and settle dependencies and acceptance gates.
Later milestones must use the measurements and contracts established by earlier
work; roadmap entries are not executable specifications or permission to
implement them wholesale.

## Next

1. **PF-10A, then PF-10B: Codex checkpoint reuse**
   ([briefs](../tasks/performance-m1.md#pf-10--codex-checkpoint-reuse)). Every
   Codex scan decodes each unchanged rollout's checkpoint three times and
   re-encodes it once. Since M2's resolver scans on every Codex load and append,
   this is now on the live path: a warm append costs 16.6 ms, 19 MB and 285k
   allocations with 64 other rollouts, against 0.7 ms with none
   ([IT-12](../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25)).
   Catalog scans pay the same cost. **PF-10A complete**
   ([evidence](../perf.md#pf-10a--decode-once-per-scan-2026-10-25)): one decode and no encode per
   unchanged source; that append now takes 3.4 ms. PF-10B removes the remaining
   decode with a bounded cross-scan cache. **PF-10B complete**
   ([evidence](../perf.md#pf-10b--cross-scan-checkpoint-cache-2026-10-25)):
   unchanged scans do zero decodes; the 64-extra append now takes 2.36 ms.
2. **Milestone 3: shared project index.** [Design](performance-m3.md) and
   [≤4h briefs](../tasks/performance-m3.md) complete (10/10 automated gate). Mandatory reconciliation preserves external-change
   correctness; retained paths are explicitly unverified, status is separate,
   and concurrent requests coalesce. Project watchers and uncapped Search reuse
   are deferred explicitly, not assumed.
3. **Next: Milestone 4 design**, then ≤4h briefs; Milestone 5 likewise requires design first.

Not assigned to a milestone yet, from the M2 gate:

- **First open of a large transcript** still parses the whole file (~1.1 s for a
  100 MB Claude session, 1.4–1.9 s for Codex). A tail-first or windowed first
  load would show the latest page sooner. Decide whether it belongs in M5 when
  M5 is designed.
- **Native verification.** Native GUI, SSH, WSL and Windows runs of the M2 and
  startup changes have not been performed.

## Milestone 1

Briefs, dependencies and acceptance criteria:
[performance-m1.md](../tasks/performance-m1.md). Evidence for each task is
linked from its brief; the gate is
[PF-15](../perf.md#pf-15--integrated-evidence-and-milestone-gate-2026-10-24).

- **Complete (13):** PF-01…PF-06, PF-09…PF-15.
- **PF-07 partial.** The bounded content proof works, but it is slower than a
  plain load on large byte-heavy roots, and no safe automatic policy was found
  ([PF-07C](../perf.md#pf-07c--cost-policy-decision-and-production-bypass-2026-10-24)).
  Production declines it. No further work is planned.
- **PF-08 superseded.** Its goal, removing Codex resolution work from page,
  Blob and indexing loads, is met by M2's IT-06 resolver and IT-09 shared FTS
  loading. The part that depends on PF-07 activation (callers passing catalog
  metadata to the source fast paths) is not implemented.
- **PF-10 reopened** as PF-10A/B. It no longer waits for PF-07: PF-07's
  outcome is final and the Codex scan code is stable.

## Later milestone boundaries

### 3 — Shared project index

Cache file paths/directories separately from Git status. Authorize every request,
including hits; isolate roots, options and subdirectory scopes. Invalidate for
file mutations, untracked changes, ignore configuration, Git operations and
watcher overflow; reconcile missed events. HEAD/index mtimes alone are inadequate.
Cached listings never replace file-presence probes, revision checks or reads.

### 4 — Startup and catalog scaling

Preserve `CommitScan` → `Catalog.Apply` → event ordering and shared-cache
writer/reader promotion, schema seeding, `LoadSnapshot`, ClearCache/Forget/Retire
exclusion. Targeted watcher paths are hints; ambiguity/overflow falls back to full
reconciliation. Progressive scans publish committed upserts, but removals happen
only after successful full enumeration. Batch FTS work fairly without losing
debounce, maximum delay, retry backoff or revision checks. Measure watcher setup
and `quick_check`; do not remove integrity checks without a tested recovery design.

### 5 — Rendering

Measure lazy rendering/memoization before virtualization. Existing fixed-height
session-list virtualization is a pattern, not a variable-height transcript drop-in.
Preserve scroll anchors, search jumps, selection, load-earlier, keyboard navigation,
source diagnostics, source coordinates, copy behavior and presentation mode.

## Shared guardrails

- Use repository fixtures/generated temporary roots and explicit cache locations.
  Do not read real session stores or run `scripts/qa-perf-bench.sh`, which does so.
- Do not reuse another application's private cache or write to its session store.
- Preserve authorization, path/symlink checks and canonical provider selection.
- Preserve drafts, conflicts, uncertain saves, undo history and host isolation.
- Bound memory, active work and invalidation bookkeeping; do not only increase LRU size.
- Do not install tools, replace the installed app, terminate processes, push, or
  produce Windows test packages as part of this work without a separate request.
- Automated tests do not establish native GUI responsiveness. Record manual checks
  as unperformed until actually exercised; do not present estimates as measurements.

## Verification policy

Each task adds focused regression tests and deterministic work-count assertions
where practical. Use barriers for concurrency tests, not timing-dependent sleeps.
Run affected Go race tests, backend tests/build/vet and frontend tests/check/build.
Use installed tooling only and report unavailable checks rather than installing it.

Compare repeated cold/warm synthetic measurements on the same host, excluding
fixture construction. Record distributions, allocations, retained memory and
payload/work counts where supported. Absolute CI timing thresholds are advisory.
A cache hit must be equivalent to the correct uncached path; stale/error/cancel
paths deserve the same coverage as successful reuse.

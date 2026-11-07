# Performance Milestone 1 — Baselines and targeted reuse — tasks

- **Plan:** [performance.md](../../plan/performance.md)
- **Progress:** 13/15 done

- Status: gate closed 2026-10-24; **13/15 tasks complete**. PF-07 partial (no further work planned), PF-08 superseded by Milestone 2 (2026-10-25), **PF-10A/B complete 2026-10-25**.
- All estimates include focused tests and are planning estimates, not promises.
- Every task is ≤4 developer hours. Split it here before coding
  if current-code investigation reveals a larger scope. Never drop acceptance
  criteria silently to fit an estimate.

## Scope

Improve repeated work without incremental transcript parsing, a shared backend
project index, wire/API deltas, schema migrations or virtualization. Those belong
to later milestones. Retain standalone provider behavior and old remote behavior.
PF-01 completed 2026-10-24; repeated synthetic baseline and verification are in
[../perf.md](../../perf.md#pf-01--synthetic-catalog-and-scanner-baseline-2026-10-24).
PF-02 completed 2026-10-24; transcript baseline evidence is appended to
[../perf.md](../../perf.md#pf-02--synthetic-transcript-baseline-2026-10-24).
PF-03 completed 2026-10-24; project listing baseline evidence is appended to
[../perf.md](../../perf.md#pf-03--synthetic-project-listing-baseline-2026-10-24).
PF-04 completed 2026-10-24; atomic per-key transcript invalidation and deterministic
race evidence are in [../perf.md](../../perf.md#pf-04--atomic-transcript-invalidation-2026-10-24).
PF-05 completed 2026-10-24; coalesced loads, owned cancellation and repeated
same-key evidence are in [../perf.md](../../perf.md#pf-05--coalesced-transcript-loads-2026-10-24).
PF-06 completed 2026-10-24; optional source contract, bounded canonical fallback,
ownership and Codex feasibility are in [../perf.md](../../perf.md#pf-06--validated-source-loading-contract-2026-10-24).
PF-07A/B implemented; PF-07C safe opt-out complete. PF-07 remains partial/blocked:
no defensible automatic activation policy; production proof is off. PF-09 and
PF-11…PF-15 complete; the milestone gate is closed. On 2026-10-25, after
Milestone 2, PF-08 was marked superseded and PF-10 was reopened (see their
briefs).

## Task index

PF-01 through PF-06, PF-09 and PF-11…PF-15 are **complete**; PF-07 is **partial** (A/B implemented, C safe opt-out complete; activation off, no further work planned); PF-08 is **superseded**; PF-10A/B are **complete**. Dependencies are completion gates, not mere ordering
suggestions. All implementation tasks also depend on the shared guardrails in the
[parent plan](../../plan/performance.md#shared-guardrails).

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [PF-01](PF-01.md) | ✅ Done | Catalog and scan baselines | 4 h | — |
| [PF-02](PF-02.md) | ✅ Done | Transcript baselines | 4 h | — |
| [PF-03](PF-03.md) | ✅ Done | Project listing baselines | 4 h | — |
| [PF-04](PF-04.md) | ✅ Done | Atomic transcript cache invalidation | 4 h | PF-02 |
| [PF-05](PF-05.md) | ✅ Done | Coalesced loads and cancellation | 4 h | PF-04 |
| [PF-06](PF-06.md) | ✅ Done | Validated source-loading contract | 3 h | PF-02 |
| [PF-07](PF-07.md) | 🟡 Partial | Codex Load/Blob fast paths (A/B implemented, C safe opt-out; blocked) | 4 + 4 + 4 | PF-06 |
| [PF-08](PF-08.md) | ➖ Superseded | Engine/FTS source-aware callers (goal met by IT-06/IT-09) | 4 h | PF-05, PF-07 |
| [PF-09](PF-09.md) | ✅ Done | Claude checkpoint reuse | 4 h | PF-01 |
| [PF-10](PF-10.md) | ✅ Done | Codex unchanged checkpoint reuse: PF-10A in-scan reuse, PF-10B cross-scan cache | 3 + 3 h | PF-01 |
| [PF-11](PF-11.md) | ✅ Done | Bounded frontend listing snapshots | 3 h | PF-03 |
| [PF-12](PF-12.md) | ✅ Done | Listing freshness and draft safety | 4 h | PF-11 |
| [PF-13](PF-13.md) | ✅ Done | Bounded directory frontiers | 3 h | PF-03 |
| [PF-14](PF-14.md) | ✅ Done | Batched ignore checks | 4 h | PF-13 |
| [PF-15](PF-15.md) | ✅ Done | Integration and before/after evidence | 4 h | PF-01…PF-14 |

Total: **67 h** (PF-07 split into 4 h + 4 h + 4 h; PF-10 into 3 h + 3 h), including focused tests. Native/manual QA or unexpected scope
requires a follow-up package, not an invented passing result.

Baseline tasks may be independent. Cache/source work shares `service.go`;
checkpoint/source work shares Codex files. Coordinate edits even when dependencies
allow parallel work. Default execution is one task at a time, updating status and
evidence before proceeding.

## Status notes

Planned 2026-10-24, updated 2026-10-25. **Milestone 3 automated gate complete; next: Milestone 4 design** (PF-10A/B complete) ([roadmap](../../plan/performance.md#next)).
Milestone 1: **13/15 complete**, PF-01…PF-06, PF-09 and PF-11…PF-15 complete; PF-07 partial (production opt-out, no further work planned); PF-08 superseded by Milestone 2; PF-10A/B complete. Milestone 1 gate closed; native GUI QA unperformed.
[Roadmap](../../plan/performance.md) ·
[Milestone 1 briefs and dependency graph](README.md).
PF-01 completed 2026-10-24: synthetic catalog/provider benchmarks, correctness and
work-count regressions; repeated baseline and verification in [docs/perf.md](../../perf.md#pf-01--synthetic-catalog-and-scanner-baseline-2026-10-24).
PF-02 completed 2026-10-24: generated large transcript equivalence, repeated
load/page/concurrency benchmarks, byte accounting and reusable deterministic barriers;
[evidence](../../perf.md#pf-02--synthetic-transcript-baseline-2026-10-24).
PF-03 completed 2026-10-24: generated Git listing/cap/isolation tests, component
benchmarks and frontend request-count baseline; [evidence](../../perf.md#pf-03--synthetic-project-listing-baseline-2026-10-24).
PF-04 completed 2026-10-24: atomic transcript invalidation using the existing LRU,
per-key tokens and clear epoch; deterministic race/capacity/paging regressions and
verification [evidence](../../perf.md#pf-04--atomic-transcript-invalidation-2026-10-24).
PF-05 completed 2026-10-24: current-flight coalescing, owned cancellation, four-load
bound and engine restart/shutdown coverage; [evidence](../../perf.md#pf-05--coalesced-transcript-loads-2026-10-24).
PF-06 completed 2026-10-24: optional source-loading contract, bounded canonical
fallback, ownership and Codex feasibility constraints; [evidence](../../perf.md#pf-06--validated-source-loading-contract-2026-10-24).
PF-07 split before coding: parent partial, not counted complete. PF-07A complete;
PF-07B matrix/evidence implemented with byte-heavy regressions. PF-07C complete (safe opt-out), planned
before coding (4 h): investigated cheap cost eligibility; no defensible policy
established. Production Load/Blob immediately decline to zero-proof canonical
fallback; experiment retained privately for tests, cold Blob proof prevented,
deterministic bypass counts and isolated small/large/Blob evidence recorded.
Parent remains partial. PF-09 completed with bounded Claude summary reuse; [evidence](../../perf.md#pf-09--claude-checkpoint-summary-reuse-2026-10-24). PF-11 completed with bounded listing-only snapshots and immediate reopen; [evidence](../../perf.md#pf-11--bounded-frontend-listing-snapshots-2026-10-24). PF-12 completed with mutation/hidden-save/Git listing invalidation and freshness/draft regressions; [evidence](../../perf.md#pf-12--listing-freshness-and-draft-safety-2026-10-24). PF-13 completed with bounded cross-directory ignore frontiers and reference-equivalence tests; [evidence](../../perf.md#pf-13--bounded-directory-frontiers-2026-10-24). PF-14 completed with 4096-path/256 KiB ignore batches and pathspec-safe anchoring; [evidence](../../perf.md#pf-14--batched-ignore-checks-2026-10-24). PF-15 completed the milestone gate: all deterministic gates pass, before/after evidence and remaining bottlenecks recorded; [evidence](../../perf.md#pf-15--integrated-evidence-and-milestone-gate-2026-10-24).
[Decision and verification](../../perf.md#pf-07c--cost-policy-decision-and-production-bypass-2026-10-24). [PF-07C brief](PF-07.md#pf-07c--cost-policy-decision-and-safe-production-bypass-4-h-pf-07b).
- [PF-07A](PF-07.md#pf-07a--bounded-content-proof-and-accepting-source-methods-4-h-pf-06): bounded content proof, accepting Load/Blob methods and focused regression/verification (4 h; PF-06).
- [PF-07B](PF-07.md#pf-07b--mutation-matrix-and-repeated-evidence-4-h-pf-07a): full mutation/overlay/parent/WAL/concurrency matrix and repeated benchmark evidence (4 h; PF-07A).
- [PF-10A](PF-10.md#pf-10a--decode-once-per-scan-skip-unchanged-re-encodes-3-h-pf-01): decode each checkpoint once per scan and skip unchanged re-encodes (3 h; PF-01).
- [PF-10B](PF-10.md#pf-10b--cross-scan-decoded-checkpoint-cache-3-h-pf-10a): bounded cross-scan decoded checkpoint cache shared with the resolver (3 h; PF-10A).

These tasks are separate from the original milestone totals. Milestone 3 is now
complete (automated gate): [shared project index design](../../plan/performance-m3.md) and
[PI-01…PI-10 briefs](../performance-m3/README.md), **10/10 complete**, 36 h.
PI-01…10 complete; [gate, measured costs and native-check limits](../../perf.md#pi-10--integrated-milestone-gate-2026-10-25).
Milestones 4/5 still require their own design and ≤4h breakdown before coding.

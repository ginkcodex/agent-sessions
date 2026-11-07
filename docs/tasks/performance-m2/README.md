# Performance Milestone 2 — Incremental transcripts — tasks

- **Plan:** [performance-m2.md](../../plan/performance-m2.md)
- **Progress:** 12/12 done

- Status: complete 2026-10-25; **12/12 complete**; [gate evidence](../../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25).
- Design: [../plan/performance-m2.md](../../plan/performance-m2.md). Roadmap: [../plan/performance.md](../../plan/performance.md).
- Estimates include focused tests and are not promises. Every task is ≤4 hours.
  If investigation shows a task is bigger, split it here
  before coding. Never drop acceptance criteria silently.
- Shared guardrails and the verification policy from the
  [roadmap](../../plan/performance.md#shared-guardrails) apply to every task:
  - generated fixtures and explicit temporary roots only;
  - never run `scripts/qa-perf-bench.sh`;
  - barriers instead of sleeps;
  - race tests for every concurrent path;
  - native GUI QA is reported as unperformed unless it was actually done.

## Task index

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [IT-01](IT-01.md) | ✅ Done | Append, resolution, retention and payload baselines | 3 h | — |
| [IT-02](IT-02.md) | ✅ Done | Incremental provider contract, continuity, harness | 4 h | IT-01 |
| [IT-03](IT-03.md) | ✅ Done | Copy-on-write message builder | 3 h | IT-02 |
| [IT-04](IT-04.md) | ✅ Done | Claude resumable loader | 4 h | IT-03 |
| [IT-05](IT-05.md) | ✅ Done | Codex resumable loader | 3 h | IT-03 |
| [IT-06](IT-06.md) | ✅ Done | Codex resolution without a cold full scan | 4 h | IT-02 |
| [IT-07](IT-07.md) | ✅ Done | Resumable engine cache entries | 4 h | IT-04, IT-05 |
| [IT-08](IT-08.md) | ✅ Done | Byte budget and revisions | 3 h | IT-07 |
| [IT-09](IT-09.md) | ✅ Done | Shared FTS loading | 3 h | IT-08 |
| [IT-10](IT-10.md) | ✅ Done | Transcript delta API | 4 h | IT-08 |
| [IT-11](IT-11.md) | ✅ Done | Frontend delta refresh | 4 h | IT-10 |
| [IT-12](IT-12.md) | ✅ Done | Integrated gate | 3 h | IT-01…IT-11 |

Total: **42 h**. The default order is sequential. IT-06 depends only on IT-02, so
it may run before IT-04 or IT-05. `service.go` and `transcript_cache.go` are
shared by IT-07…IT-10.

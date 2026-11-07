# Performance Milestone 3 — tasks

- **Plan:** [performance-m3.md](../../plan/performance-m3.md)
- **Progress:** 10/10 done

Designed 2026-10-25. Status: **10/10 automated gate complete** (PI-01…10). Implement in order, commit each
finished task; do not push. [Design and contracts](../../plan/performance-m3.md).
Estimates include focused tests. Split before coding if any brief exceeds 4 h;
never omit acceptance to fit the estimate. Native GUI QA is not an automated gate.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [PI-01](PI-01.md) | ✅ Done | Separate paths and status without behavior changes | 3 h | PF-03, PF-14 |
| [PI-02](PI-02.md) | ✅ Done | Coalesced bounded listing flights | 4 h | PI-01 |
| [PI-03](PI-03.md) | ✅ Done | Bounded immutable path snapshots | 3 h | PI-02 |
| [PI-04](PI-04.md) | ✅ Done | File mutation generation invalidation | 4 h | PI-03 |
| [PI-05](PI-05.md) | ✅ Done | Git/catalog/lifecycle invalidation | 3 h | PI-04 |
| [PI-06](PI-06.md) | ✅ Done | Optional snapshot wire capability | 4 h | PI-05 |
| [PI-07](PI-07.md) | ✅ Done | Frontend unverified snapshot integration | 4 h | PI-06 |
| [PI-08](PI-08.md) | ✅ Done | External change and concurrency integration matrix | 4 h | PI-07 |
| [PI-09](PI-09.md) | ✅ Done | Synthetic timing/work/retention evidence | 4 h | PI-08 |
| [PI-10](PI-10.md) | ✅ Done | Integrated validation and milestone gate | 3 h | PI-09 |

Total: **36 h** planning estimate. Project watchers and uncapped search-index
reuse are explicitly deferred, not acceptance requirements quietly dropped.
Mandatory request-time reconciliation covers external changes and missed events.

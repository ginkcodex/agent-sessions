# Startup responsiveness — tasks

- **Plan:** [performance-startup.md](../../plan/performance-startup.md)
- **Progress:** 10/10 done

Status: original package **6/6 complete**; large-index follow-up **ST-06A/ST-06B complete** 2026-10-24. The original gate did not reproduce the user's continuing >10 s delay. Original completion:  ST-00/ST-01 complete; ST-02 complete (ST-02A engine, ST-02B adapters); ST-03/ST-04 complete; ST-05 integrated gate complete with native GUI/SSH/Windows execution explicitly unperformed. Package closed.
[Plan and status/compatibility contract](../../plan/performance-startup.md) ·
[Roadmap](../../plan/performance.md)

This priority package precedes performance Milestone 2; it pulls forward a narrow
startup slice, not all of Milestone 4. When it ran, performance M1 stood at
12/15; see the [performance group](../performance/README.md) for current status.
The reported roughly 10-second delay is not a measured baseline and its exact
cause is unknown. ST-01 synthetic measurement is recorded in
[../perf.md](../../perf.md#st-01--full-engine-startup-baseline-2026-10-24);
the reported exact delay remains undiagnosed. Final integrated evidence:
[ST-05](../../perf.md#st-05--integrated-startup-gate-2026-10-24).

## Execution rules and index

Execute strictly **ST-00 → ST-01 → ST-02 → ST-03 → ST-04 → ST-05**, reviewing each
completed task and updating status/evidence before moving on. Every estimate
includes focused tests, is ≤4 developer hours, and is not an elapsed-time promise.
Split a larger task here **before coding**, especially if ST-02
adapter/lifecycle work exceeds its budget. Do not drop acceptance criteria.
Likely files are integration points, not instructions to create duplicate systems.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [ST-00](ST-00.md) | ✅ Done | Startup scope, status policy, safety and task documentation | 1 h | — |
| [ST-01](ST-01.md) | ✅ Done | Repeated full startup baseline; ST-04 measurement blocker | 4 h | ST-00 |
| [ST-02](ST-02.md) | ✅ Done | Backend discovery readiness contract | ≤4 h + ≤4 h | ST-01 |
| [ST-03](ST-03.md) | ✅ Done | Truthful frontend discovery/empty/error states | 3 h | ST-02B |
| [ST-04](ST-04.md) | ✅ Done | Measured fanout; early runner provider publication | ≤4 h | ST-03; ST-01 evidence |
| [ST-05](ST-05.md) | ✅ Done | Integrated barriers, before/after evidence and QA limits | 4 h | ST-04 |
| [ST-06A](ST-06A.md) | ✅ Done | Generated large FTS cache: open/integrity/catalog timings | ≤4 h | ST-05 |
| [ST-06B](ST-06B.md) | ✅ Done | Early cached hints, mandatory integrity and recovery | ≤4 h | ST-06A |
| [ST-07](ST-07.md) | ✅ Done | Remote startup responsiveness (2026-10-24) | 1 h + ≤3 h + ≤4 h + ≤3 h + ≤3 h + ≤3 h |  |
| [ST-08](ST-08.md) | ✅ Done | Startup connection snapshot rollback |  |  |

Total planned effort: **≤24 h after ST-02 split**. ST-00 is documentation only.
Reviewed task commits are separate, on `feature/improve-performance` with
required attribution; no push. Do not begin Milestone 2 implementation here.

All tasks inherit the plan's [safety/lifecycle invariants](../../plan/performance-startup.md#safety-and-lifecycle-invariants):
generated fixtures and explicit provider/cache roots only; no private stores/logs
or private caches, no `scripts/qa-perf-bench.sh`, default-root app launch, installs,
app replacement, process termination or Windows packaging. Preserve commit →
catalog apply → event ordering, ClearCache/Forget/Retire exclusion, atomic reader
snapshots, writer promotion, schema seeding, corrupt-cache recovery and root
isolation. Preserve drafts/selection/navigation and host boundaries. Own and
bound cancellation/background work; no late publications or leaked watchers.

## Status notes

Pulled forward from M4 and completed before Milestone 2. **ST-07 remote follow-up: A–F complete (synthetic gates; real SSH/native retest unperformed)** ([briefs](ST-07.md)); preserve mandatory verification while exposing cached catalog through early remote initialization. [Plan and readiness/compatibility
policy](../../plan/performance-startup.md) · [Sequential ≤4h briefs](README.md).
Progress: original **6/6 complete**; **ST-06A/ST-06B complete** after the user retest still showed >10 s. Historical package gate 2026-10-24: ST-00…ST-04 complete; ST-05 integrated gate complete ([evidence](../../perf.md#st-05--integrated-startup-gate-2026-10-24)) with native GUI/SSH/Windows execution explicitly unperformed. ST-04 resolves its measurement gate with generated fanout and fast/slow provider evidence; early disabled/fallback catalog notification is the one selected fix. Watcher fanout scaling remains a follow-up.
The reported roughly 10-second launch delay is not a measured baseline; its exact
cause is unknown. This narrow slice is pulled forward from performance M4, not a
catalog overhaul. Historical evidence and native QA limitations stand.

Strictly sequential; **≤24 h after ST-02 split** including focused tests. Split oversized tasks
before coding. Generated fixtures and explicit roots only: no private session
stores/logs or `scripts/qa-perf-bench.sh`. ST-00 makes no code changes;
no Milestone 2 implementation is included.

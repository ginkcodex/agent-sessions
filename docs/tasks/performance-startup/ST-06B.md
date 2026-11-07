# ST-06B — Publish cached hints before full integrity verification

- **Status:** ✅ Done — complete 2026-10-24; [evidence](../../perf.md#st-06b--early-writer-catalog-hints-2026-10-24)
- **Estimate:** ≤4 h
- **Depends on:** ST-06A
- **Plan:** [performance-startup.md](../../plan/performance-startup.md)

Wails v2.14.0 darwin `frontend.go:248–253` already invokes `OnStartup` on a
separate goroutine before `mainWindow.Run`; no asynchronous desktop startup rewrite
is needed. Catalog queries and discovery snapshots do not take the engine mutex.
Keep synchronous Engine.Start and its existing Close/ClearCache serialization.

Candidate design: while owning the writer lock, read only the current-schema cache
metadata through the existing read-only API and publish a consistent cached snapshot
before the existing full writer open/integrity/migration path. Keep verification
mandatory and the discovery state nonterminal. After verification/recovery, replace
hints with the authoritative catalog and notify again, including an empty rebuild.
A failed early read must not block normal startup or trigger destructive recovery.
Do not split the established reader snapshot or alter promotion ordering.

**Acceptance:** deterministic barrier inside verification proves cached groups/counts/
sessions can be queried and catalog event delivered beforehand; failed/corrupt early
read falls through safely; successful integrity recovery removes stale hints; writer
failure followed by fallback cannot leave ghost hints; cancellation/Close/restart and
reader promotion invariants preserved. Repeated generated large-index timing must
show publication no longer waits for the full integrity scan. Native responsiveness
remains unverified. Split before coding if design exceeds this scope.

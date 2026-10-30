# Performance Milestone 2 — Incremental transcripts

- Status: complete 2026-10-25; **12/12 complete**; [gate evidence](../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25).
- Design: [../plan/performance-m2.md](../plan/performance-m2.md). Roadmap: [../plan/performance.md](../plan/performance.md).
- Estimates include focused tests and are not promises. Every task is ≤4 hours.
  If investigation shows a task is bigger, split it here and in `TASKS.md`
  before coding. Never drop acceptance criteria silently.
- Shared guardrails and the verification policy from the
  [roadmap](../plan/performance.md#shared-guardrails) apply to every task:
  - generated fixtures and explicit temporary roots only;
  - never run `scripts/qa-perf-bench.sh`;
  - barriers instead of sleeps;
  - race tests for every concurrent path;
  - native GUI QA is reported as unperformed unless it was actually done.

## Task index

| Task | Deliverable | Est | Dependencies |
|---|---|---|---|
| [IT-01](#it-01--append-and-resolution-baselines) | Append, resolution, retention and payload baselines | 3 h | — |
| [IT-02](#it-02--incremental-provider-contract-and-continuity) | Provider contract, continuity marks, equivalence harness | 4 h | IT-01 |
| [IT-03](#it-03--copy-on-write-message-builder) | Shared copy-on-write message builder | 3 h | IT-02 |
| [IT-04](#it-04--claude-resumable-loader) | Claude resumable loader | 4 h | IT-03 |
| [IT-05](#it-05--codex-resumable-loader) | Codex resumable loader | 3 h | IT-03 |
| [IT-06](#it-06--incremental-codex-resolution) | Codex resolution without a cold full scan | 4 h | IT-02 |
| [IT-07](#it-07--resumable-engine-cache-entries) | Stale-not-evicted entries, resume and predecessor flights | 4 h | IT-04, IT-05 |
| [IT-08](#it-08--byte-budget-and-revisions) | Byte budget, revisions and change history | 3 h | IT-07 |
| [IT-09](#it-09--shared-fts-loading) | Injected FTS loader and background slot | 3 h | IT-08 |
| [IT-10](#it-10--transcript-delta-api) | Backend/App/RPC delta method and capability | 4 h | IT-08 |
| [IT-11](#it-11--frontend-delta-refresh) | Frontend delta refresh and fallback | 4 h | IT-10 |
| [IT-12](#it-12--integrated-gate) | Integrated evidence and milestone gate | 3 h | IT-01…IT-11 |

Total: **42 h**. The default order is sequential. IT-06 depends only on IT-02, so
it may run before IT-04 or IT-05. `service.go` and `transcript_cache.go` are
shared by IT-07…IT-10.

## IT-01 — Append and resolution baselines

**Status:** complete 2026-10-25; [evidence](../perf.md#it-01--append-and-resolution-baselines-2026-10-25).

**Goal:** measure what M2 removes before changing it.

**Likely files:**
- `internal/engine/transcript_bench_test.go`
- `internal/provider/providertest/transcript.go`
- `internal/provider/codex` benchmarks
- `docs/perf.md`

Work:

- **Append-after-load benchmark.** Claude and Codex at 38 and 100 MB: load once,
  append 1, 10 and 100 generated records, invalidate as `onChanged` does, then
  reload. Report:
  - ms/op and B/op;
  - provider calls;
  - source bytes;
  - resolution bytes (`ScanWork`).
- **Codex resolution scaling.** A fixture with 64 extra rollouts (≥1 MiB each)
  plus the target session. Measure the bytes and time of `Load`'s resolution,
  separately from the transcript read.
- **Retained bytes.** The heap delta after GC of one cached 38/100 MB
  transcript, labelled as a measurement. Add a deterministic structural estimate
  (`model.TranscriptBytes`) and its ratio to the heap measurement. IT-08 uses the
  estimate.
- **Payload.** The JSON bytes of the 50-message live window refetched today, next
  to the bytes of only the changed messages after a one-record append.

**Done when:**
- generation is excluded from timing;
- three repeated runs and the environment are recorded in `docs/perf.md`;
- the commands are documented;
- no production behaviour changes, apart from the estimate helper.

## IT-02 — Incremental provider contract and continuity

**Status:** complete 2026-10-25; [evidence](../perf.md#it-02--incremental-contract-and-continuity-2026-10-25).

**Goal:** the in-process contract plus validation tools, so that no provider
invents its own.

**Likely files:**
- `internal/provider/{provider.go,incremental.go,continuity.go}`
- `internal/provider/providertest/incremental.go`

Work:

- Add `TranscriptState`, `IncrementalResult`, `IncrementalLoader` and
  `provider.LoadIncremental`, with the design's documented ownership,
  `FirstChanged` and immutability rules. Fallback wraps `Load` as a rebuild with
  `FirstChanged` 0.
- **Continuity mark.** Record, at the committed offset:
  - the file identity;
  - size and mtime;
  - the committed offset;
  - the hash of the first ≤4 KiB;
  - the hash of the last ≤4 KiB before the offset.

  `Validate(path)` returns resume or rebuild. The rules are: same path and
  `os.SameFile`, `size >= offset`, no same-size mtime change, and both windows
  matching.
- **Contract harness** (`providertest`):
  - resume at each committed line boundary (all for small fixtures, sampled for
    large) and require deep equality with a cold `Load`;
  - check that `FirstChanged` is never above the true first difference;
  - check that earlier snapshots stay unchanged by keeping deep copies;
  - a cancellation and error case after which prev is not reused.
- A fake incremental provider exercises the harness, including deliberately broken
  variants: a high `FirstChanged`, a mutated snapshot, a resume after truncation.
  The harness must catch each of them.

**Done when:** the continuity matrix (truncate, replace, same-size rewrite, head or
tail window change, growth, no change) passes, and the harness rejects every
broken fake.

## IT-03 — Copy-on-write message builder

**Status:** complete 2026-10-25; [evidence](../perf.md#it-03--copy-on-write-message-builder-2026-10-25).

**Goal:** one tested implementation of immutable publication.

**Likely files:** `internal/provider/transcriptbuild/`.

Work:

- `Append` and `Touch(i)`. `Touch` copies `Parts`, `ToolCall`, `FileRef`, child
  `SessionRef` and `Files` for frozen messages. On the first frozen touch per
  generation it gets a fresh backing array.
- Tool locations replace raw `*ToolCall` maps.
- `Freeze()` returns a clipped slice and the lowest touched index.
- `Fork()` for the best-effort tail.
- An incremental size estimate for the touched and appended messages.

**Done when:**
- unit tests show earlier frozen snapshots stay deep-equal after touches, appends,
  forks and refreezes;
- a race test has many readers walking a frozen snapshot while the builder
  touches every message;
- allocation counts show no O(n) work per record, only per freeze.

## IT-04 — Claude resumable loader

**Status:** complete 2026-10-25. Evidence: `docs/perf.md`, IT-04.
The fuzz setup uses tool locations rather than pointers; existing fixture/golden
assertions remain unchanged.

**Goal:** Claude implements `IncrementalLoader` and keeps its full `Load` output
byte-for-byte.

**Likely files:**
- `internal/provider/claude/{load.go,incremental.go,load_test.go}`

Work:

- Move the loader onto `transcriptbuild`. The resumable state holds:
  - the current split assistant;
  - tool locations;
  - the open compaction part;
  - the scan checkpoint, cloned for the tail fork;
  - the line base;
  - the continuity mark.
- Standalone `Load` becomes `LoadIncremental(nil)`.
- At publish, recompute:
  - live status and pending → unknown status;
  - `linkChildren` child links;
  - subagent `meta.json`;
  - CWD / repo metadata.

  Track the indices each one touches, and lower `FirstChanged` when a value
  changes.
- Cap `RetainedBytes`. Above the cap, return no state.

**Done when** the IT-02 harness passes on the existing PF-02 fixtures and on new
small fixtures covering:
- split assistants;
- earlier `tool_result`;
- errors;
- compaction summaries;
- images;
- sidechains;
- long lines;
- an invalid line;
- a valid and an invalid unterminated tail;
- live → ended;
- added and removed subagent sidecars.

The existing Claude tests stay unchanged, and race tests pass.

## IT-05 — Codex resumable loader

**Status:** complete 2026-10-25. Evidence: `docs/perf.md`, IT-05.
Codex never reports `meta.Live` today, so live → ended is covered by an internal
test that sets it directly. The experimental `LoadFromSource` path still
rebuilds.

**Goal:** the same as IT-04 for `rolloutLoader`.

**Likely files:**
- `internal/provider/codex/{load.go,incremental.go,load_test.go}`

Work:

- Move onto `transcriptbuild`. The state holds:
  - the assistant ID map, which can reach any earlier assistant;
  - the `current` turn and the model;
  - tool locations;
  - DeepEqual replay suppression;
  - line numbers;
  - the continuity mark.
- Use the resolution of the time: IT-06 replaces it. A changed canonical path
  forces a rebuild.
- Live status (`meta.Live`) is applied at publish.

**Done when** the harness passes on PF-02 Codex fixtures and on small fixtures
covering:
- split and replayed assistants;
- reasoning summaries;
- earlier outputs;
- unknown-call notices;
- compaction;
- turn aborts;
- the historic envelope-less form;
- a tail.

## IT-06 — Incremental Codex resolution

**Status:** complete 2026-10-25; [evidence](../perf.md#it-06--incremental-codex-resolution-2026-10-25). Each resolution takes the full canonical set from `Scan(ctx, prev)` instead of folding Changed / Removed, so it equals a cold scan by construction.

**Goal:** remove the cold all-rollout scan from every Codex load.

**Likely files:**
- `internal/provider/codex/{resolve.go,load.go,source.go}`
- tests

Work:

- **Private resolver.** It keeps the last `ScanState` and the canonical metas by
  ref.
  - It is mutex-serialized; concurrent resolutions coalesce on one scan.
  - Each resolution runs `Scan(ctx, prev)`, applies Changed / Removed, and keeps
    the `indexRolloutPath` recheck of the selected path.
- **Retained-byte cap.** It covers checkpoints plus metas. Over the cap, or after
  any scan error, it drops its state and falls back to the cold scan used today.
  Cancellation does not corrupt state: it is replaced only after a successful scan.
- **Equivalence matrix** against `Scan(ScanState{})`, run after each step:
  - duplicates (active vs archive, newer `UpdatedAt`);
  - index overlays and an invalid index row;
  - edge changes;
  - removed and replaced files;
  - a higher-numbered `state_N.sqlite`;
  - WAL updates;
  - an added duplicate;
  - truncation.

**Done when:**
- work counts show a warm resolution reads 0 bytes of unchanged rollouts;
- the IT-01 scaling benchmark improves;
- the PF-07C production bypass and the `SourceLoader` behaviour are unchanged.

## IT-07 — Resumable engine cache entries

**Status:** complete 2026-10-25; [evidence](../perf.md#it-07--resumable-engine-cache-entries-2026-10-25). A successor never waits on a cancelled predecessor; it takes only that flight's unused state, so an uncooperative provider cannot delay a retry.

**Goal:** an append no longer forces a cold re-parse in the engine.

**Likely files:**
- `internal/engine/{transcript_cache.go,service.go}`
- `transcript_cache_test.go`

Work:

- Entries hold their snapshot and provider state.
- `evict` from `onChanged` marks the entry stale and detaches the flight, keeping
  the PF-04 guarantee: no snapshot older than an invalidation is returned after it.
  Removed refs, `Forget` and `clear()` still drop the entry.
- A stale load resumes. A new flight waits for a detached predecessor that owns
  the state, without holding a slot, and then resumes from its result. If the
  predecessor fails or is cancelled, it falls back to cold.
- Wire `loadTranscript` through `provider.LoadIncremental`.

**Done when:**
- barrier tests cover: stale resume; invalidation during a resume; predecessor
  success, failure and cancellation; shutdown; restart; and all PF-04/PF-05
  regressions;
- work counts show one cold parse, then resumes, across a burst of appends.

## IT-08 — Byte budget and revisions

**Status:** complete 2026-10-25; [evidence](../perf.md#it-08--byte-budget-and-revisions-2026-10-25). `changedSince` takes the target revision as well as the base, so a publish between a load and its delta cannot widen the range. Revisions are per entry, not per load: a reader of a detached flight gets its snapshot with no revision, which the delta API treats as a reset.

**Goal:** bounded memory and the change history the delta API needs.

**Likely files:**
- `internal/engine/transcript_cache.go`
- `internal/model` (estimate)
- tests

Work:

- Entry bytes are the incremental snapshot estimate plus `RetainedBytes`. Keep the
  count cap, and add a total budget (default from IT-01, 256 MiB planned). Evict
  LRU-first, with the MRU entry exempt.
- Opaque revision strings: process epoch, entry generation and sequence.
- A 16-entry history ring per entry. `ChangedSince(base) (from int, ok bool)`
  takes the minimum `FirstChanged` after the base. Unknown, evicted or
  older-generation revisions give `ok = false`.

**Done when:** tests cover:
- the budget with mixed sizes;
- an oversized MRU entry;
- estimates matching the full recompute after random resumes;
- ring overflow, then reset;
- a rebuild giving 0;
- clear and restart changing the epoch.

## IT-09 — Shared FTS loading

**Status:** complete 2026-10-25; [evidence](../perf.md#it-09--shared-fts-loading-2026-10-25). An index load detaches a foreground flight already running for the session, since it may have read the source before the job was queued; that flight's readers get their snapshot without a revision. Providers without incremental loading parse in full and bypass the cache.

**Goal:** FTS reuses incremental state without polluting the foreground cache.

**Likely files:**
- `internal/index/worker.go`
- `internal/engine/{cache.go,service.go,transcript_cache.go}`

Work:

- An `index.TranscriptLoader` interface, injected into
  `StartIndexer`/`IndexPending`. It defaults to provider `Load`. `index` does not
  import `engine`.
- `Service.LoadForIndex` always validates by resuming to the current EOF. It
  advances an existing foreground entry; otherwise it uses one background slot
  with its own byte cap. A foreground hit promotes the background entry.
- The FTS revision transaction checks are unchanged.

**Done when:** tests cover:
- the FTS result is never older than the queued scan revision, including when a
  cached foreground entry exists;
- background jobs never evict foreground entries;
- repeated jobs for one live session resume (work counts);
- index package tests still pass with the default loader.

## IT-10 — Transcript delta API

**Status:** complete 2026-10-25; [evidence](../perf.md#it-10--transcript-delta-api-2026-10-25).

**Goal:** a compatible, capability-gated delta method.

**Likely files:**
- `internal/engine/{backend.go,types.go,service.go}`
- `internal/app/{app.go,offline.go}`
- `internal/rpc/{types.go,client.go,server.go}`
- `cmd/agent-sessions-cli/serve.go`
- Wails bindings
- `frontend/src/lib/{api.ts,types.ts}`

Work:

- `MessagesPage.Revision` (`omitempty`) on all pages.
- `GetMessagesSince(ctx, MessagesSinceRequest) (MessagesDelta, error)` on the
  Service and an optional Backend interface.
- The RPC method `getMessagesSince` and the capability `transcriptDelta`.
- **Client gating.** A missing capability and `CodeMethodNotFound` both map to
  `ErrUnsupportedAction`. The App maps that to the frontend's unsupported error,
  following the `discoveryStatus` pattern.
- Validation is the same as `GetMessages`, plus non-negative `have` fields.

**Done when:**
- a randomized test applies deltas to a client-side window over appends and
  earlier changes, and the result always equals a fresh full page;
- there are tests for old hosts and missing methods;
- the dispatcher round-trip passes;
- no limit is grown beyond today's request.

## IT-11 — Frontend delta refresh

**Status:** complete 2026-10-25; [evidence](../perf.md#it-11--frontend-delta-refresh-2026-10-25).

**Goal:** a live refresh fetches only the changes.

**Likely files:**
- `frontend/src/lib/components/transcript/TranscriptView.svelte`
- `frontend/src/lib/{api.ts,mock/mockApi.ts}`
- `frontend/test/`

Work:

- `reloadMessages` uses `getMessagesSince` with `{revision, pageOffset,
  messages.length}`. It splices `Reused` messages plus the reply, and keeps:
  following and anchoring; load generation and pending guards; search jumps;
  keyed-by-ID rendering; translations.
- **Fallback.** An unsupported reply switches that connection permanently to
  `getMessages`. A reset reply replaces the window.
- A "load earlier" or "load more" page whose revision differs from the window's
  triggers a delta refresh.
- The mock implements a revision history.

**Done when:**
- tests cover: a tail append while following; a reading-history window; an earlier
  tool-result change; a reset; an unsupported host; superseded reloads;
  mixed-revision pages;
- `npm test`, `check` and `build` pass;
- native GUI is unperformed unless it was actually done.

## IT-12 — Integrated gate

**Status:** complete 2026-10-25; [evidence](../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25).

**Goal:** before/after evidence and closure.

Work:

- Repeat the IT-01 benchmarks and the PF-02 benchmark.
- Run the full race suite, go test/build/vet, `GOOS=windows go vet`,
  `gofumpt`, and frontend test/check/build.
- Record the results, the limits (undetected same-length in-place rewrite;
  O‍penCode still loaded in full; FTS still rewrites documents; native GUI/SSH
  unperformed) and the remaining bottlenecks in `docs/perf.md`.

**Done when:** every acceptance gate in the design is checked or explicitly
reported as not met, and the trackers are updated.

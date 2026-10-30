# Performance Milestone 2 — Incremental transcripts

Status: complete 2026-10-25; IT-01…IT-12 complete (12/12);
[gate evidence](../perf.md#it-12--integrated-evidence-and-milestone-gate-2026-10-25).
[Task briefs IT-01…IT-12](../tasks/performance-m2.md) ·
[Performance roadmap](performance.md) · [Task tracker](../../TASKS.md#performance-milestone-2).

## Problem

Measured in M1 ([PF-15](../perf.md#pf-15--integrated-evidence-and-milestone-gate-2026-10-24)),
confirmed by reading the current code:

1. **Every change re-parses the whole transcript.** `Engine.onChanged` evicts a
   changed session (`Service.EvictTranscripts`). The next `GetMessages` runs the
   whole provider `Load` again: ~1.0 s (Claude) or ~1.6 s (Codex) and 0.5–0.6 GB
   allocated for a 100 MB file. For a live session this happens on every append
   event the user watches.
2. **A Codex load scans every rollout.** `codex.rolloutFor` calls
   `Scan(ctx, ScanState{})`: a cold scan reading every byte of every rollout on the
   machine, to choose the canonical source of one session. It then reads the
   transcript again. The PF-02 fixtures have one rollout, so M1 measured this only as
   "every byte twice". With many rollouts it costs the size of the whole store.
3. **FTS indexing re-parses live sessions.** `index.Indexer.processJob` calls
   `Provider.Load` for every queued revision. A live session gets a job every
   0.5–2 s, and each job parses the whole file again, independently of the
   engine's cache.
4. **Live updates re-send the whole window.** On `catalog:changed`,
   `TranscriptView.reloadMessages` fetches the whole loaded window again (at least
   50 messages, each with up to 64 KiB of tool output) to show one appended record.
   Over SSH this is a full payload every few seconds.
5. **The cache has no memory bound.** Four entries, no byte budget; ~400 MB with
   four 100 MB transcripts.

## Goals and non-goals

Goals:

- An append costs work proportional to the appended bytes, plus the copy-on-write
  of the messages it changes. It does not cost a re-parse of the file.
- Resolving a Codex session costs an incremental scan: unchanged rollouts are
  stat-only and are not re-read.
- FTS reuses that incremental state without growing the foreground cache.
- Live refresh sends only the messages that changed since the client's revision.
  Old hosts and old clients keep today's behaviour.
- Transcript memory has a byte budget, and the result is no less correct than a
  cold `Load`.

Non-goals:

- Incremental FTS document replacement: FTS still rewrites a session's documents
  (M4 owns FTS batching).
- Incremental O‍penCode loading. O‍penCode is SQLite-based and keeps its full `Load`.
- Transcript virtualization (M5).
- Reviving the PF-07 full-byte proof, or using `SessionMeta` hints as authority.

## Design

### 1. Provider contract (`internal/provider`)

There is a new optional in-process interface. It is not a wire capability.

```go
// TranscriptState is provider-private resumable parser state. Callers treat it
// as opaque, hand it back at most once, and never use it after that.
type TranscriptState interface{ RetainedBytes() int64 }

type IncrementalResult struct {
    Transcript   *model.Transcript // immutable once returned
    State        TranscriptState   // nil: do not resume from this result
    FirstChanged int               // earliest index that may differ from prev's snapshot
    Resumed      bool              // prev was continued, not rebuilt
}

type IncrementalLoader interface {
    LoadIncremental(ctx context.Context, ref model.SessionRef, prev TranscriptState) (IncrementalResult, error)
}

// LoadIncremental uses IncrementalLoader when present and otherwise wraps Load
// as a rebuild with FirstChanged 0.
func LoadIncremental(ctx, p Provider, ref, prev TranscriptState) (IncrementalResult, error)
```

Contract:

- **Same canonical result.** `LoadIncremental` with any prev must equal a fresh
  `Load` of the same bytes: messages, nested parts, tool calls and metadata.
  Canonical source selection happens on every call; a prev never authorizes a path.
- **Ownership.** prev is consumed. The provider may mutate it in place, and after
  an error it is invalid. A failed or cancelled call means the next load is cold.
- **FirstChanged.** For `i < FirstChanged`, `Messages[i]` equals the previous
  snapshot's `Messages[i]`. `len(prev)` means append-only. A rebuild gives 0.
  The value is conservative: it may be lower than the true first difference, but
  never higher.
- **Immutable snapshots.** A returned transcript and everything it points to
  (`Parts`, `*ToolCall`, `*FileRef`, `*SessionRef`, `Files`, `Input` bytes) is
  never written again. It is not enough to copy the message slice shallowly, because
  a later tool result or split assistant record writes into older messages.
- **Continuity.** A provider resumes only when all of these hold:
  - the canonical path is unchanged;
  - `os.SameFile` holds;
  - `size >= committed offset`;
  - the same size does not come with a new mtime (the scanner's rewrite rule);
  - a hash of the first ≤4 KiB, and of the last ≤4 KiB before the committed
    offset, matches.

  Otherwise it rebuilds. This is strictly stronger than the scanner checkpoints
  the catalog already trusts. One limit is documented and tested: an in-place
  rewrite that keeps both windows and the length is not detected. Detecting it
  would mean re-reading the prefix, which defeats the purpose. The agents
  append to these files and never rewrite them in place.
- **Best-effort tail.** A valid unterminated final record is shown, as today,
  but never committed. The committed offset stays at the end of the last complete
  line. The tail is applied to a fork of the state, which is discarded after
  publishing. On resume, the completed line is parsed normally.
- **Publish-time finalization.** These are recomputed when a snapshot is
  published, not stored as parser state:
  - live status and pending → unknown tool status;
  - Claude child-session links from sidecars;
  - subagent `meta.json`;
  - CWD / repository metadata.

  The state records which messages each of these touched. When a value changes,
  `FirstChanged` is lowered to the earliest affected message.
- **Bounded state.** When a provider's `RetainedBytes` would exceed its cap, it
  returns `State == nil` and the next load is cold.

### 2. Copy-on-write message store (`internal/provider/transcriptbuild`)

There is one shared builder, so that Claude and Codex cannot each get immutability
subtly wrong.

- `Append(msg) int` and `Touch(i) *model.Message`. Touching a message frozen in a
  published snapshot copies its `Parts`, its `ToolCall`s and its `FileRef`s first.
  On the first touch below the frozen length since the last publish, the builder
  also gets a fresh backing array: one O(n) struct copy per publish, never per
  record.
- `ToolLoc` / `Tool(loc)` replace the parser maps that held raw `*ToolCall`
  pointers. A pointer would keep writing into the frozen copy.
- `Freeze()` returns a clipped `[]model.Message` (cap == len) and the lowest
  index touched since the last freeze.
- `Fork()` is used for the best-effort tail, and costs one copy plus map clones.
  It is used only when a valid unterminated tail exists.

### 3. Claude and Codex resumable loaders

- **Claude.** The loader becomes resumable state over `transcriptbuild`:
  - the current split assistant index and ID, the tool locations and the open
    compaction part;
  - the scan `checkpoint`, which is already resumable, cloned for the tail fork;
  - the diagnostics line base.

  Earlier messages change through `tool_result` pairing, split assistant parts,
  `Tokens` and `Model`, compaction summaries, and pending → unknown status. Each of
  these goes through `Touch`.
- **Codex.** The same work for `rolloutLoader`:
  - the assistant ID map: split continuations can reach any earlier assistant;
  - the `current` turn, the model, tool locations and `reflect.DeepEqual` replay
    suppression.
- **Codex resolution without a cold full scan.** The provider keeps a private
  resolver: its last `ScanState` and its canonical metas. Each resolution runs
  `Scan(ctx, prev)` and applies Changed / Removed.
  - Unchanged rollouts are only stat'ed, and appended ones read only their new
    bytes.
  - The resolver relies on the incremental-scan equivalence the catalog already
    trusts. It does not trust a caller-supplied `SessionMeta`.
  - It is serialized behind a mutex, so concurrent resolutions share one scan.
  - It has a retained-byte cap. Over the cap, or on error, it falls back to the
    current cold scan.
  - Standalone `Load` uses the same resolver, so this also removes the double read
    for FTS and for remote hosts.

### 4. Engine cache (`internal/engine/transcript_cache.go`)

The PF-04/PF-05 coordinator keeps its token, waiter, slot and lifetime rules.
An entry becomes `{snap, rev, state, stale, bytes, history, background}`.

- **Invalidate marks stale, keeps state.** Eviction from `onChanged` marks the entry
  stale and detaches the current flight. As before, that flight never publishes
  as fresh. A load after invalidation never returns a snapshot older than the
  invalidation; PF-04's guarantee stays. Removal (`Forget`, removed refs) and
  `clear()` still drop everything.
- **Resume flights.** A load of a stale entry resumes from its state. A flight that
  starts while a detached predecessor still owns the state waits for that flight,
  without holding a slot, then resumes from its result. If the predecessor fails
  or is cancelled, the new flight loads cold. A burst of appends therefore never
  starts a second cold parse.
- **Byte budget.** Each entry's bytes are its snapshot estimate plus
  `RetainedBytes`. The estimate is updated incrementally: subtract the old
  suffix from `FirstChanged`, add the new one. The entry count stays capped, and
  there is also a total byte budget. Least-recently-used entries are evicted
  until the cache fits.
  - The most-recently-used entry is never evicted for size: a huge session the
    user is reading must not re-parse on every scroll.
  - IT-01 measurements set the default budget. 256 MiB is planned.
- **Revisions.** Each publish gets an opaque revision: a random process epoch,
  the entry generation and a sequence. Each entry keeps a short ring (16 entries)
  of `(rev, FirstChanged)`. For a base revision, the earliest changed index is the
  minimum over all later publishes. A revision that is unknown, evicted or from an
  earlier generation means reset.

### 5. FTS shared loading

- `index` gets an injected interface,
  `TranscriptLoader{ LoadForIndex(ctx, ref) (*model.Transcript, error) }`. When no
  loader is injected it uses provider `Load`. The engine injects its service.
  `index` does not import `engine`.
- `LoadForIndex` always validates: it resumes and reads to the current EOF, so its
  result is never older than the scan that queued the job. It never returns a hit
  without checking it first.
- FTS keeps ownership of its revision checks and of the DB metadata it compares,
  as PF-06 requires.
- A background load may advance an existing foreground entry. Otherwise it uses one
  separate background slot with its own byte cap, so background jobs do not evict
  sessions the user opened. Repeated jobs for a live session that is not open then
  resume instead of re-parsing.
- A foreground request for a session in the background slot promotes it.

### 6. Delta wire API

`GetMessagesSince` is a new Backend, App and RPC method, gated by the capability
`transcriptDelta`.

```go
type MessagesSinceRequest struct {
    Ref          model.SessionRef
    BaseRevision string // the revision the client's window came from; "" = none
    HaveOffset   int    // the client's window: [HaveOffset, HaveOffset+HaveCount)
    HaveCount    int
    Offset       int    // the window wanted, as in GetMessages (-1 = latest)
    Limit        int
}
type MessagesDelta struct {
    MessagesPage        // window and total of the new window, and Revision
    Reused       int    // the first Reused messages of the window equal the client's copies
}
```

- **Server.** Computes `c`, the earliest index changed since the base, and the new
  window `[o, e)`. If `o >= HaveOffset`, then
  `Reused = clamp(min(c, HaveOffset+HaveCount, e) - o, 0)`. Otherwise
  `Reused = 0`. `Messages` holds window indices `[o+Reused, e)`.
- **Client.** New window =
  `old.slice(o-HaveOffset, o-HaveOffset+Reused).concat(Messages)`.
- **Resets.** An unknown base or a rebuild gives `Reused = 0`, which is a full
  page as today.
- **Revision on pages.** `MessagesPage` gains an additive `revision,omitempty`
  field. The frontend uses it to detect "load earlier" pages from a newer
  revision, then refreshes its window by delta.
- **Compatibility.**
  - Old RPC hosts don't advertise the capability and return `CodeMethodNotFound`.
    The client maps both to `ErrUnsupportedAction`.
  - The frontend then falls back to `getMessages` for that connection.
  - Old frontends never call the method.

### 7. Frontend

- `TranscriptView.reloadMessages` calls `getMessagesSince` with its revision and
  window. It splices the reply in, and keeps the follow/anchor scroll, load
  generation, search jump and keyed-by-ID rendering.
- The mock backend implements the delta. Tests cover: tail append, an earlier
  tool-result change, rebuild/reset, an unsupported host, and a superseded
  reload.

## Correctness gates

- **Split-point equivalence.** For each generated fixture, resuming from every
  committed line boundary (sampled on large fixtures, exhaustive on small ones)
  must give a transcript deep-equal to a cold `Load` of the whole file. The test
  also checks that `FirstChanged` is never too high.
- **Immutability.** Deep-copy every published snapshot, resume several times, and
  compare. Also run race-detector tests with readers active during resumes.
- **Continuity matrix.** Cases: truncation, replacement (new inode), same-size
  rewrite, a tail-window or head-window change, a rename to a new path, a
  duplicate Codex rollout, an index overlay or edge change, a removed source, a
  live → ended status change, and new or removed subagent sidecars. Each must
  rebuild or match a cold load.
- **Cache.** Barrier tests for:
  - stale resume;
  - predecessor chaining and its cancellation;
  - the PF-04 invalidation race;
  - the budget, with the MRU entry exempt;
  - background slot isolation;
  - revision ring reset.
- **Wire.** Capability gating; old-host fallback; delta == full-page equivalence
  under a randomized append/mutate sequence.

## Measurements

IT-01 adds append-after-load benchmarks. IT-12 repeats them. Each workload
reports ms/op, B/op, source bytes, resolution bytes, retained bytes and payload
bytes.

| Workload | Expected after M2 |
|---|---|
| Claude/Codex 38/100 MB, append 1/10/100 records then reload | appended bytes only |
| Codex resolution with 64 extra rollouts, cold vs. warm | warm reads 0 unchanged rollout bytes |
| FTS job after an append | resume |
| Live window refresh payload, 50-message window plus 1 append | ~1 message |
| Retained cache bytes with four 100 MB sessions | within budget |

## Risks

- **COW mistakes.** These would silently corrupt published snapshots. Mitigation:
  one shared builder, immutability tests and race tests on both providers.
- **Over-high `FirstChanged`.** This would make clients keep stale messages.
  Mitigation: the equivalence harness checks it against a full diff, and resets
  are always safe.
- **Resolver divergence from a cold scan.** Mitigation: the PF-07B-style mutation
  matrix compares the resolver against `Scan(ScanState{})`, and the resolver falls
  back to a cold scan on any error.
- **Undetected same-length in-place rewrite.** A documented limit (see
  Continuity), shared with the existing scanner checkpoints.

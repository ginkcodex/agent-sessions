# AGY-12 — Provider review fixes: report keys, late child links, scan continuity, renamed transcripts

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** [AGY-03](AGY-03.md), [AGY-09](AGY-09.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Branch review of `internal/provider/antigravity`:

1. **Report Blob keys.** Each subagent report added a segment to the call's
   Blob key with no limit, but `Blob` rejects a key of more than
   `maxSegments` (64). A subagent with 64 or more reports got an
   `OutputRef` that could not be fetched.
2. **Late child links.** The child transcript check ran once, when the
   `invoke_subagent` result was parsed. If the child's transcript appeared
   later, incremental loads never linked it.
3. **Scan continuity.** `scanSource` resumed from the old offset whenever
   the file had grown, with no check that the bytes it had read were
   unchanged.
4. **Renamed transcripts.** When a session moved from `transcript.jsonl` to
   `transcript_full.jsonl` and the old file stayed, the old entry was kept
   with a warning on every scan.

Also: the `QuotePOSIX` doc comment in `internal/remote/quoting.go`, which
gofumpt had turned into curly quotes in `d492267`, was fixed in its own
commit (`55655da`).

## Acceptance criteria

- A call with more than 63 reports has a key `Blob` serves. The output
  says that the later reports are left out.
- A child whose transcript appears later is linked on the next incremental
  load, and `FirstChanged` covers its message.
- A larger transcript whose read bytes changed is scanned from the start.
- A renamed transcript gives no warning and no stale state, and its session
  is not removed.

## Evidence

Done:

- `parse.go`: when a report would push a key past `maxSegments` segments,
  the key ends in `,more` and stops growing. The inline output still
  appends every report, clipped at 64 KiB as before. `load.go` `Blob` strips
  the `,more` marker, still checks the segment limit, and adds "[Later
  subagent reports are left out here. Open the subagent's session to read
  them.]". A key of only `rec:more` is rejected.
- `parse.go`: the result step only records the child. `linkChildren` runs
  on every `LoadIncremental`, for the committed parse and for the tail fork.
  It links the calls with no `Child` whose child transcript now exists, and
  `Tool` marks their message changed.
- `internal/provider/continuity.go`: `PrefixDigest(ctx, path, offset)` hashes
  the same head and committed-tail windows (4 KiB each) as a
  `ContinuityMark`, and returns them as text.
- `scan.go`: the checkpoint has a `Prefix` field, and `checkpointVersion` is
  now 2, so older checkpoints rebuild once.
  - A grown file resumes only if its digest at the old offset matches.
  - Each read stores the digest at the new offset. If the digest fails, the
    next growth rebuilds.
  - File identity is not part of the digest: the checkpoint outlives the
    process, and an `os.FileInfo` does not serialize.
- `scan.go` `Scan`: an old path whose session is tracked under its other
  transcript name is dropped before the `Lstat` check. This is the review's
  suggested change. The later "not removed" check is no longer needed.

Tests:

- `load_test.go`:
  - `TestLoadIncrementalLinksLateChild`: resumed, `FirstChanged` 1, child
    linked, and the earlier snapshot is untouched.
  - `TestLoadCapsReportSegments`: 69 reports give a key with 64 segments
    plus `,more`. `Blob` serves 63 reports and the closing note, and
    rejects `rec:more`.
- `scan_test.go`:
  - `TestScanRebuildsReplacedTranscript`: a grown file with a new head is
    rescanned. It gets the new file's counts and first prompt, not
    `first` + `more`.
  - `TestScanTranscriptRenamed`: over two scans after the rename there are
    no warnings, nothing is removed, and only the new path keeps state.
  - `TestScanIncrementalAppend` now garbles the middle of a file longer than
    both windows, so it still shows that a resumed scan does not reread
    consumed bytes. It garbled the head before, which the digest now
    catches.
- Against the old `scan.go`, the replaced and renamed tests fail.
- `go test -race ./internal/provider/ ./internal/provider/antigravity/`: ok.
- `golangci-lint run` on both packages: 3 findings, all older and outside
  the changed lines (`continuity_test.go:123,126`, `continuity.go:128`).
  `golangci-lint fmt` applied to the new test only.

Not checked:

- A real agy session with 64 or more reports from one subagent.
- A replacement that keeps both 4 KiB windows the same is not detected.
  This is the same limit as `ContinuityMark` without its file identity
  check.

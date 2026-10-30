# Agent Sessions MVP Performance & QA Report

**Date:** 2026-09-28  
**Milestone:** M1 MVP QA Pass (M1-28)  
**Host Environment:** Ubuntu 26.04 LTS x86_64, Linux kernel 7.0.0-34-generic, Go 1.26.8, WebKitGTK 4.1  
**Scanned Data:** Live local environments for Claude Code (`~/.claude`), OpenAI Codex CLI (`~/.codex`), and OpenCode (`~/.local/share/opencode`).

---

## 1. SPEC §8 Performance Targets & Results

| Metric | SPEC §8 Target | Measured Value | Status | Notes |
|---|---|---|---|---|
| **Cold first scan** | < 5,000 ms (≈300 MB history) | **485 ms** | **PASS** (10.3× faster) | Scans all 3 providers concurrently; 68 total sessions found |
| **Warm scan / re-scan** | < 1,000 ms | **120 ms** | **PASS** (8.3× faster) | Catalog state reuse; unchanged files skipped |
| **Loading 38 MB session** | < 1,000 ms to first screen | **629 ms** (full parse) | **PASS** (1.6× faster) | 37.2 MB JSONL (`695fb531...`, 2,321 msgs, 6,058 records) parsed in 629 ms; paged first 50 msgs displayed immediately |
| **Session list scroll FPS** | 60 FPS (1,000+ items) | **60 FPS** | **PASS** | Fixed-height `VirtualList` caps active DOM items to ~25 |
| **Transcript scroll FPS** | 60 FPS (large session) | **60 FPS** | **PASS** | Progressive pagination (50 msgs/page) + DOM load-on-scroll |
| **Desktop binary size** | < 30 MB ELF | **13.4 MB** | **PASS** | Complete self-contained desktop binary with embedded frontend assets |
| **Memory usage (idle)** | < 150 MB RSS | **~48 MB** | **PASS** | Go backend + WebKitGTK idle runtime |
| **Memory usage (38 MB view)** | < 250 MB RSS | **~112 MB** | **PASS** | Controlled by 4-entry transcript LRU cache |

---

## 2. Ingestion & Diagnostics Breakdown

Scanned across detected local providers:

```
AGENT        PRESENT  ROOTS                              GENERATIONS           SESSIONS  PARSE ERRORS
claude-code  true     ~/.claude                          projects/*.jsonl            31             0
codex        true     ~/.codex                           state:state_5.sqlite         1             0
opencode     true     ~/.local/share/opencode            v2, v1                      36             0
TOTAL                                                                                68             0
```

- **Parse Errors:** `0` errors across all providers.
- **Diagnostic Notices:** Claude Code logged 89 unknown non-conversational event types (`atis-latch`, `cost-state`, `queue-operation`), counted without crashing the scan or polluting transcripts.
- **Total Messages Indexed:** 22,710 across all 68 sessions.

---

## 3. Large Session Transcript Profiling

Tested against the largest local session:
- **File:** `~/.claude/projects/-home-haith-Workspaces-ginkcode-new-vimoon/695fb531-6661-4157-8042-50faff27f74b.jsonl`
- **File size:** 37.2 MB (39,010,488 bytes)
- **Turn records:** 6,058 raw JSON records
- **Consolidated messages:** 2,321 (split assistant content blocks merged, tool calls paired)
- **Token usage:** 56,929,190 tokens
- **Full Parse & Consolidate Duration:** 629 ms
- **First Page Retrieval (50 messages):** 631 ms on cold load, < 2 ms on LRU hit
- **Paging / Slicing Duration:** < 1 ms per page

Tested against large OpenCode SQLite session:
- **Session:** `ses_f6614946bffexSe3jwGrQ2j625`
- **Messages:** 789 messages (1,686 database parts)
- **Load Duration:** 185 ms

Tested against Codex session:
- **Session:** `01a0e61e-e703-75a2-bc1d-6349e32f6dd4`
- **Messages:** 2 messages (rollout JSONL)
- **Load Duration:** 6 ms

---

## 4. UI Rendering & Virtualization Verification

1. **VirtualList DOM Nodes:**
   - With 68 local sessions (and tested up to 2,000 synthetic sessions), the rendered row count in DOM is bounded between 20 and 28 elements.
   - `ResizeObserver` on container keeps scroll metrics accurate during splitter drag operations.

2. **Progressive Transcript Loading:**
   - 50 messages per page.
   - Load-on-scroll triggers when user scrolls within 250px of the bottom.
   - Stale-selection guards prevent out-of-order race conditions when switching sessions rapidly.

3. **Zero-Dependency Markdown & Highlighting:**
   - Regex GFM renderer with strict HTML entity escaping for all untrusted text.
   - Syntax highlighter outputs CSS classes (`hljs-*`), allowing instant light/dark mode switching with zero re-parsing.
   - Container-level click delegation for copy buttons handles arbitrarily long code listings with no per-block event listeners.

4. **WebView Security Hardening:**
   - CSP: `default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self' ipc: wails:; frame-src 'none'; object-src 'none'; base-uri 'none';`
   - External links intercepted and dispatched via native OS browser opener (`xdg-open` / `wruntime.BrowserOpenURL`), with fail-closed scheme validation.

---

## 5. Verification Commands

Run the automated benchmarking script:
```bash
./scripts/qa-perf-bench.sh
```

Run test suite:
```bash
go test -race -count=1 ./...
cd frontend && npm test && npm run check
```

Build release desktop binary:
```bash
make gui-build
./build/bin/agent-sessions
```


---

## PF-01 — Synthetic catalog and scanner baseline (2026-10-24)

This is a new current-code baseline, **not** a rerun or validation of the historical
MVP/private-store results above. Only generated `testing.TB.TempDir()` provider
roots and explicit SQLite cache directories were used. No real session stores,
private caches, QA performance script, GUI or native package were used.

### Environment and method

- Revision: `035ef9f` plus uncommitted PF-01 code/docs; branch `feature/editor-essentials`.
- Apple M5, 32 GiB RAM; Darwin 27.0.0, arm64; Go 1.26.8, CGO enabled,
  default GOMAXPROCS 10. Installed Node 22.23.2 used for frontend checks.
- Three independent benchmark repetitions, `-benchtime=200ms -benchmem`.
  Fixture construction, database seeding and append writes are outside timers.
  Database close is excluded from open/open-restore measurements.
- Catalog: 1,000 / 10,000 unique deterministic metadata rows (counts, tokens,
  timestamps, model, prompt, CWD/repository and source fields). Roundtrip tests
  compare every restored field and catalog ordering. Restore includes
  `LoadCatalog → NewCatalog → Reset → All`; open includes existing DB integrity
  checks, not initial schema creation. Open-restore combines both paths.
- Each provider: 64 generated files, 32 user records per file. Claude totals
  250,624 bytes / 2,048 records. Codex adds one session_meta per file: 346,688
  bytes / 2,112 records. No index/sidecars/live sessions in the timed fixture;
  metadata-change correctness/work accounting is tested separately.
- Cold means no prior ScanState, **not** flushed OS disk caches. Unchanged uses
  the initial state. Append adds one newline-terminated user record to one file,
  then repeatedly scans from the *same pre-append snapshot*, so every iteration
  measures exactly one pending append without unbounded fixture growth.
- `ScanWork` is opt-in/context-local, atomic across workers, and contains no paths
  or session contents. Counts are logical transcript open attempts, consumed
  records/record bytes (excluding newline), and checkpoint JSON transformations.
  They do not measure kernel reads, physical I/O, discovery/index/sidecar bytes,
  retained heap, or wall time with accounting disabled. No schema/API change.

Reproduce safely (do not use the historical QA script above):

```sh
go test ./internal/index ./internal/provider ./internal/provider/claude ./internal/provider/codex
go test ./internal/index ./internal/provider -run '^$' -bench '^BenchmarkSynthetic' -benchmem -benchtime=200ms -count=3
```

### Measured summary

Times below are median [min–max] across three repetitions. B/op and allocations
are ranges, **allocated work**, not retained memory or RSS. Absolute timings are
advisory and are never CI assertions.

| Workload | ms/op median [min–max] | B/op range | allocs/op range |
|---|---:|---:|---:|
| Catalog 1k open | 1.547 [1.528–1.563] | 20,222–20,314 | 188 |
| Catalog 1k restore | 4.284 [4.283–4.468] | 4,707,500–4,707,724 | 21,034 |
| Catalog 1k open-restore | 6.007 [5.966–6.008] | 4,729,615–4,729,681 | 21,226–21,227 |
| Catalog 10k open | 12.527 [12.327–14.831] | 20,265–20,397 | 188–189 |
| Catalog 10k restore | 59.952 [53.453–67.491] | 53,223,618–53,223,670 | 210,083 |
| Catalog 10k open-restore | 68.646 [68.028–71.628] | 53,245,445–53,245,706 | 210,273–210,276 |
| Claude cold | 3.724 [3.719–3.734] | 67,738,318–67,744,977 | 11,722–11,745 |
| Claude unchanged | 0.269 [0.266–0.273] | 188,053–188,070 | 1,337 |
| Claude one-source append | 0.373 [0.366–0.374] | 1,242,470–1,242,571 | 1,364 |
| Codex cold | 6.044 [6.016–6.047] | 68,764,572–68,767,334 | 17,852–17,869 |
| Codex unchanged | 3.103 [3.101–3.117] | 1,356,366–1,356,785 | 10,745–10,746 |
| Codex one-source append | 3.335 [3.225–3.356] | 2,423,647–2,424,677 | 10,765–10,769 |

| Provider / mode | Transcript opens | Records | Record bytes | Checkpoint decodes | Checkpoint encodes |
|---|---:|---:|---:|---:|---:|
| Claude cold | 64 | 2,048 | 248,576 | 0 | 64 |
| Claude unchanged | 0 | 0 | 0 | 64 | 0 |
| Claude append | 1 | 1 | 122 | 64 | 1 |
| Codex cold | 64 | 2,112 | 344,576 | 64 | 128 |
| Codex unchanged | 0 | 0 | 0 | 192 | 64 |
| Codex append | 1 | 1 | 165 | 192 | 65 |

### Correctness, verification and limits

- New synthetic scan tests assert initial counts/prompts, zero unchanged changes,
  exactly one appended user count, no second append effect, source cardinality,
  diagnostics, and exact open/record/checkpoint work counts.
- Existing `TestScanSubagentAndMetadataChange` now asserts sidecar-only update work
  (1 open, 0 records, 2 decodes, 1 encode); metadata/count/token/removal parity
  remains checked. Existing Codex changed-title/index-disappearance test asserts
  index-only work (0 opens/records, 3 decodes, 1 encode). Existing edge, duplicate,
  archive, index-fallback, checkpoint roundtrip and resume-boundary tests pass.
- New active-session regression uses only its own process PID and temporary
  live-session file: initial overlay, status change and removal are observed via
  the existing Live channel without transcript reparsing. It intentionally does
  not change unchanged-Scan's existing emission semantics.
- Passed: focused packages; affected `go test -race`; `go test ./...`;
  `go vet ./...`; `go build ./...`; frontend tests (665 passed), check and build.
  Cached pinned gofumpt 0.12.0 used. Cached golangci-lint 2.14.0 full-repo run
  reports existing unrelated findings (including errcheck, unused helpers,
  refresh SA2001 and search suggestions); new task files have no remaining
  findings. No unrelated lint cleanup attempted.
- No native GUI/Windows execution, disk-cache flushing, retained-memory profiling,
  large/private history claims or performance optimization is implied. Synthetic
  sources are intentionally manageable; this baseline does not model transcript
  loading, thousands of source files, filesystem watcher setup or FTS work.
- Evidence points to unchanged Codex checkpoint canonicalization (three decode
  passes and one encode per source), Claude's one decode per unchanged source,
  and catalog restoration allocation scaling. Cold scans allocate roughly 1 MiB
  per opened source even on these small files; source-open allocation deserves
  later profiling, not an unmeasured fix inside PF-01.
- Recommended next task: **PF-02** transcript/load/concurrency baselines; retain
  this PF-01 evidence for PF-09/PF-10 before/after comparisons.

### Raw repeated benchmark output

```text
goos: darwin
goarch: arm64
pkg: github.com/ginkcode/agent-sessions/internal/index
cpu: Apple M5
BenchmarkSyntheticCatalog/1000/open-10    	     152	   1527958 ns/op	   20314 B/op	     188 allocs/op
BenchmarkSyntheticCatalog/1000/open-10    	     156	   1563197 ns/op	   20222 B/op	     188 allocs/op
BenchmarkSyntheticCatalog/1000/open-10    	     153	   1546931 ns/op	   20226 B/op	     188 allocs/op
BenchmarkSyntheticCatalog/1000/restore-10 	      48	   4468280 ns/op	 4707724 B/op	   21034 allocs/op
BenchmarkSyntheticCatalog/1000/restore-10 	      50	   4283617 ns/op	 4707500 B/op	   21034 allocs/op
BenchmarkSyntheticCatalog/1000/restore-10 	      51	   4282637 ns/op	 4707503 B/op	   21034 allocs/op
BenchmarkSyntheticCatalog/1000/open-restore-10         	      37	   5966387 ns/op	 4729654 B/op	   21227 allocs/op
BenchmarkSyntheticCatalog/1000/open-restore-10         	      37	   6008429 ns/op	 4729681 B/op	   21226 allocs/op
BenchmarkSyntheticCatalog/1000/open-restore-10         	      38	   6007123 ns/op	 4729615 B/op	   21227 allocs/op
BenchmarkSyntheticCatalog/10000/open-10                	      18	  12327315 ns/op	   20330 B/op	     188 allocs/op
BenchmarkSyntheticCatalog/10000/open-10                	      19	  12526763 ns/op	   20265 B/op	     188 allocs/op
BenchmarkSyntheticCatalog/10000/open-10                	      18	  14830718 ns/op	   20397 B/op	     189 allocs/op
BenchmarkSyntheticCatalog/10000/restore-10             	       4	  67491010 ns/op	53223670 B/op	  210083 allocs/op
BenchmarkSyntheticCatalog/10000/restore-10             	       4	  59951604 ns/op	53223642 B/op	  210083 allocs/op
BenchmarkSyntheticCatalog/10000/restore-10             	       4	  53452938 ns/op	53223618 B/op	  210083 allocs/op
BenchmarkSyntheticCatalog/10000/open-restore-10        	       3	  71628194 ns/op	53245706 B/op	  210276 allocs/op
BenchmarkSyntheticCatalog/10000/open-restore-10        	       3	  68027972 ns/op	53245498 B/op	  210275 allocs/op
BenchmarkSyntheticCatalog/10000/open-restore-10        	       3	  68645694 ns/op	53245445 B/op	  210273 allocs/op
PASS
ok  	github.com/ginkcode/agent-sessions/internal/index	27.438s
goos: darwin
goarch: arm64
pkg: github.com/ginkcode/agent-sessions/internal/provider
cpu: Apple M5
BenchmarkSyntheticScan/claude/cold-10    	      63	   3734303 ns/op	         0 cp-decodes/op	        64.00 cp-encodes/op	    250624 fixture-B	    248576 record-B/op	      2048 records/op	        64.00 source-opens/op	67744401 B/op	   11742 allocs/op
BenchmarkSyntheticScan/claude/cold-10    	      62	   3719340 ns/op	         0 cp-decodes/op	        64.00 cp-encodes/op	    250624 fixture-B	    248576 record-B/op	      2048 records/op	        64.00 source-opens/op	67744977 B/op	   11745 allocs/op
BenchmarkSyntheticScan/claude/cold-10    	      58	   3724175 ns/op	         0 cp-decodes/op	        64.00 cp-encodes/op	    250624 fixture-B	    248576 record-B/op	      2048 records/op	        64.00 source-opens/op	67738318 B/op	   11722 allocs/op
BenchmarkSyntheticScan/claude/unchanged-10         	     873	    265918 ns/op	        64.00 cp-decodes/op	         0 cp-encodes/op	    250624 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	  188067 B/op	    1337 allocs/op
BenchmarkSyntheticScan/claude/unchanged-10         	     908	    269040 ns/op	        64.00 cp-decodes/op	         0 cp-encodes/op	    250624 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	  188053 B/op	    1337 allocs/op
BenchmarkSyntheticScan/claude/unchanged-10         	     894	    273129 ns/op	        64.00 cp-decodes/op	         0 cp-encodes/op	    250624 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	  188070 B/op	    1337 allocs/op
BenchmarkSyntheticScan/claude/one-source-append-10 	     678	    373321 ns/op	        64.00 cp-decodes/op	         1.000 cp-encodes/op	    250624 fixture-B	       122.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 1242571 B/op	    1364 allocs/op
BenchmarkSyntheticScan/claude/one-source-append-10 	     668	    366249 ns/op	        64.00 cp-decodes/op	         1.000 cp-encodes/op	    250624 fixture-B	       122.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 1242500 B/op	    1364 allocs/op
BenchmarkSyntheticScan/claude/one-source-append-10 	     655	    373611 ns/op	        64.00 cp-decodes/op	         1.000 cp-encodes/op	    250624 fixture-B	       122.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 1242470 B/op	    1364 allocs/op
BenchmarkSyntheticScan/codex/cold-10               	      37	   6016338 ns/op	        64.00 cp-decodes/op	       128.0 cp-encodes/op	    346688 fixture-B	    344576 record-B/op	      2112 records/op	        64.00 source-opens/op	68764572 B/op	   17852 allocs/op
BenchmarkSyntheticScan/codex/cold-10               	      37	   6043628 ns/op	        64.00 cp-decodes/op	       128.0 cp-encodes/op	    346688 fixture-B	    344576 record-B/op	      2112 records/op	        64.00 source-opens/op	68767334 B/op	   17869 allocs/op
BenchmarkSyntheticScan/codex/cold-10               	      37	   6046916 ns/op	        64.00 cp-decodes/op	       128.0 cp-encodes/op	    346688 fixture-B	    344576 record-B/op	      2112 records/op	        64.00 source-opens/op	68766172 B/op	   17863 allocs/op
BenchmarkSyntheticScan/codex/unchanged-10          	      81	   3116727 ns/op	       192.0 cp-decodes/op	        64.00 cp-encodes/op	    346688 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	 1356719 B/op	   10746 allocs/op
BenchmarkSyntheticScan/codex/unchanged-10          	      81	   3100622 ns/op	       192.0 cp-decodes/op	        64.00 cp-encodes/op	    346688 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	 1356366 B/op	   10745 allocs/op
BenchmarkSyntheticScan/codex/unchanged-10          	      79	   3103301 ns/op	       192.0 cp-decodes/op	        64.00 cp-encodes/op	    346688 fixture-B	         0 record-B/op	         0 records/op	         0 source-opens/op	 1356785 B/op	   10746 allocs/op
BenchmarkSyntheticScan/codex/one-source-append-10  	      80	   3225017 ns/op	       192.0 cp-decodes/op	        65.00 cp-encodes/op	    346688 fixture-B	       165.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 2424677 B/op	   10769 allocs/op
BenchmarkSyntheticScan/codex/one-source-append-10  	      68	   3355997 ns/op	       192.0 cp-decodes/op	        65.00 cp-encodes/op	    346688 fixture-B	       165.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 2423647 B/op	   10765 allocs/op
BenchmarkSyntheticScan/codex/one-source-append-10  	      78	   3334757 ns/op	       192.0 cp-decodes/op	        65.00 cp-encodes/op	    346688 fixture-B	       165.0 record-B/op	         1.000 records/op	         1.000 source-opens/op	 2423656 B/op	   10766 allocs/op
PASS
ok  	github.com/ginkcode/agent-sessions/internal/provider	5.313s
```


## PF-02 — Synthetic transcript baseline (2026-10-24)

PF-02 complete; PF-01 evidence above is unchanged. No cache optimization.

### Method and correctness

- Revision `035ef9f` plus uncommitted PF-01/PF-02 work, Apple M5 / 32 GiB,
  Darwin 27.0.0 arm64, Go 1.26.8, default GOMAXPROCS 10.
- Generated temporary roots only; no private session stores, QA performance script,
  app launches, installs, or process termination. Each source has 256 turns / 512
  messages, split assistant records, 256 completed tool results, and a valid JSON
  final record without a newline adding an unanswered tool to the last assistant.
  Existing provider tests also cover malformed/truncated JSON tails.
- “38/100 MB” means decimal target prompt bytes plus valid envelope overhead, not
  MiB. All generated prompt bytes are checked, then complete message output is
  compared with the small equivalent after normalizing only prompt length.
  Identity/count/token/model/creation metadata is compared too; path, mtime and
  prompt-derived metadata intentionally differ. Cold/warm/invalidated pages match.
- Three repetitions, one operation each (`-benchtime=1x -count=3`). Final runs
  executed without other verification jobs; aborted exploratory runs are excluded.
  Generation, service/barrier setup, heap sampling and JSON marshaling are outside
  timers. Cold means provider parse/cache miss, **not** flushed filesystem caches.
  Warm is GetMessages only, not serialization/network/frontend rendering.
- Same-key and independent-key operations are batches of four concurrent misses.
  The reusable `providertest.Barrier` blocks all four before forwarding real Load
  calls, eliminating scheduler-dependent cache hits. Independent keys route to
  four separate generated roots through a test-only adapter. Batch timing includes
  goroutine/barrier scheduling; no wall-clock assertions. Deterministic tests
  release loads in reverse order and verify output and subsequent cache hits.
- `LoadWork` uses context-local atomics for public Load calls, successful transcript
  opens, and bytes delivered by file Read calls (including tail/newlines). This is
  **not physical disk I/O**; discovery/index/sidecar bytes are excluded. Existing
  `ScanWork` separately counts canonical-resolution record bytes (no newlines or
  tail), exposing Codex's extra full scan. No logging, content/path retention or
  production behavior/cache policy changes.

Reproduce safely:

```sh
go test ./internal/engine -run 'TestTranscript' -count=1
go test ./internal/provider/providertest -run TestBarrier -count=10
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscript$' -benchmem -benchtime=1x -count=3
go test -race ./internal/engine ./internal/provider/claude ./internal/provider/codex ./internal/provider/providertest -run 'Test(TranscriptBaselineEquivalence|Barrier|GetMessages|Load|.*Evict.*|.*Cache.*)' -count=1
go test -race ./internal/engine ./internal/provider/providertest -run 'Test(TranscriptConcurrentMissBaseline|Barrier)' -count=10
```

### Repeated timing and allocation summary

Median [min–max] of three single-operation samples; advisory, not CI thresholds.
B/op is cumulative allocation, not retained heap. Concurrent rows are per **batch**.
Warm nanosecond samples are especially noisy at 1x; the robust result is zero
provider calls/bytes and about 8 KiB of shallow page-copy allocation.

| Provider / target / mode | ms/op median [min–max] | B/op range | allocs/op range |
|---|---:|---:|---:|
| claude/38MB/provider-cold | 384.548666 [376.542875–384.669833] | 202,456,848–202,457,344 | 36,745–36,758 |
| claude/38MB/cold-page | 403.148375 [394.399917–403.654333] | 202,465,480–202,465,672 | 36,751–36,758 |
| claude/38MB/warm-page | 0.001959 [0.001708–0.002208] | 8,240–8,240 | 2–2 |
| claude/38MB/invalidated | 389.215041 [388.237875–395.956292] | 202,465,512–202,468,616 | 36,758–36,769 |
| claude/38MB/same-key-4 | 414.349875 [409.229541–417.494918] | 809,866,776–809,872,448 | 147,131–147,133 |
| claude/38MB/independent-key-4 | 422.546000 [414.389834–445.371250] | 809,868,056–809,870,232 | 147,114–147,162 |
| claude/100MB/provider-cold | 1044.725666 [1030.151334–1064.910041] | 506,781,888–506,783,088 | 36,747–36,754 |
| claude/100MB/cold-page | 1019.021541 [1005.775167–1019.639584] | 506,791,416–506,791,784 | 36,752–36,762 |
| claude/100MB/warm-page | 0.003542 [0.002792–0.005708] | 8,240–8,352 | 2–3 |
| claude/100MB/invalidated | 1040.826709 [1039.741458–1041.261791] | 506,791,448–506,792,856 | 36,757–36,767 |
| claude/100MB/same-key-4 | 1098.334917 [1091.758875–1102.974666] | 2,027,166,328–2,027,167,416 | 147,142–147,147 |
| claude/100MB/independent-key-4 | 1124.937625 [1120.778041–1131.795041] | 2,027,165,872–2,027,166,560 | 147,132–147,141 |
| codex/38MB/provider-cold | 641.262708 [639.532000–647.186375] | 244,256,768–244,269,936 | 50,124–50,133 |
| codex/38MB/cold-page | 595.952625 [592.217999–611.419292] | 244,261,672–244,281,336 | 50,123–50,144 |
| codex/38MB/warm-page | 0.002042 [0.001792–0.002333] | 8,240–8,240 | 2–2 |
| codex/38MB/invalidated | 614.305375 [609.924833–616.346833] | 244,278,072–244,281,912 | 50,136–50,144 |
| codex/38MB/same-key-4 | 689.029917 [688.688334–701.021292] | 977,095,120–977,109,600 | 200,545–200,562 |
| codex/38MB/independent-key-4 | 686.296583 [680.195292–708.728583] | 977,101,328–977,107,872 | 200,546–200,555 |
| codex/100MB/provider-cold | 1666.129208 [1665.711375–1668.671542] | 609,411,856–609,413,296 | 50,131–50,134 |
| codex/100MB/cold-page | 1663.465499 [1650.557167–1715.253459] | 609,420,376–609,420,856 | 50,136–50,137 |
| codex/100MB/warm-page | 0.003375 [0.002917–0.004000] | 8,240–8,240 | 2–2 |
| codex/100MB/invalidated | 1680.405708 [1661.063500–1684.713667] | 609,420,216–609,420,552 | 50,136–50,139 |
| codex/100MB/same-key-4 | 1867.674584 [1847.084918–1884.795458] | 2,437,668,032–2,437,669,824 | 200,547–200,554 |
| codex/100MB/independent-key-4 | 1897.435167 [1897.342249–1903.175999] | 2,437,670,224–2,437,677,744 | 200,543–200,562 |

### Work, payload and memory

Each cold/provider/invalidated operation asserts one Load/open and exactly one
source-file byte count; warm asserts zero; both four-request batches assert four.
Codex resolution record bytes are additional to transcript bytes, not a substitute.

| Provider / target | Source bytes / Load | Resolution record bytes / Load | Latest 50-message JSON bytes | Full transcript JSON bytes | Retained **estimate** bytes | Measured live HeapAlloc delta bytes |
|---|---:|---:|---:|---:|---:|---:|
| claude/38MB | 38,155,553 | 0 | 3,722,952 | 38,122,450 | 38,220,486 | 40,424,976 |
| claude/100MB | 100,155,681 | 0 | 9,777,652 | 100,122,579 | 100,220,614 | 101,691,920 |
| codex/38MB | 38,198,233 | 38,196,812 | 3,723,052 | 38,123,397 | 38,221,400 | 40,419,304 |
| codex/100MB | 100,198,361 | 100,196,940 | 9,777,752 | 100,123,526 | 100,221,528 | 101,698,776 |

- Retained estimate: struct/slice capacity plus message text, IDs and tool-content
  lengths using `unsafe.Sizeof`. Excludes allocator rounding, metadata strings,
  sharing, runtime/GC overhead and parse temporaries; not a measured heap size.
- Measured live heap: process-wide `runtime.MemStats.HeapAlloc` delta after forced
  GCs before/after one public Load, with the result kept alive. Sampled once per
  provider/size parent fixture, repeated as context on child rows (not three
  independent heap samples). This is noisy, not peak heap/RSS or cache capacity.
- Latest page retains shallow references into a full cached transcript. Four
  independent 100 MB entries can retain roughly four times the per-transcript
  estimate (~401 MB); this is an extrapolation, not a measured cache heap.
- Same-key misses currently do four loads and ~4× allocations, just like independent
  keys. At 100 MB that is ~2.03 GB Claude / ~2.44 GB Codex cumulative allocated work
  per four-request batch. Codex also scans ~400.8 MB of resolution records per batch.
  Warm pages avoid this work but still serialize ~9.78 MB for this text-heavy fixture.
  No claim about typical private histories or native GUI latency is implied.

### Verification and limits

Passed: focused engine/provider load/paging/barrier tests; large-equivalent tests;
focused affected race tests (including the large fixtures); barrier/concurrent
regressions repeated ten times under race; `go test ./...`; `go build ./...`;
`go vet ./...`; frontend tests (665 passed), check (0 errors, existing
ignoreDeprecations warning), and build. `git diff --check` passes.
No native GUI/Windows execution or peak memory profile. Existing dirty PF-01 files
and baselines were preserved. PF-03…PF-15 remain unimplemented; PF-04/PF-05 can reuse
the per-attempt barrier, cancellation context, ordered replies, failure replies,
ignore-cancellation mode and real-provider forwarding. Future coalescing changes
must update the explicitly named four-load baseline and its barrier expectations.

### Raw repeated benchmark output

```text
goos: darwin
goarch: arm64
pkg: github.com/ginkcode/agent-sessions/internal/engine
cpu: Apple M5
BenchmarkTranscript/claude/38MB/provider-cold-10         	       1	 376542875 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202456848 B/op	   36745 allocs/op
BenchmarkTranscript/claude/38MB/provider-cold-10         	       1	 384548666 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202457344 B/op	   36758 allocs/op
BenchmarkTranscript/claude/38MB/provider-cold-10         	       1	 384669833 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202457040 B/op	   36750 allocs/op
BenchmarkTranscript/claude/38MB/cold-page-10             	       1	 394399917 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202465592 B/op	   36754 allocs/op
BenchmarkTranscript/claude/38MB/cold-page-10             	       1	 403654333 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202465672 B/op	   36758 allocs/op
BenchmarkTranscript/claude/38MB/cold-page-10             	       1	 403148375 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202465480 B/op	   36751 allocs/op
BenchmarkTranscript/claude/38MB/warm-page-10             	       1	      1708 ns/op	  40424976 heap-live-B/transcript	         0 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38122450 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/claude/38MB/warm-page-10             	       1	      2208 ns/op	  40424976 heap-live-B/transcript	         0 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38122450 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/claude/38MB/warm-page-10             	       1	      1959 ns/op	  40424976 heap-live-B/transcript	         0 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38122450 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/claude/38MB/invalidated-10           	       1	 388237875 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202468616 B/op	   36769 allocs/op
BenchmarkTranscript/claude/38MB/invalidated-10           	       1	 389215041 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202465512 B/op	   36758 allocs/op
BenchmarkTranscript/claude/38MB/invalidated-10           	       1	 395956292 ns/op	  40424976 heap-live-B/transcript	         1.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	  38155553 source-B/op	         1.000 source-opens/op	  38122450 transcript-JSON-B	202465512 B/op	   36758 allocs/op
BenchmarkTranscript/claude/38MB/same-key-4-10            	       1	 414349875 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809872448 B/op	  147132 allocs/op
BenchmarkTranscript/claude/38MB/same-key-4-10            	       1	 409229541 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809867368 B/op	  147131 allocs/op
BenchmarkTranscript/claude/38MB/same-key-4-10            	       1	 417494918 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809866776 B/op	  147133 allocs/op
BenchmarkTranscript/claude/38MB/independent-key-4-10     	       1	 422546000 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809870232 B/op	  147146 allocs/op
BenchmarkTranscript/claude/38MB/independent-key-4-10     	       1	 445371250 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809868056 B/op	  147114 allocs/op
BenchmarkTranscript/claude/38MB/independent-key-4-10     	       1	 414389834 ns/op	  40424976 heap-live-B/transcript	         4.000 loads/op	   3722952 page-JSON-B	         0 resolve-record-B/op	  38220486 retained-est-B/transcript	 152622212 source-B/op	         4.000 source-opens/op	  38122450 transcript-JSON-B	809869704 B/op	  147162 allocs/op
BenchmarkTranscript/claude/100MB/provider-cold-10        	       1	1044725666 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506781888 B/op	   36754 allocs/op
BenchmarkTranscript/claude/100MB/provider-cold-10        	       1	1064910041 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506783088 B/op	   36752 allocs/op
BenchmarkTranscript/claude/100MB/provider-cold-10        	       1	1030151334 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506782896 B/op	   36747 allocs/op
BenchmarkTranscript/claude/100MB/cold-page-10            	       1	1005775167 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506791624 B/op	   36757 allocs/op
BenchmarkTranscript/claude/100MB/cold-page-10            	       1	1019021541 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506791784 B/op	   36762 allocs/op
BenchmarkTranscript/claude/100MB/cold-page-10            	       1	1019639584 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506791416 B/op	   36752 allocs/op
BenchmarkTranscript/claude/100MB/warm-page-10            	       1	      5708 ns/op	 101691920 heap-live-B/transcript	         0 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100122579 transcript-JSON-B	    8352 B/op	       3 allocs/op
BenchmarkTranscript/claude/100MB/warm-page-10            	       1	      2792 ns/op	 101691920 heap-live-B/transcript	         0 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100122579 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/claude/100MB/warm-page-10            	       1	      3542 ns/op	 101691920 heap-live-B/transcript	         0 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100122579 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/claude/100MB/invalidated-10          	       1	1041261791 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506792856 B/op	   36759 allocs/op
BenchmarkTranscript/claude/100MB/invalidated-10          	       1	1039741458 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506791832 B/op	   36767 allocs/op
BenchmarkTranscript/claude/100MB/invalidated-10          	       1	1040826709 ns/op	 101691920 heap-live-B/transcript	         1.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 100155681 source-B/op	         1.000 source-opens/op	 100122579 transcript-JSON-B	506791448 B/op	   36757 allocs/op
BenchmarkTranscript/claude/100MB/same-key-4-10           	       1	1091758875 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027167416 B/op	  147142 allocs/op
BenchmarkTranscript/claude/100MB/same-key-4-10           	       1	1098334917 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027166328 B/op	  147143 allocs/op
BenchmarkTranscript/claude/100MB/same-key-4-10           	       1	1102974666 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027167400 B/op	  147147 allocs/op
BenchmarkTranscript/claude/100MB/independent-key-4-10    	       1	1120778041 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027165872 B/op	  147141 allocs/op
BenchmarkTranscript/claude/100MB/independent-key-4-10    	       1	1131795041 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027166544 B/op	  147134 allocs/op
BenchmarkTranscript/claude/100MB/independent-key-4-10    	       1	1124937625 ns/op	 101691920 heap-live-B/transcript	         4.000 loads/op	   9777652 page-JSON-B	         0 resolve-record-B/op	 100220614 retained-est-B/transcript	 400622724 source-B/op	         4.000 source-opens/op	 100122579 transcript-JSON-B	2027166560 B/op	  147132 allocs/op
BenchmarkTranscript/codex/38MB/provider-cold-10          	       1	 641262708 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244256864 B/op	   50128 allocs/op
BenchmarkTranscript/codex/38MB/provider-cold-10          	       1	 639532000 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244269936 B/op	   50133 allocs/op
BenchmarkTranscript/codex/38MB/provider-cold-10          	       1	 647186375 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244256768 B/op	   50124 allocs/op
BenchmarkTranscript/codex/38MB/cold-page-10              	       1	 592217999 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244264440 B/op	   50128 allocs/op
BenchmarkTranscript/codex/38MB/cold-page-10              	       1	 595952625 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244281336 B/op	   50144 allocs/op
BenchmarkTranscript/codex/38MB/cold-page-10              	       1	 611419292 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244261672 B/op	   50123 allocs/op
BenchmarkTranscript/codex/38MB/warm-page-10              	       1	      2042 ns/op	  40419304 heap-live-B/transcript	         0 loads/op	   3723052 page-JSON-B	         0 resolve-record-B/op	  38221400 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38123397 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/38MB/warm-page-10              	       1	      2333 ns/op	  40419304 heap-live-B/transcript	         0 loads/op	   3723052 page-JSON-B	         0 resolve-record-B/op	  38221400 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38123397 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/38MB/warm-page-10              	       1	      1792 ns/op	  40419304 heap-live-B/transcript	         0 loads/op	   3723052 page-JSON-B	         0 resolve-record-B/op	  38221400 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	  38123397 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/38MB/invalidated-10            	       1	 609924833 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244281912 B/op	   50144 allocs/op
BenchmarkTranscript/codex/38MB/invalidated-10            	       1	 614305375 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244278072 B/op	   50136 allocs/op
BenchmarkTranscript/codex/38MB/invalidated-10            	       1	 616346833 ns/op	  40419304 heap-live-B/transcript	         1.000 loads/op	   3723052 page-JSON-B	  38196812 resolve-record-B/op	  38221400 retained-est-B/transcript	  38198233 source-B/op	         1.000 source-opens/op	  38123397 transcript-JSON-B	244281544 B/op	   50144 allocs/op
BenchmarkTranscript/codex/38MB/same-key-4-10             	       1	 701021292 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977103248 B/op	  200545 allocs/op
BenchmarkTranscript/codex/38MB/same-key-4-10             	       1	 689029917 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977095120 B/op	  200551 allocs/op
BenchmarkTranscript/codex/38MB/same-key-4-10             	       1	 688688334 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977109600 B/op	  200562 allocs/op
BenchmarkTranscript/codex/38MB/independent-key-4-10      	       1	 680195292 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977101328 B/op	  200546 allocs/op
BenchmarkTranscript/codex/38MB/independent-key-4-10      	       1	 686296583 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977107872 B/op	  200555 allocs/op
BenchmarkTranscript/codex/38MB/independent-key-4-10      	       1	 708728583 ns/op	  40419304 heap-live-B/transcript	         4.000 loads/op	   3723052 page-JSON-B	 152787248 resolve-record-B/op	  38221400 retained-est-B/transcript	 152792932 source-B/op	         4.000 source-opens/op	  38123397 transcript-JSON-B	977105632 B/op	  200554 allocs/op
BenchmarkTranscript/codex/100MB/provider-cold-10         	       1	1666129208 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609413296 B/op	   50134 allocs/op
BenchmarkTranscript/codex/100MB/provider-cold-10         	       1	1668671542 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609411856 B/op	   50131 allocs/op
BenchmarkTranscript/codex/100MB/provider-cold-10         	       1	1665711375 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609411856 B/op	   50131 allocs/op
BenchmarkTranscript/codex/100MB/cold-page-10             	       1	1663465499 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420392 B/op	   50136 allocs/op
BenchmarkTranscript/codex/100MB/cold-page-10             	       1	1715253459 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420856 B/op	   50137 allocs/op
BenchmarkTranscript/codex/100MB/cold-page-10             	       1	1650557167 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420376 B/op	   50136 allocs/op
BenchmarkTranscript/codex/100MB/warm-page-10             	       1	      4000 ns/op	 101698776 heap-live-B/transcript	         0 loads/op	   9777752 page-JSON-B	         0 resolve-record-B/op	 100221528 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100123526 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/100MB/warm-page-10             	       1	      2917 ns/op	 101698776 heap-live-B/transcript	         0 loads/op	   9777752 page-JSON-B	         0 resolve-record-B/op	 100221528 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100123526 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/100MB/warm-page-10             	       1	      3375 ns/op	 101698776 heap-live-B/transcript	         0 loads/op	   9777752 page-JSON-B	         0 resolve-record-B/op	 100221528 retained-est-B/transcript	         0 source-B/op	         0 source-opens/op	 100123526 transcript-JSON-B	    8240 B/op	       2 allocs/op
BenchmarkTranscript/codex/100MB/invalidated-10           	       1	1661063500 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420216 B/op	   50136 allocs/op
BenchmarkTranscript/codex/100MB/invalidated-10           	       1	1680405708 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420216 B/op	   50136 allocs/op
BenchmarkTranscript/codex/100MB/invalidated-10           	       1	1684713667 ns/op	 101698776 heap-live-B/transcript	         1.000 loads/op	   9777752 page-JSON-B	 100196940 resolve-record-B/op	 100221528 retained-est-B/transcript	 100198361 source-B/op	         1.000 source-opens/op	 100123526 transcript-JSON-B	609420552 B/op	   50139 allocs/op
BenchmarkTranscript/codex/100MB/same-key-4-10            	       1	1847084918 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437668032 B/op	  200554 allocs/op
BenchmarkTranscript/codex/100MB/same-key-4-10            	       1	1867674584 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437669136 B/op	  200547 allocs/op
BenchmarkTranscript/codex/100MB/same-key-4-10            	       1	1884795458 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437669824 B/op	  200550 allocs/op
BenchmarkTranscript/codex/100MB/independent-key-4-10     	       1	1897435167 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437677744 B/op	  200562 allocs/op
BenchmarkTranscript/codex/100MB/independent-key-4-10     	       1	1903175999 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437674160 B/op	  200546 allocs/op
BenchmarkTranscript/codex/100MB/independent-key-4-10     	       1	1897342249 ns/op	 101698776 heap-live-B/transcript	         4.000 loads/op	   9777752 page-JSON-B	 400787760 resolve-record-B/op	 100221528 retained-est-B/transcript	 400793444 source-B/op	         4.000 source-opens/op	 100123526 transcript-JSON-B	2437670224 B/op	  200543 allocs/op
PASS
ok  	github.com/ginkcode/agent-sessions/internal/engine	138.861s
```

## PF-03 — Synthetic project listing baseline (2026-10-24)

PF-03 complete; PF-01/PF-02 evidence above is unchanged. No listing optimization.

### Method and correctness

- Revision `035ef9f` plus uncommitted PF-03 work (PF-01/PF-02 committed during the
  task as `bb9eb49`/`e5bbc44`; files under test unchanged by them), Apple M5 / 32 GiB,
  Darwin 27.0.0 arm64, Go 1.26.8, Git 2.54.0 (Apple Git-157), GOMAXPROCS 10.
- `internal/files/listing_bench_test.go` generates a Git repository in `TempDir` per
  case, with Git global/system config disabled, `HOME`/`XDG_CONFIG_HOME` redirected
  and inherited `GIT_*` variables cleared. Tracked state is index-only (no commit).
  Each tree has 16 source directories, root and nested `.gitignore` (directory,
  glob and negation rules), an ignored directory tree, a deleted tracked file, an
  untracked file and empty directories. No user repository or session store is read.
- `TestSyntheticListingBaseline` covers 128 and 20,001 generated files: listed
  count/cap/truncation, deleted/untracked status, ignored and negated paths,
  sorted output, empty-directory inclusion, ignored-directory exclusion, a nested
  subroot listing, and that an uncounted context records no Git work.
  `TestSyntheticListingSymlinkIsolation` checks in/out-of-root directory and file
  symlinks stay leaves, `Resolve` refuses outside targets and search does not read
  them (skips when symlinks are unavailable). `TestSyntheticGitDirectoryCap` checks
  the Git directory walker's 20,000 cap. Git-unavailable cases skip.
- Git-count seam: an unexported context key whose callback runs before each `git`
  command in `runGit` and `ignoredDirectories`. Absent by default; request-local, so
  concurrent listings cannot pollute counts. Both sizes run 6 commands per `List`:
  `ls-files` ×2, `rev-parse` ×1, `status` ×1, `check-ignore` ×2 (one per directory
  batch, here root and `nested`).
- Baseline observation, not changed: `Search` uses `gitFiles` directly and does not
  apply `List`'s 20,000-file cap, so it is untruncated and finds paths beyond the
  listing boundary.
- Frontend: `file-drafts.test.mjs` "PF-03 listing baseline" uses the existing store
  mocks and deferred replies (no sleeps): reopening an already-open populated root
  makes 0 requests; close drops the listing but keeps tabs/drafts/expansion;
  reopen and refresh each make 1 request while refresh keeps the old snapshot; a
  closed root's late reply cannot populate another root; the same path on another
  host is refetched. This is request behavior, not native GUI latency.

### Results

`go test ./internal/files -run '^$' -bench 'BenchmarkProjectListing|BenchmarkSearch$' -benchtime=5x -count=5 -benchmem`,
then the `Status`/`Ignored` components separately with the same flags. Generation
is outside timers; filesystem caches are warm after setup (not a cold-disk run).
Time is min / median / max of five runs; bytes and allocations are medians.
`List` is the composite; components are measured independently and overlap it.

| Benchmark | ms/op | Git/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| 1000/List | 43.9 / 45.3 / 55.2 | 6 | 1,457,128 | 8,232 |
| 1000/GitFiles | 8.6 / 8.6 / 8.8 | 1 | 682,544 | 3,163 |
| 1000/Status | 14.0 / 14.2 / 25.8 | 2 | 305,681 | 250 |
| 1000/Ignored | 6.7 / 7.0 / 7.1 | 1 | 22,497 | 113 |
| 1000/Directories | 14.2 / 14.4 / 15.2 | 2 | 446,152 | 4,702 |
| 1000/Walk | 0.93 / 0.93 / 0.95 | 0 | 301,600 | 3,377 |
| 1000/Resolve | 0.017 / 0.018 / 0.019 | 0 | 7,808 | 85 |
| 20001/List | 151.3 / 152.3 / 153.2 | 6 | 24,903,505 | 142,959 |
| 20001/GitFiles | 50.5 / 51.2 / 53.4 | 1 | 13,369,676 | 60,304 |
| 20001/Status | 38.2 / 39.2 / 65.6 | 2 | 4,440,948 | 378 |
| 20001/Ignored | 13.3 / 13.5 / 13.7 | 1 | 22,632 | 113 |
| 20001/Directories | 47.3 / 47.7 / 49.1 | 2 | 7,068,936 | 82,161 |
| 20001/Walk | 13.4 / 13.8 / 14.6 | 0 | 6,400,214 | 60,440 |
| 20001/Resolve | 0.016 / 0.018 / 0.022 | 0 | 7,808 | 85 |
| Search (existing, 5,000 files) | 108.5 / 109.4 / 111.9 | — | 103,651,028 | 122,980 |

- At 20,001 files, Git file listing (≈51 ms, including one `Lstat` per path),
  directory discovery with `check-ignore` (≈48 ms) and status (≈39 ms) dominate
  `List`; ignored listing adds ≈13 ms. The non-Git walk is ≈14 ms and path
  validation is ≈18 µs. The first `Status` run of each size is a warm-up outlier.
- These guide PF-11/PF-13/PF-14; absolute timings are advisory and host-specific.

### Verification and limits

Passed: `go test ./internal/files -run TestSynthetic -v`; `go test -race ./internal/files`;
`make test` (full race suite); `go vet ./...`; `go build ./...`; gofumpt on changed
files; `golangci-lint run --new-from-rev=HEAD ./internal/files/...` (0 issues).
Package-wide `golangci-lint` on `internal/files` reports 39 existing issues in
untouched lines (errcheck, gofumpt, staticcheck, errorlint), not introduced here.
Frontend: `npm test` (666 passed), `npm run check` (0 errors, existing
ignoreDeprecations warning), `npm run build` (existing chunk-size warning).
No native GUI, Windows, or Linux runs; no cold-cache or peak-memory profile; no
symlink-unavailable or Git-unavailable host was exercised (those paths skip).

## PF-04 — Atomic transcript invalidation (2026-10-24)

### Design and deterministic evidence

`internal/engine/transcript_cache.go` wraps the existing LRU (capacity remains
four transcripts). One coordinator mutex covers cache lookup plus token capture,
per-key/batch eviction, clear and conditional publication. Provider I/O runs
outside that mutex. Unique, non-zero-sized per-key token objects identify current
outstanding loads; clear also advances an epoch. Invalidated requests may return
their snapshot, but cannot publish it or remove a newer key's token.

Bookkeeping contains only keys with current outstanding loads, not historical
keys or tombstones. Completion/error removes the last pending token; eviction
removes its key immediately; clear drops all tokens. Detached tokens live only
with their outstanding callers. Thus bookkeeping is bounded by outstanding work,
not by catalog size or the number of past invalidations. Active provider-work
limits and coalescing remain PF-05, not implemented or claimed here.

Barrier regressions in `internal/engine/transcript_cache_test.go` cover single
and batch eviction/clear between lookup and publication, both old/new completion
orders, removal/reappearance of the same key, snapshot delivery to old callers,
publication-before-eviction, and isolation from unrelated/empty invalidations.
Direct coordinator tests exercise 1,000 completed unique keys, 1,000 repeated
same-key invalidation/clear cycles, multiple pending same-key attempts, failure
cleanup, capacity/recency and useful publication after unrelated capacity churn.
Cancellation/retry and unknown-provider cleanup are covered. Existing concurrent
miss tests still require independent provider calls (no accidental coalescing).
Existing paging/LRU and generated 38/100 MB transcript equivalence tests pass.
No test uses sleeps as its ordering mechanism.

Lifecycle review: `NewService` initializes the coordinator for both engine and
direct service tests; all production reads use `loadTranscript`. Refresh/change,
removal, promotion and full-clear paths retain their existing invalidation entry
points. No engine-start requirement, new goroutine, owned context or shutdown
hook is introduced; provider cancellation remains owned by each caller.

### Verification and limits

Environment: Go 1.26.8, Darwin arm64. Passed focused transcript/provider-helper
race tests including generated large fixtures; paging and focused invalidation,
capacity, cancellation and concurrent-miss race tests repeated 20 times;
`gofumpt -d` on changed Go files; changed-line engine `golangci-lint` (0 issues).
Frontend tests: 666 passed; Svelte check and production build passed (existing
ignoreDeprecations/chunk-size warnings). `make test` (full backend race suite),
`go build ./...` and `go vet ./...` passed. Repository-wide `make lint` reports
121 existing issues in untouched lines (including errcheck, errorlint, gofumpt
and staticcheck); repository-wide changed-line lint also reports 0 issues. No unrelated lint cleanup
is included.

This task establishes correctness, not a measured speedup. No new benchmark
comparison or native GUI responsiveness claim; no private session/cache stores,
installs, app launches, process termination, pushes or Windows packages.


## PF-05 — Coalesced transcript loads (2026-10-24)

### Design and deterministic evidence

The transcript-specific coordinator still uses the existing four-entry LRU.
Current flight pointer identity replaces PF-04's explicit token/epoch: eviction
or clear detaches the pointer, and only the matching current flight can publish
or delete itself. Detached readers can finish with their original snapshot;
new readers never join obsolete work. No historical key tombstones are retained.

Shared contexts belong to the service/engine lifetime, never the first waiter.
Standalone `NewService` uses a background lifetime; its last departing waiter
cancels orphan work. Engine Start binds the owner even with caching disabled;
Close cancels current and detached loads, including before Start. Restart installs
a fresh lifetime; repeated Start is a no-op. Unknown-provider errors remain
synchronous. Caller cancellation is independent, and shutdown releases waiters
even when a provider ignores cancellation. Such a provider cannot be forcibly
terminated: it retains its slot and owned-flight record until it returns.

A four-slot context-aware semaphore bounds actual provider I/O, including
superseded/orphaned work. Queued flights are cancellable and do not hold the cache
lock. Bookkeeping is proportional to outstanding requests/unfinished loads, not
all keys ever seen; there is no admission/queue-size policy in this task.

PF-02 barriers cover four shared successful waiters, distinct simultaneous keys,
shared failure/retry, first/other/last cancellation, owner cancellation, both
old/new completion orders after single/batch/clear invalidation, orphan versus
retry identity, uncooperative shutdown completion, engine Close/restart and the
concurrency bound with a cancelled queued request. State observations use locked
predicates (with a timeout only as a failure guard), not sleeps. PF-02 concurrent
tests/benchmarks now expect one same-key provider attempt, four independent-key
attempts, so they do not wait for four announcements from one shared flight.
Synthetic work collectors are attached to the owning lifetime, not a waiter.

### Repeated same-key evidence

Darwin arm64, Apple M5, Go 1.26.8, default GOMAXPROCS=10; generated temporary
38/100 MB PF-02 fixtures, four page requests per cold same-key batch, construction
excluded. Three one-iteration runs, same host as PF-02. Command:

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscript/(claude|codex)/(38MB|100MB)/same-key-4$' -benchtime=1x -count=3
```

Historical before results are preserved in PF-02 above. Times are median
[min–max] milliseconds; allocation ranges are measured per four-request batch.

| Fixture | PF-02 before ms | PF-05 after ms | After B/op | After allocs/op |
|---|---:|---:|---:|---:|
| Claude 38 MB | 414.350 [409.230–417.495] | 392.843 [392.030–402.451] | 202,494,536–202,496,512 | 36,798–36,801 |
| Claude 100 MB | 1098.335 [1091.759–1102.975] | 1026.776 [1015.688–1043.684] | 506,818,920–506,822,400 | 36,801–36,809 |
| Codex 38 MB | 689.030 [688.688–701.021] | 590.582 [589.012–606.494] | 244,288,392–244,306,984 | 50,152–50,170 |
| Codex 100 MB | 1867.675 [1847.085–1884.795] | 1640.225 [1625.508–1653.801] | 609,430,536–609,447,688 | 50,152–50,167 |

All runs assert **4 → 1 loads and source opens per batch**, with logical source
bytes respectively 152,622,212 → 38,155,553; 400,622,724 → 100,155,681;
152,792,932 → 38,198,233; and 400,793,444 → 100,198,361. Allocations/bytes
are approximately one quarter of PF-02 (before B/op ranges in its table).
Codex resolution-record bytes also fall fourfold but still scan the source once:
38,196,812 / 100,196,940 after. Page JSON remains 3,722,952 / 9,777,652 bytes
for Claude, 3,723,052 / 9,777,752 for Codex. Measured single-transcript live
heap remains roughly 40.4 / 101.7 MB; retained-size estimates remain 38.2 / 100.2
MB, not a new cache-memory saving measurement.

Wall-clock improvement is modest relative to work/allocation reduction because
previous loads ran in parallel; these are synthetic one-shot batches, not a
native UI latency distribution, physical disk measurements or serial repeated
cache hits. Concurrent backend verification ran during these samples, adding
noise; no statistical speedup guarantee is claimed. Full parsing, Codex canonical
resolution, large payloads and the count-based cache budget remain unchanged.
A separate one-iteration Claude 38 MB run verifies cold-page, warm-page,
invalidated and independent-key-4 work-count expectations; it is not comparative
evidence for those modes.

### Verification and limits

Passed full `go test -race ./...`, `go test ./...`, `go build ./...`, `go vet ./...`;
focused coordinator race cases repeated 50 times; generated transcript equivalence
and paging regressions; changed-line engine `golangci-lint` (0 issues), and diff
whitespace checks. Frontend `npm test`, `npm run check`, `npm run build` passed.
An initial broad three-repeat race command with all large fixtures exceeded its
180-second aggregate timeout while parsing (not a barrier deadlock); the full
race suite passed with its normal timeout, and the focused repeated race run
excludes those expensive equivalence fixtures. Repository-wide lint debt recorded
in PF-04 is not changed or cleaned up here.

No native GUI/Windows/RPC latency claim. No private stores, QA perf script,
installs, app launches, process termination, pushes or Windows packages. PF-06
through PF-15 remain unimplemented; changes are left uncommitted for review.

## PF-06 — Validated source-loading contract (2026-10-24)

Added independent optional `SourceLoader` / `SourceBlobLoader` interfaces and
`LoadWithSource` / `BlobWithSource` dispatch helpers without changing Provider,
RPC, generated bindings or frontend protocols. Identity mismatch and unsupported
providers use standalone methods; wrapped `ErrStaleSource` triggers exactly one
canonical fallback. Other errors propagate, and cancellation prevents fallback.
The source value is an untrusted canonical metadata hint, not a capability.

[Contract and Codex feasibility](tasks/performance-m1.md#pf-06-contract-and-pf-07-feasibility)
records path/ref validation, canonical overlays, freshness, cancellation, Blob key
semantics, engine catalog ownership and FTS queued-revision ownership. Deep review
of discovery, index loading, canonical scan selection and Load/Blob found that
metadata alone cannot exclude unseen duplicates or prove index/WAL/parent
freshness. PF-07 must obtain sufficient provider-owned evidence or conservatively
re-resolve; file stat/UpdatedAt alone is not a valid shortcut. No performance gain,
zero-discovery guarantee, validation cache or fast-path implementation is claimed.

### Evidence and verification

- New dispatch tests count optional/base calls for success, mismatched ref, wrapped
  stale errors, ordinary failures, cancellation before dispatch and during stale
  rejection. Both Load and Blob are covered, including canonical fallback output.
- Generated temporary Codex fixtures test unsupported providers and a deliberately
  stale-only optional adapter. Outside-root/traversal/missing/duplicate hints and
  forged title overlays cannot alter canonical transcript/Blob output; invalid
  Blob keys and a source replaced by an outside-root symlink are rejected.
  This tests fallback safety, not a future accepting fast-path validator. PF-07
  must add accepting-path duplicate/index/freshness validation tests.
- Passed focused `go test ./internal/provider ./internal/provider/codex -run
  TestSource -count=1`; repeated source contract race tests with `-count=25`;
  `go test -race ./internal/provider/... -run
  'TestSource|TestLoad|TestBlob|TestIndex|TestScan' -count=1`.
- Passed `go test ./...`, `go build ./...`, `go vet ./...`; frontend `npm test`,
  `npm run check`, `npm run build`; cached pinned `go tool gofumpt`;
  `go tool golangci-lint run --new-from-rev=HEAD ./internal/provider/...` (0 issues)
  and `git diff --check`. Existing repository-wide lint debt remains as recorded
  in PF-04; no unrelated lint cleanup. Tool invocation used `GOPROXY=off`.

Only PF-06 is implemented. PF-07…PF-15 remain unimplemented; no engine/FTS caller
activation. No private stores, QA perf script, installs, process termination,
application launches, Windows packages, commits or pushes. Native GUI/manual QA
and Windows runtime verification are unperformed. Changes left for review.

## PF-07A — Bounded content proof and accepting Codex sources (2026-10-24)

Split PF-07 before coding into two ≤4 h packages; PF-07A complete, parent
**partial**, M1 still **6/15**. PF-07B retains the full mutation matrix and repeated
benchmark/retained-memory evidence; PF-08 remains blocked.

Only an optional cold source-aware call captures before/after reconciliation
evidence around one canonical Scan and publishes one private immutable snapshot
per Provider root under a mutex. Ordinary Scan and standalone Load/Blob are
unchanged and perform zero proof hashing, even after optional use. A cold call
serves its canonical resolution directly if proof cannot be established, avoiding
a second fallback Scan. Both
optional source methods accept matching canonical metadata only after rediscovery,
index reread (SQLite, not DB/WAL mtime), validated index-path classification and
streaming SHA-256 of every selectable source agree with the snapshot. Identity,
mode, size and mtime also agree, but are not sufficient alone. This preserves raw
duplicate and index/parent selection by reusing the existing canonical Scan, not
reimplementing its overlays. Changed/uncertain/unproven hints return stale for
PF-06's single standalone fallback. Incremental checkpoints never establish proof.

Limits: at most 4096 sources/index rows/edges, 512 MiB total rollout content per
proof and 8 MiB serialized index string/metadata payload retained; no rollout bytes
or parser checkpoints retained. Existing discovery/index readers still allocate
transient results before cap checks; this is not a bound on all Scan allocations.
Proof reads use 64 KiB chunks and context checks. Reconciliation still enumerates,
opens and reads unrelated sources; it eliminates unrelated JSON parsing, not disk
I/O. Optional cold resolution incurs two extra content passes; ordinary Scan incurs
none. Actual retained-memory measurement and larger repeated workloads remain
PF-07B. Normal
best-effort filesystem/SQLite read races remain; no atomic cross-source snapshot
or adversarial concurrent rewrite guarantee is claimed. Old snapshots may be
held by active callers, but provider ownership retains only one; concurrent old
publication cannot authorize changed content because each use revalidates.

Deterministic synthetic two-source test: standalone Load performs **2 metadata
source opens and nonzero record parses**; direct accepting Load plus Blob perform
**0 scan source opens, 0 records, 0 checkpoint decodes/encodes** with identical
output. Digest opens/bytes/passes/enumerations are counted separately by opt-in
Codex SourceWork; zero parse counters do not imply zero source I/O.
Tests include forged hints, fresh-provider fallback, cancellation, concurrent
readers, immutable old snapshots, oversized proof rejection, same-size/mtime
rewrite, replacement, disappearance, outside-root symlink, duplicate appearance,
index addition and title update. Existing Codex lazy-output/tail/index tests pass.
PF-07B must still exercise the complete dedicated WAL/parent/index-path mutation
matrix and concurrent Scan publication gates, rather than infer these from design.

Passed `go test ./...`, `go vet ./...`, `go build ./...`,
`go test -race ./internal/provider/codex ./internal/provider`, frontend `npm test`
(666 passed), `npm run check`, `npm run build`, cached `go tool gofumpt`,
`GOPROXY=off go tool golangci-lint run --new-from-rev=HEAD ./internal/provider/codex/...`
(0 issues), and `git diff --check` on Darwin
arm64 using installed tools. No private stores, forbidden script, installs,
app launches, process termination, commits or pushes. GUI/Windows runtime QA
unperformed. No engine/FTS wiring or checkpoint optimization.


### PF-07A lazy-proof correction and repeated microbenchmark

Removed proof work from ordinary Scan after review: only optional cold calls
establish evidence. `BenchmarkSourceReconciliation` uses 16 generated rollouts,
100 messages each, 212,614 total bytes. Apple M5/Darwin arm64, three runs,
`go test ./internal/provider/codex -run '^$' -bench BenchmarkSourceReconciliation
-benchmem -benchtime=200ms -count=3`, fixture generation excluded:

| Path | ms/op range | allocated B/op (approx) | proof bytes/op | proof passes/enumerations |
|---|---:|---:|---:|---:|
| Ordinary Scan before optional use | 1.654–1.658 | 17,293,000 | 0 | 0 |
| Ordinary Scan after optional use | 1.650–1.704 | 17,293,000 | 0 | 0 |
| Standalone Load | 1.945–2.007 | 18,521,000 | 0 | 0 |
| Optional cold Load | 3.379–3.420 | 18,888,000 | 425,228 | 2 |
| Optional proven warm Load | 0.952–0.958 | 1,404,884 | 212,614 | 1 |

Ordinary Scan is the unchanged pre-PF07 implementation, with no new wrapper;
these before/after labels compare absence/presence of an optional snapshot, not
historical independent binaries. Counters prove no new no-hint byte work; timings
are advisory. Optional cold is materially more expensive; warm avoids unrelated
parsing but still hashes every byte. Digest opens, actual read bytes (including
failed proofs), discovery passes, and index reads are explicitly accounted.
Discovery readability probes and SQLite internal page I/O remain outside digest
byte counters, as do canonical Scan reads (tracked separately by ScanWork).

## PF-07B — Mutation matrix, recovery and larger evidence (2026-10-24)

Implemented the dedicated synthetic Load/Blob matrix and retained-state evidence.
**Parent PF-07 remains partial; M1 stays 6/15 and PF-08 activation remains gated.**
The larger byte-heavy workload is slower even on proven warm hits, so these
results do not justify unconditional integration or a general speedup claim.

Correctness review found a permanent-reuse failure: an immutable stale snapshot
was never refreshed, including when a newly scanned hint differed from its cached
metadata. Validation now checks proof before hint equality, compare-and-clears
only the snapshot actually checked, and returns stale for the one canonical
fallback. The next optional call establishes fresh before/Scan/after evidence.
Cold publication carries a generation so an older in-flight establish cannot
replace newer evidence; a delayed validator cannot invalidate a newer snapshot.
Ordinary Scan and standalone Load/Blob neither invalidate nor establish proof.
An unstable or corrupt source may still prevent proof indefinitely by design.
There is no filesystem/SQLite atomicity claim or active caller cancellation.

Dedicated coverage now includes WAL-only title updates with an open writer,
numeric 9→10 selection, index removal/replacement/corruption, unconventional
index path override and ID mismatch, archive-column removal (raw archived state
restored), authoritative edge removal and parent disappearance/visibility, raw
newer/active-tie/lexical-tie duplicates beneath index timestamp overlays, append
recovery, existing disappearance/replacement/outside-symlink and same-stat rewrite
cases, lazy output and valid/invalid final tails. Each matrix branch compares
canonical standalone output, proves stale Load and Blob independently, rebuilds,
and accepts Load plus a real inline-file Blob without metadata parsing. Corrupt
indexes intentionally cannot retain proof. Deterministic late invalidation and
publication ordering plus concurrent ordinary Scan/validation exercise ownership;
source-count and byte caps reject proof. No production hook or private fixture.

### Isolated repeated larger workloads

Apple M5, Darwin arm64, generated eight-rollout fixtures, approximately 38 and
100 MiB total; mostly 8 KiB ignored world-state records plus a message per file.
This deliberately measures byte-heavy, low-parsing-cost sources, not typical
message-heavy data. Fixture construction and warm seeding excluded. Command:
`go test ./internal/provider/codex -run '^$' -bench '^BenchmarkSourceLarge$'
-benchmem -benchtime=1x -count=3`, isolated from broad tests. These are three
single-operation samples per mode, not steady-state statistics or cold disk I/O.

| Path | 38 MiB ms/op range | 100 MiB ms/op range | allocated MB/op (38 / 100) |
|---|---:|---:|---:|
| Ordinary Scan before optional use | 7.50–7.97 | 17.48–18.17 | 54.30 / 129.03 |
| Ordinary Scan after optional use | 7.30–8.02 | 17.96–18.99 | 54.30 / 129.02 |
| Standalone Load | 19.24–20.62 | 49.63–51.27 | 61.29 / 145.71 |
| Optional cold Load | 46.15–46.37 | 120.71–121.38 | 61.48 / 145.90 |
| Optional proven warm Load | 25.27–26.40 | 67.06–68.14 | 7.09 / 16.78 |
| Changed source + one fallback | 33.41–34.85 | 85.68–90.26 | 61.38 / 145.80 |

Exact full proof bytes/pass: **39,792,696 / 104,808,952**. Warm does one
full digest pass and **zero Scan bytes/records/opens**; cold does two passes plus
one Scan. Standalone and both ordinary Scan modes perform **zero proof work**,
8 Scan opens and **4,824 / 12,680** records (**39,787,872 / 104,796,272**
record bytes). Changed-mode samples append one blank line, invalidate proof and
perform one canonical fallback (8 Scan opens); fallback frequency is 100% for
these changed samples and 0% for stable warm samples. Subsequent cold rebuild
cost is not hidden in these changed samples. The benchmark also supports longer
runs, where alternating stale/rebuild calls are included in the changed average.
Digest counters include unrelated source bytes; Scan bytes count record payload
rather than newline bytes. Neither includes SQLite internal pages, readability
probe opens, or selected transcript parser bytes. Timings include those actual
operations; no claim of zero total I/O is made.

**Concrete activation gate/follow-up:** do not wire PF-08 unconditionally. Warm
hashing is about 25–37% slower here despite lower allocations; cold is >2×
standalone. Above 512 MiB or 4096 sources, no proof is retained and every optional
cold call may repeat proof attempts plus a full Scan. Before PF-08, establish an
explicit workload/size and mutation-rate policy with measured end-to-end benefit,
including cold amortization and over-cap bypass/backoff, or develop stronger
incremental freshness evidence in a separately scoped follow-up. No cheap
mtime-only trust, reduced canonicality, or ordinary-Scan hashing is acceptable.
PF-07B correctness/evidence work is implemented; parent performance acceptance
and integration suitability are not claimed complete.

A final three-repeat 200ms small-fixture rerun measured warm 1.003–1.013 ms
versus standalone 2.069–2.109 ms; ordinary before/after Scan 1.760–1.769 /
1.747–1.788 ms, zero proof work. This advisory rerun was launched while a full
race command was outstanding, so it is **not** part of the isolated acceptance
benchmark evidence above and is not used to justify integration.

### Actual retained heap sample and verification

`go test ./internal/provider/codex -run '^TestSourceRetainedHeapSample$' -v
-count=3`, run separately before benchmarks: after forced GC, keeping Provider
alive and dropping transcript, the 100 MiB/eight-source snapshot retained process
HeapAlloc deltas **52,504 / 15,192 / 18,488 bytes** (final test-cleanup rerun:
47,488 / 13,536 / 12,720 bytes). This process-global subtraction
includes caches/runtime noise; it is neither object attribution, a maximum nor
peak memory evidence. Source bytes are not retained. Existing 4096-source,
512 MiB-byte and 8 MiB serialized-index caps remain; transient discovery/index
allocations occur before bounds, multiple in-flight snapshots can coexist, and
canonical metadata retention is not independently byte-capped.

Passed focused source tests repeated three times, provider/Codex race tests
repeated three times, deterministic recovery/publication tests repeated ten times,
full `go test ./...` and `go test -race ./...`, `go vet ./...`, `go build ./...`, and
frontend `npm test` (666 passed), `npm run check`, `npm run build`, cached
`go tool gofumpt`, changed-line golangci-lint (0 issues), and `git diff --check`.
Frontend emitted Node experimental-loader/MockTimers and bundle-size warnings;
no test/check/build failures. Initial new-test failures
were fixture errors (non-discoverable duplicate directory, Darwin /var alias
comparison, and an invalid index row does not hide a discovered parent); corrected
fixtures/assertions and reruns pass. No installed tooling, private stores,
qa-perf-bench, application launches/kills, engine/FTS wiring, commits or pushes.

## PF-07C — Cost-policy decision and production bypass (2026-10-24)

PF-07C was documented in both trackers before coding (≤4 h package). **Safe
opt-out implemented; parent PF-07 remains partial/blocked, M1 6/15.** No automatic
eligibility policy is justified by the available evidence. PF-08 and dependent
PF-10 remain blocked; **PF-09 is the next independent package**, not implemented
here. This is a negative performance conclusion, not a universal speedup claim.

### Decision and exact supported workload

Cheap discovered source count and stat aggregate size can bound digest cost, but
cannot predict the cost of decoding ignored versus meaningful records, selected
transcript cost, or the number of future unchanged requests needed for amortization.
They also omit unconventional index-added paths unless index semantics are read.
Obtaining density by a new parse defeats cheap eligibility; canonical Scan's
existing request-local counters can describe records but not their decode
complexity or future reuse. Ordinary Scan must not gain extra parsing, hashing or
persistent policy machinery merely to classify this experiment. No cutoff chosen
between the small and large fixtures establishes a portable break-even rule.
Timing-adaptive activation would itself incur costly trial reads and needs further
scope/evidence across workload shapes, I/O and mutation rates.

Production therefore supports **no automatically eligible workload**: New leaves
the private experiment flag off; both optional methods immediately return stale
(after cancellation checking), even if evidence exists. The PF-06 dispatcher then
does exactly one canonical standalone Load/Blob. Zero proof attempts is the retry
bound, including large, over-cap, cold and rapidly changing roots. There is no
cooldown clock, sleep, periodic trial, hidden size preflight, public activation
switch, discovery or SQLite read in the decline. Ordinary Scan is unchanged.
This avoids the regression rather than asserting a learned policy. Any future
activation requires a separately documented measured package and parent acceptance.

The test-only experiment continues to support the measured **16 rollouts × 100
small messages, 212,614 bytes total**, stable unchanged warm Load case. It still
hashes all bytes and is not production eligibility. Experimental Blob may consume
an existing Load-established snapshot but now cannot initiate cold before/Scan/after
proof, including if evidence disappears between calls. Mutation/canonicality matrix
and publication-generation safeguards remain covered. Large experimental retained
heap evidence above remains historical experimental evidence, not production state.

### Isolated repeated evidence

Apple M5/Darwin arm64; synthetic fixtures only; no other test/race work was running.
Setup and experimental warm seeding excluded. Commands (each count=3):

```sh
go test ./internal/provider/codex -run '^$' -bench '^BenchmarkSourceReconciliation/(OrdinaryLoad|ProductionBypass|OptionalCold|OptionalWarm)$' -benchmem -benchtime=300ms -count=3
go test ./internal/provider/codex -run '^$' -bench '^BenchmarkSourceLarge/(38MiB|100MiB)/(StandaloneLoad|ProductionBypass)$' -benchmem -benchtime=3x -count=3
go test ./internal/provider/codex -run '^$' -bench '^BenchmarkSourceBlobPolicy$' -benchmem -benchtime=3x -count=3
```

| Load path | small ms/op | 38 MiB ms/op | 100 MiB ms/op |
|---|---:|---:|---:|
| Standalone | 1.946–1.997 | 22.364–22.471 | 55.821–57.476 |
| Production bypass + fallback | 1.964–1.998 | 22.304–22.435 | 56.002–58.207 |
| Experimental cold | 3.395–3.438 | historical above | historical above |
| Experimental warm | 0.952–0.994 | historical regression above | historical regression above |

Standalone/bypass allocations approximately 18.52 / 61.29 / 145.71 MB/op for
small/38/100. Experimental small cold 18.89 MB/op, warm 1.405 MB/op. Small cold
reads 425,228 proof bytes in two passes; warm 212,614 in one. The roughly 1.4 ms
extra small cold cost needs about two subsequent unchanged warm hits just to repay
on this host/fixture; mutation can erase that benefit. This is not a general
break-even estimate.

| Blob path (real HELLO attachment) | 1 MiB ms/op | 38 MiB ms/op | 100 MiB ms/op |
|---|---:|---:|---:|
| Standalone | 1.261–1.576 | 7.645–8.117 | 18.187–18.999 |
| Production bypass + fallback | 1.103–1.265 | 7.920–7.949 | 17.930–18.456 |

Blob allocations approximately 10.76 / 55.36 / 130.09 MB/op for both paths.
Three operations per large/Blob sample are advisory, not latency distributions or
cold-disk measurements; differences within these ranges are not speedup claims.
Bypass is approximately standalone in these repeated samples.

**No hidden byte work:** every bypass/standalone measured proof counter is zero.
Large Load canonical counts are identical: eight Scan opens, 4,824 / 12,680
records, 39,787,872 / 104,796,272 record bytes per operation. Blob standalone and
bypass both do eight Scan opens. Proof bytes do not include canonical Scan,
selected transcript reads, SQLite pages or readability probes; those ordinary
costs remain, rather than being described as zero I/O. Deterministic tests assert
all five SourceWork counters stay zero on repeated declines (including >512 MiB
sparse source and >4096 sources), production with a retained snapshot, rapid
appends, cancellation and concurrent dispatch. Three appended Load+Blob pairs do
exactly six canonical source opens in a one-file root, with canonical output;
experimental repeated cold Blob calls do no Scan or proof.

### Verification and boundaries

Passed `go test ./internal/provider/codex ./internal/provider -count=1`, focused
source race tests for both packages `-count=3`, `go test ./...`, `go vet ./...`,
`go build ./...`, gofmt and `git diff --check`. Frontend code was untouched;
frontend tests/check/build and full repository race were not rerun for PF-07C
(previous PF-07B results above are historical). No private stores/scripts, installs,
app launch/kill, extra agents, engine/FTS activation, commits or pushes.

## PF-09 — Claude checkpoint summary reuse (2026-10-24)

Implemented only PF-09. PF-07 remains partial/blocked; PF-08/PF-10 remain
blocked. No persisted checkpoint/version or parser changes. A provider-owned
summary caches only the validated unchanged decision and sidecar dependencies,
not decoded metadata, pending usage or pending tool slices. Exact copied raw
checkpoint bytes, Size/ModTime/Offset, transcript file identity and sidecar
path/size/mtime must match. Changed sources decode normally; cold/reset scans
parse normally. New providers validate persisted checkpoints once. The raw copy
prevents caller in-place edits from silently authorizing reuse; no content hash
or duplicate marshal is introduced. Existing best-effort stat-based freshness
semantics remain: same-inode content edits preserving size and mtime (including
sidecars) are not detected; this is not a content-proof feature.

Admission is at most 1024 summaries with raw checkpoints at most 16 KiB each
(16 MiB raw upper bound, plus structs/map/path/FileInfo storage). Oversized and
non-admitted sources use the old decode path. No LRU or parser snapshot is
retained. Discovery prunes removed/reset entries; concurrent scans can reduce
hit rates but cannot match different raw state. Locks guard cache bookkeeping;
caller mutation of ScanState concurrently with Scan remains unsupported.

### Repeated synthetic evidence

Darwin arm64, Go 1.26.8, same host as the immediately preceding baseline.
Generated PF-01 fixture: 64 sources × 32 records, 250,624 bytes. Generation is
outside timed work. Commands before and after:

```sh
go test ./internal/provider -run '^TestSyntheticScanBaseline$' -bench '^BenchmarkSyntheticScan/claude/' -benchmem -benchtime=200ms -count=3
go test ./internal/provider/claude -run 'TestCheckpointReuse' -v -count=3
```

| Mode | Before µs/op (3 runs) | After µs/op (3 runs) | Before → after B/op | Before → after allocs/op |
|---|---|---|---|---|
| cold | 3717, 3775, 3782 | 3860, 3889, 3858 | 67,741,388–67,745,958 → 67,796,368–67,799,104 | 11,733–11,751 → 11,807–11,817 |
| unchanged | 261, 257, 260 | 174, 172, 172 | 188,039–188,066 → 149,523–149,547 | 1337 → 444 |
| one-source append | 371, 369, 366 | 233, 232, 230 | 1,242,363–1,242,488 → 1,204,607–1,204,728 | 1363 → 484 |

Unchanged checkpoint decodes fall **64 → 0**, append **64 → 1**;
encodes/opens/records stay respectively 0/0/0 and 1/1/1. Cold stays
0 decodes, 64 encodes/opens, 2048 records. Approximate median improvements:
34% unchanged and 37% append, with roughly 2% cold overhead. Advisory warm
filesystem timings, not native latency claims. The append benchmark repeatedly
uses its fixed pre-append state; this intentionally measures the same one-record
resume each iteration, not an ever-growing transcript. Historical PF-01 exact
counts above are preserved; current assertions now require the reduced work.

### Retained-state cost

Measured summary struct size: 88 bytes. Basic fixture raw identity: 802 bytes;
125-byte temporary path; accounted payload 1015 bytes per entry excluding map,
allocator and FileInfo backing storage. GC/HeapAlloc deltas for 1024 summaries
with 802-byte synthetic raw identities: **1,105,936 / 1,146,096 / 1,105,920 bytes**.
This isolated measurement includes map/raw allocations but excludes preallocated
paths and a shared FileInfo; it is not whole-provider heap or worst-case usage.
Raw-state retention is capped rather than retaining a potentially huge parsed
checkpoint or its PendingTools allocation. Entry saturation is conservative
admission, so workloads above 1024 sources may see smaller gains.

### Verification and limitations

Passed full `go test ./...`, `go build ./...`, `go vet ./...`, affected
`go test -race ./internal/provider/claude ./internal/provider`, frontend
`npm test`, `npm run check`, `npm run build`, gofmt and diff whitespace checks.
Regressions cover versions (missing/old/future), corrupt and in-place edited raw
state, valid foreign state, reset/new provider, identical-stat inode replacement,
same-size rewrite, truncation, sidecar changes/removal, source removal, admission
bounds and concurrent scans. Existing every-fixture-boundary resume, split
usage/tool state and checkpoint roundtrip tests pass. Prior ScanState immutability
is checked. No private stores, forbidden script, installation, app launch/kill,
Windows package, extra agents, commits or pushes. Native GUI QA is unperformed.

## PF-11 — Bounded frontend listing snapshots (2026-10-24)

Frontend-only listing reuse, following PF-03's mocked-store baseline. No backend,
wire/API, editor-content cache or fresh-presence policy changes. The listing LRU
has **four entries**, keyed by captured data-host identity plus root (trailing
separators removed, Windows drive/UNC separators normalized; case and symlink
aliases deliberately not merged). Each retained snapshot is limited to **500,000
serialized UTF-16 code units**: at most 2,000,000 serialized units across the LRU,
not a measured JS heap-byte bound. Oversized replies still display, but replace
any older retained entry with no admission. Saved tabs/editors/drafts use their
existing independent store and are not subject to listing eviction.

Copied/frozen arrays and status objects back immutable `$state.raw` replacement.
Reopen publishes cached paths synchronously, then starts a fresh list independently
of editor reconciliation. Existing sequence/epoch checks remain, with captured
host/root checks before both visible and cached publication. Successful reset
clears listings; guarded draft resets remain guarded. Cached paths/status are
explicitly labelled retained/not yet verified until a successful response.
Refreshing and truncation remain visible, refresh errors retain browsable paths
plus an alert, and explicit refresh remains available after failure.

### Deterministic work counts and evidence

The extended PF-03 lifecycle regression still counts **0** new list requests for
already-open show, **1** for close/reopen, **1** for explicit refresh and **1** for
each new root/host. This is **not fewer backend requests**: before PF-11 close/
reopen displayed no paths until list resolution; now it immediately displays the
previous paths while performing the same refresh. A barrier test holds list,
presence probe and content read unresolved: cached paths are visible immediately,
**1 list + 1 probe** start without resolution; after a present probe, **1 read**
still occurs. No cached listing replaces those calls. Refresh replacement removes
old results rather than merging them; failure keeps hints without claiming success.

Tests also cover LRU recency/eviction, oversized replacement/non-admission,
independent draft/editor identity retention, immutable API ownership, root and
host isolation (including late old-host completion), Windows separator and trailing
separator normalization, same-root old-request and old-epoch rejection, loading,
truncation, refresh failure and retry presentation. The harness uses the existing
Svelte server module/component compiler and `test/resolver.mjs`; no new framework.

### Verification and limitations

From `frontend/`:

```sh
node --loader ./test/resolver.mjs --test test/file-drafts.test.mjs test/go-to-file.test.mjs
npm test
npm run check
npm run build
```

Focused **38/38** and full **671/671** tests pass (no skips). Check passes with
0 errors and the existing tsconfig `ignoreDeprecations` warning. Production build
passes with Vite's large-chunk advisory. Node reports its existing experimental-loader warning. Backend is unchanged;
Go tests/race/build/vet were **not rerun** for this frontend-only task. No native
GUI/Windows responsiveness, wall-clock improvement or retained heap measurement
is claimed. PF-12 remains next: comprehensive app-mutation/hidden-save/Git
invalidation and reconnect freshness regressions are not implemented here.
Snapshots are hints even when stale app mutations are not yet invalidated; every
reopen revalidates and existing fresh file probes/reads/revision checks remain.
No private data, installation, app launch/kill, extra agents, commits or pushes.

## PF-12 — Listing freshness and draft safety (2026-10-24)

Frontend-only follow-up to PF-11. No backend, wire/API, editor-content cache,
presence-probe or diagnostics change. Before coding, current mutation paths were
inspected (files store create/rename/delete/save/uncertain-save reconciliation and
replace reload; Git write/pull; replace in files; add file) and the scope fit 4 h,
so no split was needed.

### Policy

- `FilesStore.invalidateListings(root, host)` drops retained snapshots of that host
  whose normalized root is equal to, inside or containing `root`, compared
  case-insensitively (trailing/Windows separators normalized as in PF-11). Over-
  matching only discards a hint. Git writes and pulls pass no root and drop every
  snapshot of the host: commits, hooks and pulls can change status outside the
  visible root, and `repoRoot` is unaliased metadata.
- An overlapping visible listing on that host is superseded (`listSeq`), so a list
  request started before the mutation cannot publish or be retained, and it is
  labelled unverified; the caller (or `listingsChanged`) relists when not blocked.
- Applied on success **and** possibly-applied outcomes: create/rename/delete
  success, unknown outcome and partial delete; saves that succeed or become
  uncertain (including a reply after reconnect/host change) and acknowledged
  uncertain saves; replace in files (including hidden/overlapping roots and failed
  requests); add file (success or failure); Git stage/unstage/discard/commit/switch
  (also after failure, before reloads) and pull, even when the project closed or
  changed meanwhile. Hidden saves invalidate while the dialog is closed.
- Reconnect still relists the visible root; a blocked reconnect now labels the
  retained listing unverified instead of leaving it marked current.
- Tabs, drafts, conflict/uncertain-save state, undo versions, saved editor state and
  `reset()` draft guards are untouched. Unknown/failed probes remain non-deletion.

### Deterministic work counts and regressions

- Hidden save while an aliased visible root (`/p/` vs saved `/p`) lists: **2** list
  requests (reopen + post-save), the pre-save reply is rejected and only post-save
  status is retained; the hidden editor, draft and undo version are identical.
- Create, rename and delete each keep the visible root at **1** post-mutation list;
  an older in-flight refresh reply is rejected. Case-variant nested hidden roots
  lose their snapshot (reopen shows loading), unrelated roots keep theirs.
- Host-wide Git invalidation frees a superseded refresh so the next refresh is not
  skipped (**2** requests, old reply rejected). Previously a pre-mutation in-flight
  list caused the post-Git refresh to be skipped, publishing pre-write status as
  current. Mutations elsewhere do **0** relists; overlapping ones **1**.
- Git store: invalidation precedes reloads and refresh; a failed hook in a project
  closed meanwhile still invalidates and relists the visible same-host listing
  without reporting the old project's error. A host-switched sync only invalidates.
- Blocked replace reload still drops overlapping hidden snapshots. Windows case and
  separator aliases and other-host isolation are covered.
- Reconnect while blocked: **0** lists, listing unverified; after unblocking a failed
  relist and failed probe keep the tab, draft and version (no deletion claim). A save
  then drops the hint, so a failed reopen list shows no stale paths.
- Uncertain save after a host switch drops the old-host snapshot with **0** new-host
  lists; the draft stays protected.

Behavior notes for review: a same-host Git write or overlapping replace/add that
completes after the project changed now refreshes the visible project (list plus
read-only tab reconciliation). Failed replace requests and uncertain same-host saves
now relist; dropped snapshots make reopen show loading instead of retained paths.

### Verification and limitations

From `frontend/`:

```sh
node --loader ./test/resolver.mjs --test test/file-drafts.test.mjs test/git-changes.test.mjs \
  test/content-search.test.mjs test/go-to-file.test.mjs test/create-directory.test.mjs \
  test/rename-file.test.mjs test/delete-file.test.mjs
npm test
npm run check
npm run build
```

Focused **110/110** and full **677/677** tests pass (no skips; 6 new tests, 2
existing expectations updated for the new invalidation calls). Check passes with 0
errors and the existing tsconfig `ignoreDeprecations` warning; build passes with
Vite's large-chunk advisory. Node reports its existing experimental-loader warning.
Backend is unchanged; Go tests/race/build/vet were **not rerun**. The add-file
dialog path has no component test; its store call is covered directly. Symlink
aliases and external edits (terminal, other tools) are not detected; snapshots stay
hints that every reopen revalidates. No native GUI/Windows verification, timing or
heap claim. No private data, installation, app launch/kill, extra agents, commits or pushes.

## PF-13 — Bounded directory frontiers (2026-10-24)

Backend-only change to `internal/files/directories.go`. No listing API, frontend,
cache or Git-runner change.

### Policy

- `gitDirectories` now delegates to `walkDirectories`, which keeps reading unvisited
  directories (still in 128-entry `ReadDir` chunks) and collects their candidate
  subdirectories into one frontier. The frontier is decided by a single
  `check-ignore -z --stdin` when it reaches **128 paths or 64 KiB** of NUL-terminated
  input, or when no undecided directory remains. A full frontier is decided mid-read.
- Only survivors of a decision are queued, so no directory is opened before its own
  ignore decision. `.git` and symlinks are never candidates; empty directories are
  listed; one oversized path may form a batch alone.
- Unchanged limits: 20 s walk timeout (truncated), caller cancellation (error),
  `MaxFiles` directories and `MaxFiles*4` visited entries (truncated). Entries read
  before the visit cap are still decided and listed but never entered, as before. A
  failed decision truncates the result and stops the walk (fail closed).
- Untruncated results are identical to the previous walker. Under truncation the
  surviving subset may differ, because a frontier can span directories; only the
  `Truncated` flag and caps are contractual.

### Evidence

`internal/files/directories_frontier_test.go` keeps the previous per-chunk walker as
a test-only oracle:

- A generated tree (300-wide level, 40-deep chain, empty and ignored directories,
  repository metadata, inside/outside directory symlinks, files, and names with
  spaces, Unicode, leading `-`, `#`, `!`, brackets, `*`, plus `:`, newline, backslash
  and `?` off Windows) lists identically with frontier limits {1 path}, {2 paths},
  {7 paths, 64 B}, the production limit and an unbounded one. Every batch respects
  its limit and contains only children of already-decided, non-ignored directories.
- Failure on the second decision: truncated, no further checks, only the first
  batch's survivors listed. Cancellation from inside a decision returns
  `context.Canceled` with no result. A flat `MaxFiles*4 + 10` entry root matches the
  oracle's capped output without descent (skipped under `-short`).
- Real Git with nested `.gitignore` rules (`listingTree(300)` plus 40 nested
  `srcNN/subNN/leaf` chains and an ignored `nested/hidden/deeper`): identical output;
  **3** `check-ignore` processes versus **58** for the per-chunk oracle. Most real
  trees have few subdirectories per directory, so batching across directories is
  where processes are saved; PF-14 tunes budgets and covers the remaining matrix.
- PF-03 fixture (flat `srcNN` directories, so already 2 checks): unchanged counts.
  `BenchmarkProjectListing`, 20 iterations × 3 runs on the same Apple Silicon host:
  1000 files `Directories` 15.2–15.4 ms, `List` 52–57 ms; 20,001 files
  `Directories` 49–56 ms, `List` 156–164 ms; **2** and **6** Git processes per op.
  Advisory timings only, comparable with PF-03's 152 ms `List` median.

### Verification

```sh
go test -race -count=1 ./internal/files/
go vet ./internal/files/
go build ./...
go test -count=1 -run '^$' -bench 'ProjectListing/(1000|20001)/(Directories|List)$' -benchtime=20x ./internal/files/
```

All pass. Windows path behavior was not run on Windows; candidate paths still use
`filepath.ToSlash`. No real project, private data, installation, app launch or push.

## PF-14 — Batched ignore checks (2026-10-24)

Backend-only follow-up to PF-13 in `internal/files/{directories,files}.go`. No API,
frontend, cache or deleted-tracked filtering change; `MaxFiles` caps are unchanged.

### Policy

- The production frontier is **4096 paths or 256 KiB** of check-ignore input,
  whichever comes first. Input, echoed output and the candidate slice stay within a
  few hundred KiB; the 20 s per-process and per-walk timeouts are unchanged.
- `check-ignore` now runs through the shared Git runner (`gitOutput`, extracted from
  `runGit`): resolved binary, hidden console, unquoted paths. It cannot use
  `--literal-pathspecs` (Git rejects it for this command), so each path is sent as
  `./path` and only `./`-anchored echoes count as answers.
- **Bug fixed:** a top-level directory named like pathspec magic (`:!a`,
  `:(exclude)a`) made Git exit fatally, which failed closed and returned an
  **empty, truncated** directory list for the whole project; `:ign-x` was checked as
  `ign-x`. Larger batches would have widened that blast radius, so anchoring ships
  with them. A mutation test with anchoring removed reproduces the empty listing.
- Check-before-descent, fail-closed truncation, cancellation, `.git`/symlink
  exclusion and empty directories are unchanged from PF-13.

### Evidence (`internal/files/directories_ignore_test.go`, real Git, isolated config)

- Pathspec-like and unusual names, top-level and nested: `:!a`, `:(exclude)a`,
  `:(top)x`, `:/a`, `:^b`, `:colon`, `*star`, `[ab]`, `?q`, leading/trailing spaces,
  `-dash`, `#hash`, `!bang`, Unicode, plus newline, backslash and `"` off Windows;
  rules `:ign-x/`, `\*ign/`, `ign\?/` hide exactly their literal directories.
- Nested rules: anchored `/rootonly/` (nested copy listed), `vendor/` with a nested
  `!vendor/` re-include, `cache*/` with `!cache-keep/`, `build/` under `app/`, an
  ignore-all-files `*`/`!*/` directory, empty directories; an ignored parent's
  `!inner/` stays hidden because it is never entered. Output equals the PF-13 oracle
  with no more processes.
- Failure (corrupt `HEAD`): no answer, empty truncated listing. Cancellation: no
  answer and `context.Canceled` from the walk.
- `MaxFiles + 1` top-level directories: capped at `MaxFiles`, truncated, exactly
  **5** check-ignore processes.
- Wide tree (1000 directories × 1 child, 1010 entries at the root):
  `BenchmarkWideDirectories`, 5 iterations × 3 runs, same Apple Silicon host:

  | Strategy | check-ignore / walk | Time / walk | Bytes / walk |
  |---|---|---|---|
  | Per 128-entry chunk (pre-PF-13 oracle) | 1010 | 6.39–6.56 s | 26.6 MB |
  | PF-13 frontier (128 paths) | 16 | 147–149 ms | 5.2 MB |
  | PF-14 frontier (4096 paths) | **2** | **54–56 ms** | 5.0 MB |

  Same output in all three. The oracle uses the anchored checker, so the gap is
  structural, not the bug fix. The per-chunk cost was one process per directory that
  has subdirectories, which is the common shape of real source trees.
- PF-03 fixture (`BenchmarkProjectListing`, 20 × 3): 1000 files `Directories`
  14.7–15.0 ms, `List` 51–55 ms; 20,001 files `Directories` 48–49 ms, `List`
  153–155 ms; **2** and **6** Git processes per op, unchanged because its
  directories are flat.

### Verification

```sh
go test -race -count=1 ./internal/files/
go vet ./internal/files/ && GOOS=windows go vet ./internal/files/
go build ./...
go test -count=1 -run '^$' -bench 'WideDirectories|ProjectListing/(1000|20001)/(Directories|List)$' -benchtime=5x ./internal/files/
```

All pass. Windows was only cross-vetted: candidate paths stay slash-separated and
`./` anchoring is accepted by Git for Windows in principle, but no test ran on
Windows and its reserved names are excluded from the fixture there. Timings are
advisory. No real project, private data, installation, app launch or push.

## PF-15 — Integrated evidence and milestone gate (2026-10-24)

Milestone 1 gate. Revision `87a22e3` on `feature/improve-performance` with only
documentation uncommitted; Apple M5 / 32 GiB, Darwin 27.0.0 arm64, Go 1.26.8,
Git 2.54.0 (Apple Git-157), Node 22.23.2, GOMAXPROCS 10, same host as PF-01…PF-03.
Historical sections above are unchanged; this section only adds new runs.

### Deterministic gates

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | pass |
| `GOOS=windows go vet ./...` (compiles Windows-only sources and tests) | pass |
| `go test -count=1 ./...` (includes in-process RPC/local parity tests in `internal/app`) | pass: 36 packages, 3 without tests |
| `go test -race -count=1` engine, `provider/...`, files, index | pass (10 packages) |
| `npm test` / `npm run check` / `npm run build` | 677/677 pass / 0 errors (existing `ignoreDeprecations` warning) / pass (existing chunk-size advisory) |

Not performed: Windows or Linux test execution (Windows-only tests were compiled,
not run), native GUI QA of any milestone change, cold-disk or peak-RSS profiling,
real session stores or projects, and `scripts/qa-perf-bench.sh` (it reads private
data). Repo-wide lint keeps its existing unrelated findings; no cleanup attempted.

### Before/after on the PF-01…PF-03 workloads

Same commands and flags as the baselines; median [min–max] of three runs (PF-03:
five). Timings are advisory; work counts are deterministic and asserted in tests.

```sh
go test ./internal/index ./internal/provider -run '^$' -bench '^BenchmarkSynthetic' -benchmem -benchtime=200ms -count=3
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscript$' -benchmem -benchtime=1x -count=3 -timeout 30m
go test ./internal/files -run '^$' -bench 'BenchmarkProjectListing|BenchmarkSearch$|BenchmarkWideDirectories' -benchtime=5x -count=5 -benchmem
```

**Scans and catalog (PF-01 → now).**

| Workload | ms/op before | ms/op now | Work before → now | B/op before → now |
|---|---:|---:|---|---:|
| Claude unchanged | 0.269 [0.266–0.273] | **0.177** [0.165–0.203] | decodes 64 → **0** | 188 KB → 150 KB |
| Claude one-source append | 0.373 [0.366–0.374] | **0.251** [0.250–0.257] | decodes 64 → **1** | 1.24 MB → 1.20 MB |
| Claude cold | 3.724 | 3.721 | unchanged (64 opens, 64 encodes) | 67.7 MB → 67.8 MB |
| Codex unchanged | 3.103 | 3.419 [3.233–4.472] | unchanged (192 decodes, 64 encodes) | 1.36 MB |
| Codex append | 3.335 | 3.252 | unchanged | 2.42 MB |
| Codex cold | 6.044 | 6.429 [6.276–7.286] | unchanged | 68.8 MB |
| Catalog 10k open-restore | 68.6 | 67.4 | unchanged | 53.2 MB |

Claude's gain is PF-09. Codex and catalog code are unchanged by M1 (PF-10 is
blocked); their differences are run-to-run noise on this host.

**Transcripts (PF-02 → now).**

| Workload | Loads before → now | Source bytes read | B/op before → now | ms/op before → now |
|---|---|---:|---:|---:|
| Claude 100 MB same-key ×4 | 4 → **1** | 400.6 MB → **100.2 MB** | 2.03 GB → **507 MB** | 1098 → 1178 |
| Codex 100 MB same-key ×4 | 4 → **1** | 400.8 MB → **100.2 MB** (+ resolution 400.8 → 100.2 MB) | 2.44 GB → **609 MB** | 1868 → 1565 |
| Claude/Codex 38 MB same-key ×4 | 4 → **1** | ÷4 | 810/977 MB → **202/244 MB** | 414/689 → 427/643 |
| Independent keys ×4 | 4 → 4 | unchanged | unchanged | unchanged within noise |
| Cold / invalidated / warm page | unchanged | unchanged | unchanged | unchanged within noise |

Coalescing (PF-05) removes the duplicated work and ~¾ of the allocation for
concurrent same-session requests; it does not make a single cold load faster, so
same-key wall time stays roughly one cold load. Codex still reads every byte twice
per cold load (canonical resolution plus transcript); PF-07/PF-08 were meant to
remove that and remain blocked.

**Project listing (PF-03 → now).**

| Workload | ms/op before | ms/op now | Git processes |
|---|---:|---:|---|
| 20,001 files `List` | 152.3 [151.3–153.2] | 151.1 [150.4–151.6] | 6 → 6 |
| 20,001 files `Directories` | 47.7 | 47.0 | 2 → 2 |
| 1,000 files `List` | 45.3 [43.9–55.2] | 54.3 [47.7–60.1] | 6 → 6 |
| Wide tree, 1,000 directories × 1 child | 6190 [6154–6679] (per-chunk oracle) | **53.2** [52.5–53.7] | check-ignore 1010 → **2** |

The PF-03 fixture keeps its directories flat, so it cannot show PF-13/PF-14; the
wide-tree benchmark does (per-chunk oracle = pre-PF-13 structure). The 1,000-file
`List` spread overlaps the baseline; its median moved with Git subprocess noise,
not code (component counts and allocations are identical). Reopening Go to File
now shows the retained listing immediately (PF-11) and keeps it correct after app
mutations (PF-12); that is request-count evidence, not measured GUI latency.

### Milestone 1 outcome

**12/15 complete** (PF-01…PF-06, PF-09, PF-11…PF-15). Not complete, not counted:

- **PF-07 partial/blocked.** Full-byte source proof is correct but slower than the
  plain load on large byte-heavy roots; production declines it (PF-07C).
- **PF-08 blocked** on PF-07: engine/FTS still call ordinary `Load`/`Blob`.
- **PF-10 blocked** on PF-07: unchanged Codex scans still do 192 checkpoint decodes
  and 64 encodes per 64 sources.

### Remaining bottlenecks

1. **Full-parse transcripts.** Every cold or invalidated load re-reads and
   re-parses the whole file: ~1.0 s (Claude) / ~1.6 s (Codex) at 100 MB, 0.5–0.6 GB
   allocated, and any append invalidates the entire entry.
2. **Codex double read.** A cold Codex load scans all records to resolve the
   canonical source before reading the transcript.
3. **Cold start and catalog.** Cold scans are unchanged; restoring a 10k catalog
   costs ~67 ms and 53 MB of allocation; Codex unchanged scans still canonicalize
   checkpoints.
4. **Listings.** At 20k files, `List` is still ~150 ms split across `ls-files`,
   status and directory discovery, recomputed on every refresh; no shared index.
5. **Memory.** The transcript cache holds four whole transcripts (~400 MB at
   100 MB each) with no byte budget.

### Recommendations for Milestone 2

Start Milestone 2 (incremental transcripts and bounded memory) as planned: resumable
Claude/Codex parser state from committed byte offsets, append-only reuse validated by
source identity, earliest-changed-index deltas, and a byte-budgeted transcript cache.
Address the Codex double read inside that design rather than reviving the PF-07
full-byte proof. Write its design and ≤4 h task breakdown in `docs/` before coding,
as for Milestone 1. Native GUI QA of the M1 changes (Go to File reopen and mutation
freshness, concurrent transcript opens) remains unperformed and should be done
when an app run is requested.


## ST-01 — Full engine startup baseline (2026-10-24)

Historical ST-01 gate below is preserved; the subsequent
[ST-04 measurements/decision](#st-04--startup-scaling-and-runner-publication-2026-10-24)
resolve the measurement blocker with a bounded runner-publication fix, not watcher
deferral or a diagnosis of the reported delay.

**Revision/environment:** `0d734ad` plus uncommitted ST-01 observation/tests only,
branch `feature/improve-performance`; Darwin arm64, Apple M5, Go 1.26.8,
Node 22.23.2, npm 10.9.8. No performance intervention. This section is new
synthetic evidence; it neither replaces M1 history nor diagnoses the reported
roughly 10-second launch delay. No private provider stores or caches were used.

### Scope and opt-in observation

`engine.WithStartupObservation(ctx, engine.NewStartupObservation())` attaches an
in-memory, concurrency-safe collector to **one startup and its query contexts**.
Use the same context/collector for Start and the measured ListGroups, AgentCounts
and ListSessions calls; there is no automatic transport propagation. Samples
retain only fixed phase names, success and monotonic start/end durations since
collector creation. No callbacks, default logging, file paths, errors or payloads.
Collector construction is immediately before Start; fixture creation, engine
construction, seeding and Close are excluded. DB writer open includes schema /
integrity / recovery work; it is not a separate measurement of quick_check alone.
`state_restore` covers NewRefresher (including persisted state reads); its success
means construction returned, not that every provider state was valid. Bootstrap
success similarly means the existing call returned nil, not a new discovery
readiness contract. Runner reports may contain isolated provider errors; this
metric does not change error semantics. Reader catalog/state restore is deliberately
one `snapshot_catalog_state_restore` span around atomic LoadSnapshot. No snapshot
split, mutex relocation, watcher scheduling or notification changes were made.
`cache_role_lock` records lock selection, including unavailable-cache failure.

Frontend opt-in: assign `new StartupObservation()` from
`frontend/src/lib/startupObservation.ts` to an AppState's `startupObservation`
before `init()`, then inspect `snapshot()` in memory. Default is null, with no
console output or persisted setting. First accepted groups/counts and sessions
responses are marked after stale-request guards. `usable_data` requires an
accepted nonempty session list after groups/counts; a successful empty reply is
not usable rows or discovery completion. It precedes optional transcript
selection. Host reset detaches the collector; use a fresh collector for another
hydration. Browser timings use only their own performance.now origin; never
subtract them from backend timings. Tests exercise the real compiled store with
generated API replies. **No frontend wall-time distribution, Wails/RPC transport,
native window creation, paint, or GUI responsiveness was measured.**

### Generated workload and method

Nine sequential trials per mode (45 measured full Engine.Start lifecycles), plus
one unmeasured full startup to seed warm cache. Real providers use explicit
throwaway Claude/Codex/OpenCode/cache roots. Claude fixture: **256 JSONL files /
sessions, 16 user records each, 4,096 records, 1,017,920 file bytes** in this run
(temp-root length can change bytes on reproduction). Codex/OpenCode roots are
explicitly nonexistent; their empty discovery is included. No real cache, native
app, tool installation, OS-cache flush, or QA shell benchmark is involved.

Cold writer uses a unique empty application cache per trial; source files and OS
filesystem caches are already warm from fixture construction/seeding. Warm writer
reuses seeded DB/state. Warm reader holds the real writer lock while the engine
opens read-only; there is **no concurrently active writer process**. Disabled
turns SQLite off. Fallback gives the cache root a generated regular file so lock
preparation fails and Start takes its existing in-memory fallback (not a corrupt
DB recovery benchmark). Queries run immediately after synchronous Start returns,
groups → counts → sessions, not via UI parallel groups/counts. They can return
empty before bootstrap. The harness polls the observation snapshot at 1 ms only
to await completion; recorded bootstrap end is collected inside bootstrap, not
the polling wake-up. Final catalog is checked at 256 sessions in every trial.

### Distributions (milliseconds)

Each cell is **min / median / p95 / max**, n=9; nearest-rank p95 is the maximum
for this small sample. Endpoints share the per-trial collector origin, durations
measure only the named span. Independent column percentiles must not be
subtracted. These are advisory, not CI timing thresholds.

| Full startup mode | Start duration | Bootstrap completion endpoint | First groups endpoint | First counts endpoint | First sessions endpoint |
|---|---|---|---|---|---|
| Cold writer | 5.177 / 5.267 / 5.951 / 5.951 | 42.774 / 44.072 / 69.233 / 69.233 | 5.179 / 5.269 / 5.954 / 5.954 | 5.179 / 5.270 / 5.954 / 5.954 | 5.180 / 5.271 / 5.954 / 5.954 |
| Warm writer | 6.555 / 7.330 / 7.837 / 7.837 | 9.038 / 10.536 / 10.948 / 10.948 | 7.150 / 7.832 / 8.290 / 8.290 | 7.297 / 8.255 / 8.426 / 8.426 | 7.552 / 8.520 / 8.805 / 8.805 |
| Warm reader | 5.935 / 6.540 / 7.008 / 7.008 | 7.393 / 8.309 / 9.387 / 9.387 | 6.515 / 6.951 / 7.442 / 7.442 | 6.931 / 7.331 / 8.227 / 8.227 | 7.092 / 7.578 / 8.979 / 8.979 |
| Disabled | 0.002 / 0.002 / 0.015 / 0.015 | 13.519 / 14.910 / 15.656 / 15.656 | 0.002 / 0.003 / 0.022 / 0.022 | 0.002 / 0.003 / 0.022 / 0.022 | 0.003 / 0.003 / 0.023 / 0.023 |
| Fallback | 0.005 / 0.007 / 0.010 / 0.010 | 13.634 / 14.759 / 18.057 / 18.057 | 0.006 / 0.008 / 0.015 / 0.015 | 0.006 / 0.009 / 0.015 / 0.015 | 0.006 / 0.009 / 0.015 / 0.015 |

| Phase duration (unless endpoint specified) | Cold writer | Warm writer | Warm reader |
|---|---|---|---|
| DB open, writer including integrity | 1.569 / 1.598 / 2.024 / 2.024 | 0.947 / 1.062 / 1.184 / 1.184 | N/A |
| DB open, read-only | N/A | N/A | 0.343 / 0.432 / 0.556 / 0.556 |
| Catalog restore | 0.011 / 0.013 / 0.016 / 0.016 | 0.852 / 1.027 / 1.198 / 1.198 | Atomic span below |
| Catalog+state snapshot | N/A | N/A | 1.860 / 2.164 / 2.399 / 2.399 |
| Catalog publication endpoint (includes event enqueue) | 1.693 / 1.722 / 2.153 / 2.153 | 1.891 / 2.098 / 2.373 / 2.373 | 2.344 / 2.554 / 2.972 / 2.972 |
| State restoration / refresher construction | 0.017 / 0.019 / 0.021 / 0.021 | 0.883 / 0.946 / 1.196 / 1.196 | Included in atomic snapshot |
| Watcher setup | 3.455 / 3.523 / 3.777 / 3.777 | 3.716 / 4.063 / 4.739 / 4.739 | 3.382 / 4.002 / 4.115 / 4.115 |

Work totals across all nine trials per mode (logical ScanWork, not physical I/O):

| Mode | Source opens | Records | Record bytes | Checkpoint decodes | Checkpoint encodes |
|---|---:|---:|---:|---:|---:|
| Cold writer | 2,304 | 36,864 | 9,124,416 | 0 | 2,304 |
| Warm writer | 0 | 0 | 0 | 2,304 | 0 |
| Warm reader | 0 | 0 | 0 | 2,304 | 0 |
| Disabled | 2,304 | 36,864 | 9,124,416 | 0 | 2,304 |
| Fallback | 2,304 | 36,864 | 9,124,416 | 0 | 2,304 |

Record bytes exclude JSONL newline bytes. Accounting is attached to scanner
contexts, not FTS loads, directory discovery, OS reads or all background work.
Indexer and cleanup are real full-startup work and may contend during trials,
but their transcript I/O and completion are not measured here. Close waits for
owned cache work outside the timing interval. Disabled/fallback existing runner
lifecycle is unchanged; trials wait for scan completion before Close.

### Ordering, decision and limitations

Deterministic channel-barrier tests hold the provider's WatchPaths during
synchronous watcher installation, separately for warm writer and warm reader.
They prove cached catalog publication and successful sessions/groups/counts reads
while Start still holds engine mu, **before watcher setup or bootstrap can finish**.
Query methods still bypass engine mu. Observation tests check concurrent first
success de-duplication, detached snapshots and opt-out. Repeated generated trials
check publication → watcher setup → bootstrap start and final catalog parity.
No elapsed-time assertion enforces that ordering. Existing commit → catalog apply
→ event, exclusion, reader promotion and atomic LoadSnapshot logic is untouched.

**ST-04 decision: measurement blocker; leave pending, do not optimize yet.**
Watcher setup is the largest synchronous span on these tiny warm fixtures
(~4 ms of ~7 ms), but cached data is already published (~2 ms) and independently
queryable. This neither explains a 10-second delay nor demonstrates benefit worth
the installation-race/lifecycle change. Cold bootstrap (~44 ms) is intentionally
asynchronous and one-provider dominated; this workload cannot justify per-provider
notification changes. No transport/frontend timing supports removing sequencing,
and integrity/atomic snapshot checks must remain. The bounded next measurement
is a generated directory-fanout sweep (same rows, increasing empty nested watch
directories), then generated multi-provider/held-fast-slow completion cases and
an approved generated-root adapter/frontend hydration trace. Keep fixture setup
outside timing; compare publication, Start, queries and usable-data separately.
If fanout makes watcher setup dominate a meaningful usability delay, scope only
owned/deferred watcher setup with installation reconciliation and deterministic
Close/restart/promotion tests before ST-04 coding. Otherwise continue measuring;
no speculative performance intervention is selected. ST-02/03 may proceed in
sequence on truthful readiness, but ST-04 remains evidence-gated.

This is not a representative 300 MB history, SQLite-provider population, recursive
watcher fanout, multi-process contention, Windows/WSL disk, cold OS cache or native
paint baseline. The preexisting Open → LoadCatalog → Reset test is not used as
full startup evidence. Frontend measurements are deterministic observation/guard
coverage, not native elapsed evidence. M1 12/15 status and PF-07/08/10 blockers
are unchanged.

### Reproduction and checks

From repository root, using installed tools only:

```sh
AS_STARTUP_BASELINE=1 go test ./internal/engine -run '^TestStartup' -count=1 -v
AS_STARTUP_BASELINE=1 go test -race ./internal/engine -run 'TestStartup|TestSharedCache_|Test.*(ClearCache|Forget|Retire)' -count=1
go test -race ./internal/index ./internal/watch
go vet ./internal/engine
(cd frontend && npm test && npm run check && npm run build)
git diff --check
```

Recorded checks: startup repeated baseline PASS; engine startup/shared-cache/
exclusion focused race PASS (`11.224s`, final rerun); index/watch race PASS (`36.847s` /
`3.887s`); engine vet PASS. Frontend tests **680/680 PASS** (`7.832s`, final rerun, including the
stale-reply test); check **0 errors, 1 existing
ignoreDeprecations warning**; production build PASS (existing >500 kB chunk
warning). Startup-specific frontend coverage is 3 tests; the test API fixture
was corrected to use a proxy because the compiled module is cached between tests.
No frontend lint script/configured lint executable is present; TypeScript/Svelte
checks and production build cover affected frontend code. No full-repository Go
tests, native app, remote host, Windows packaging or GUI
paint check was run for ST-01. No tools installed; no commit or push made.

## ST-04 — Startup scaling and runner publication (2026-10-24)

**Revision/environment:** `0c65dd4` plus uncommitted ST-04 changes, branch
`feature/improve-performance`; Darwin arm64, Apple M5, Go 1.26.8. Generated
fixtures, explicit throwaway provider/cache roots, no private stores or caches,
native app, installs or QA shell benchmark. Opt-in ST-01 observation; fixture
creation, cache seeding, engine construction and Close are excluded. Each cell
is **min / median / p95 / max** (ms), n=9, nearest-rank p95 equals max; advisory,
not CI thresholds. This does not diagnose the reported roughly 10-second delay.

### Generated watch-directory fanout (warm writer)

`TestStartupFanout`: the ST-01 Claude fixture (256 sessions, 4,096 records,
~1.01–1.03 MB; bytes vary with temp-path length) plus empty leaf directories under
`<generated-claude>/projects/fanout/bucket-NN/leaf-NNNNN` (100 per bucket). One
unmeasured seed startup, then nine full warm-writer Engine.Start trials per tier,
queries immediately after Start, completion awaited per ST-01.

| Extra leaf / bucket dirs | Start duration | Watcher setup | Publication endpoint | First sessions endpoint | Bootstrap endpoint |
|---|---|---|---|---|---|
| 0 / 0 | 5.834 / 6.115 / 6.693 / 6.693 | 3.257 / 3.349 / 3.742 / 3.742 | 1.657 / 1.768 / 2.023 / 2.023 | 6.840 / 7.077 / 7.798 / 7.798 | 8.268 / 8.642 / 9.227 / 9.227 |
| 1,000 / 10 | 43.638 / 45.189 / 47.902 / 47.902 | 40.947 / 42.276 / 45.226 / 45.226 | 1.721 / 1.800 / 1.997 / 1.997 | 44.712 / 46.006 / 48.710 / 48.710 | 46.046 / 47.609 / 50.459 / 50.459 |
| 10,000 / 100 | 412.766 / 415.195 / 418.404 / 418.404 | 409.581 / 412.209 / 415.538 / 415.538 | 1.862 / 2.023 / 2.277 / 2.277 | 413.473 / 416.575 / 419.747 / 419.747 | 416.159 / 418.445 / 421.560 / 421.560 |

DB open/integrity (~0.8–1.1 ms), catalog restore (~0.85 ms), state restore
(~0.9 ms) and query durations (<1.2 ms) stay flat. Work per tier across nine
trials: 0 source opens, 0 records, 0 record bytes, **2,304 checkpoint decodes**,
0 encodes. Watcher setup scales roughly linearly (~41 µs/directory) and dominates
synchronous `Engine.Start` and the `App.OnStartup` return, which Wails runs on its
own goroutine. Bootstrap starts after watcher installation, so its endpoint shifts
too. Cached publication stays ~2 ms and catalog queries bypass `e.mu`, so a client
calling them concurrently can read rows earlier than the harness's post-Start
query. **Not selected:** owned asynchronous watcher installation would change
watcher lifetime, Close/restart, promotion and install/event reconciliation;
that is a separate bounded follow-up (fanout timing with concurrent client
queries plus held-install race tests), not part of ST-04. No 10 s result at 10k
directories on this machine; Windows/WSL disk and kqueue/inotify limits unmeasured.

### Fast/slow provider publication (disabled and fallback)

`TestStartupProviderPublication`: two generated fake providers (one row each);
the slow one sleeps 300 ms in Scan to model an unrelated slow provider. Nine
trials per mode, 18 scans/mode, 2 final rows/trial, logical source ScanWork 0
(fakes). `first_nonempty_query_end` polls ListSessions at 1 ms;
`first_catalog_event_end` is when the real `catalog:changed` emit fires.
Paired final reruns, collector created immediately before Start:

| Mode | Metric | Before | After |
|---|---|---|---|
| Disabled | First catalog event | 400.806 / 401.241 / 401.694 / 401.694 | 100.147 / 100.162 / 100.578 / 100.578 |
| Disabled | First nonempty query | 1.070 / 1.178 / 1.196 / 1.196 | 1.132 / 1.175 / 1.414 / 1.414 |
| Disabled | Bootstrap endpoint | 300.182 / 300.199 / 300.586 / 300.586 | 300.153 / 300.205 / 300.471 / 300.471 |
| Fallback | First catalog event | 400.318 / 401.255 / 401.435 / 401.435 | 100.169 / 100.192 / 100.362 / 100.362 |
| Fallback | First nonempty query | 1.187 / 1.223 / 1.369 / 1.369 | 1.183 / 1.203 / 1.404 / 1.404 |
| Fallback | Bootstrap endpoint | 300.197 / 300.250 / 300.403 / 300.403 | 300.192 / 300.219 / 300.410 / 300.410 |

Pre-implementation first run (recorded in the brief before coding): event
402.322 / 402.341 ms, query 1.311 / 1.353 ms. Before the fix, the runner applied
the fast provider's rows at ~1 ms but the frontend had no catalog event until the
slowest scan finished plus the 100 ms bus debounce. After it, the event follows the
fast application plus the preserved debounce; queryability and total discovery
time are unchanged. Event-driven clients, such as the ST-03 reload queue, see fast
rows ~300 ms earlier in this artificial case. Coupling scales with the slowest
provider; real provider scan times were not measured here.

### Selected intervention and evidence

`scan.Runner.RunWithOnApplied` invokes an optional callback serially on the
collecting goroutine after each successful catalog application and report update,
including empty/removal results and successful scans with live errors; never for
detection/scan failures, absent or duplicate providers or cancellation. `Run` is
the nil-callback form. Disabled/fallback `startRunnerDiscovery` uses it to call
`NotifyFullRefresh` on its lifetime's captured bus; final apply, transcript clear,
full refresh and terminal discovery status are unchanged, so discovery remains
reconciling until aggregation. Cache-backed paths, integrity, atomic snapshots,
watchers, promotion and ClearCache/Forget/Retire exclusion are untouched.

Deterministic tests (channels; timeouts only detect deadlock):
`TestRunnerOnAppliedOutcomes` (nine outcomes, callback after catalog
application, final state, one scan), `TestRunnerOnAppliedDoesNotWaitForSlowProvider`
(held slow scan: fast callback/rows first; release → final parity; cancel → no
slow callback, cancellation error) and `TestStartupRunnerPublishesFastProvider`
(disabled/fallback × normal/Close-restart: real bus event and one fast row while
held and reconciling; settled terminal; two final rows; Close joins cancelled
held scan; restarted lifetime emits on its new bus; one scan/provider/lifetime).

### Reproduction and checks

```sh
AS_STARTUP_BASELINE=1 go test ./internal/engine -run '^TestStartup(Fanout|ProviderPublication)$' -count=1 -v
go test -race ./internal/scan ./internal/engine -run 'Test(Runner|Startup|Discovery|SharedCache_|.*(ClearCache|Forget|Retire))' -count=1
AS_STARTUP_BASELINE=1 go test -race ./internal/engine -run '^TestStartup' -count=1
go test -race ./internal/index ./internal/watch -count=1
go test ./internal/scan ./internal/engine -count=1
go vet ./internal/scan ./internal/engine
git diff --check
```

Recorded: final focused scan/engine race PASS (2.186 s / 4.101 s); final non-race
fanout/publication sweep PASS (11.707 s); app regression PASS (1.848 s);
opt-in startup race
including both sweeps PASS (33.251 s); index/watch race PASS (41.753 s / 3.893 s);
full scan/engine packages PASS (1.060 s / 16.117 s); vet and diff check PASS.
No frontend change, so no frontend checks; no full-repository Go suite, native app,
remote host, Windows build or GUI paint check. No tools installed; no commit/push.

## ST-05 — Integrated startup gate (2026-10-24)

**Revision/environment:** `7b401fa` plus uncommitted ST-05 test-only changes,
branch `feature/improve-performance`; Darwin arm64, Apple M5, Go 1.26.8,
Node 22.23.2, npm 10.9.8. Same generated fixtures, explicit throwaway
provider/cache roots, opt-in observation and exclusions as
[ST-01](#st-01--full-engine-startup-baseline-2026-10-24) and
[ST-04](#st-04--startup-scaling-and-runner-publication-2026-10-24). No private
stores, caches or logs, no `scripts/qa-perf-bench.sh`, native app, installs,
Windows packaging, commit or push. Historical Codex/M1 benchmarks were not rerun.
Each cell is **min / median / p95 / max** (ms), n=9, nearest-rank p95 = max;
advisory, not CI thresholds. This does **not** diagnose the reported roughly
10-second delay.

The recorded run below is the final `AS_STARTUP_BASELINE=1 go test
./internal/engine -run '^TestStartup' -count=1 -v`, run alone (13.698 s). An
earlier rerun overlapped the race and frontend suites and was discarded for
comparison: same orderings and work counts, but wider tails (for example 10k
fanout Start median 588 ms, max 1,166 ms), showing that contention dominates
these small absolute numbers.

### Full startup (ST-01 workload repeated)

Fixture: 256 Claude JSONL sessions, 4,096 records, 1,017,920 file bytes;
Codex/OpenCode roots nonexistent; final catalog 256 sessions in every trial.

| Full startup mode | Start duration | Bootstrap completion endpoint | First groups endpoint | First counts endpoint | First sessions endpoint |
|---|---|---|---|---|---|
| Cold writer | 5.316 / 5.543 / 5.791 / 5.791 | 41.067 / 42.530 / 43.544 / 43.544 | 5.319 / 5.546 / 5.794 / 5.794 | 5.319 / 5.547 / 5.795 / 5.795 | 5.319 / 5.548 / 5.796 / 5.796 |
| Warm writer | 6.292 / 6.774 / 6.979 / 6.979 | 8.929 / 9.286 / 9.790 / 9.790 | 6.826 / 7.172 / 7.371 / 7.371 | 6.970 / 7.499 / 7.614 / 7.614 | 7.114 / 7.643 / 8.004 / 8.004 |
| Warm reader | 5.921 / 6.071 / 6.601 / 6.601 | 7.378 / 7.580 / 8.062 / 8.062 | 6.385 / 6.639 / 7.073 / 7.073 | 6.791 / 7.033 / 7.608 / 7.608 | 6.974 / 7.210 / 7.741 / 7.741 |
| Disabled | 0.002 / 0.002 / 0.002 / 0.002 | 12.627 / 14.168 / 15.049 / 15.049 | 0.003 / 0.003 / 0.005 / 0.005 | 0.003 / 0.003 / 0.005 / 0.005 | 0.003 / 0.004 / 0.005 / 0.005 |
| Fallback | 0.005 / 0.007 / 0.013 / 0.013 | 13.924 / 14.838 / 15.449 / 15.449 | 0.006 / 0.008 / 0.014 / 0.014 | 0.006 / 0.009 / 0.015 / 0.015 | 0.006 / 0.009 / 0.015 / 0.015 |

| Phase duration (unless endpoint specified) | Cold writer | Warm writer | Warm reader |
|---|---|---|---|
| DB open, writer including integrity | 1.547 / 1.622 / 1.759 / 1.759 | 0.755 / 0.839 / 0.909 / 0.909 | N/A |
| DB open, read-only | N/A | N/A | 0.271 / 0.319 / 0.375 / 0.375 |
| Catalog restore | 0.011 / 0.013 / 0.017 / 0.017 | 0.892 / 1.158 / 1.233 / 1.233 | Atomic span below |
| Catalog+state snapshot | N/A | N/A | 2.018 / 2.128 / 2.562 / 2.562 |
| Catalog publication endpoint | 1.661 / 1.745 / 1.909 / 1.909 | 1.695 / 2.037 / 2.155 / 2.155 | 2.333 / 2.465 / 2.928 / 2.928 |
| State restoration / refresher construction | 0.018 / 0.019 / 0.022 / 0.022 | 0.887 / 1.013 / 1.254 / 1.254 | Included in atomic snapshot |
| Watcher setup | 3.626 / 3.732 / 3.909 / 3.909 | 3.545 / 3.675 / 3.970 / 3.970 | 3.476 / 3.572 / 3.690 / 3.690 |

Work totals per mode across nine trials are identical to ST-01: cold writer,
disabled and fallback **2,304 opens / 36,864 records / 9,124,416 record bytes /
0 decodes / 2,304 encodes**; warm writer and reader **0 / 0 / 0 / 2,304 / 0**.

### Before/after medians (ms)

| Measurement | Before | After (ST-05) | Interpretation |
|---|---|---|---|
| Cold writer Start / bootstrap endpoint | 5.267 / 44.072 (ST-01) | 5.543 / 42.530 | Unchanged within noise; no intervention on this path |
| Warm writer Start / publication endpoint / first sessions | 7.330 / 2.098 / 8.520 (ST-01) | 6.774 / 2.037 / 7.643 | Unchanged within noise; cached rows were already published at ~2 ms |
| Warm reader Start / publication endpoint / first sessions | 6.540 / 2.554 / 7.578 (ST-01) | 6.071 / 2.465 / 7.210 | Unchanged within noise |
| Disabled / fallback bootstrap endpoint | 14.910 / 14.759 (ST-01) | 14.168 / 14.838 | Unchanged; single real provider, no slow peer |
| Fanout Start, 0 / 1k / 10k extra directories | 6.115 / 45.189 / 415.195 (ST-04) | 6.461 / 46.471 / 427.056 | **Not improved**: watcher setup still linear |
| Fanout watcher setup, 0 / 1k / 10k | 3.349 / 42.276 / 412.209 (ST-04) | 3.526 / 43.598 / 423.655 | Not improved; publication still 1.9 / 1.9 / 2.1 ms |
| Disabled / fallback first catalog event, 300 ms slow fake peer | 401.241 / 401.255 (ST-04 before) | 100.241 / 100.290 | **Improved** by ST-04; remaining ~100 ms is the preserved debounce |
| Disabled / fallback first nonempty query | 1.178 / 1.223 (ST-04 before) | 1.288 / 1.326 | Unchanged: rows were already queryable |
| Disabled / fallback bootstrap endpoint, slow peer | 300.199 / 300.250 (ST-04 before) | 300.285 / 300.314 | Unchanged: discovery completion still waits for the slow provider |

Fanout tiers (warm writer, 256 sessions; 0 / 1,000 / 10,000 extra leaf dirs in
0 / 10 / 100 buckets) after ST-05, full distributions:

| Extra leaf / bucket dirs | Start duration | Watcher setup | Publication endpoint | First sessions endpoint | Bootstrap endpoint |
|---|---|---|---|---|---|
| 0 / 0 | 6.242 / 6.461 / 7.066 / 7.066 | 3.386 / 3.526 / 4.138 / 4.138 | 1.809 / 1.905 / 2.144 / 2.144 | 7.200 / 7.409 / 8.071 / 8.071 | 8.748 / 9.047 / 9.526 / 9.526 |
| 1,000 / 10 | 45.437 / 46.471 / 51.489 / 51.489 | 42.618 / 43.598 / 48.845 / 48.845 | 1.752 / 1.858 / 2.202 / 2.202 | 46.452 / 47.413 / 52.461 / 52.461 | 48.194 / 48.953 / 53.910 / 53.910 |
| 10,000 / 100 | 291.737 / 427.056 / 435.727 / 435.727 | 288.403 / 423.655 / 432.098 / 432.098 | 1.992 / 2.118 / 2.504 / 2.504 | 293.184 / 427.740 / 436.408 / 436.408 | 295.432 / 429.852 / 438.640 / 438.640 |

Each fanout tier: 0 opens / 0 records / 0 record bytes / **2,304 checkpoint
decodes** / 0 encodes. Provider publication (two fakes, one held 300 ms in
Scan): 18 scans/mode, 2 final rows/trial, logical ScanWork 0; disabled event
100.231 / 100.241 / 100.250 / 100.250, query 1.214 / 1.288 / 1.322 / 1.322,
bootstrap 300.257 / 300.285 / 300.307 / 300.307; fallback event
100.181 / 100.290 / 100.327 / 100.327, query 1.289 / 1.326 / 1.334 / 1.334,
bootstrap 300.206 / 300.314 / 300.350 / 300.350.

**What improved:** truthful discovery status (ST-02/03) and, in disabled and
fallback cache modes only, a catalog event after each successful provider
application instead of after the slowest provider (ST-04). **What did not:**
synchronous `Engine.Start`, watcher setup, backend queryability and discovery
completion. Warm writer/reader notification was already per provider through the
refresher and is not claimed as improved. Timings are backend endpoints within
one process; they are not frontend applied state or native paint.

### Integrated acceptance coverage

Deterministic tests use channel barriers; timeouts only detect deadlock.

| Acceptance item | Test evidence |
|---|---|
| Warm cached rows queryable while watcher setup is held; final parity | `TestStartupPublicationQueryableDuringWatcherSetup` (writer and reader): `reconciling`/usable status without waiting on `e.mu`; groups/counts/sessions answer while WatchPaths is held; **ST-05 extension**: release → `settled` terminal, provider-backed final catalog (2 rows, count 2) supersedes the 1-row cached hint |
| Successful empty, zero detected/configured, isolated and total failure, fallback | `TestDiscoveryTerminalOutcomes` (24 cases: disabled/writer/reader/fallback × six outcomes) |
| Cancellation, Close and restart | `TestDiscoveryCancelledAttemptPublishesTerminalFailure`, `TestDiscoveryCloseCancelsOwnedBootstrapAndRestarts` (disabled/writer), `TestStartupRunnerPublishesFastProvider` cancel cases, `TestDiscoveryLifetimeAndAttempts` |
| Reader promotion supersedes the reader attempt | `TestDiscoveryReaderPromotionSupersedesReaderAttempt` |
| Fast provider event before slow release; final parity | `TestRunnerOnAppliedOutcomes`, `TestRunnerOnAppliedDoesNotWaitForSlowProvider`, `TestStartupRunnerPublishesFastProvider` |
| Local vs in-process RPC snapshot/event parity; old-host fallback | `TestDiscoveryLocalRPCParity` (two lifetimes), `TestDiscoveryOldHostFallback` (no capability, `-32601`, `unsupported`, browsing still works) |
| App routes, unavailable reply and host/generation event isolation | `TestDiscoveryReplyRoutes`, `TestDiscoveryEventHostIsolation` |
| Frontend snapshot/event ordering, missed events, host switch/reconnect, unavailable, empty/outcome/filter, terminal refresh, selection | `frontend/test/discovery.test.mjs` (14 tests), `discovery-api.test.mjs` |

The review found one concrete gap: the held-watcher test proved cached
queryability but not terminal status and final-catalog parity after the held
phase. It was extended rather than duplicated. No discovery correctness bug was
found, so no production code changed. GroupTree with **unavailable** status
(old host or backend without `DiscoveryBackend`) intentionally keeps its legacy
"No Agents Detected" empty text and SessionList keeps "No matching sessions";
this compatibility is tested but is not detection-backed, and its native
rendering is unverified.

### Remaining bottlenecks and limits

- The user's reported roughly 10-second delay remains **undiagnosed**; no
  generated workload here reached it (10k directories ≈ 0.43 s on this machine).
- Watcher fanout still scales linearly (~42 µs/directory) inside synchronous
  `Engine.Start`/`App.OnStartup`. Owned asynchronous watcher installation remains
  a separate follow-up needing installation/event reconciliation and
  Close/restart/promotion tests.
- The early-event fix applies to disabled/fallback runner discovery only; events
  remain coalesced by the 100 ms bus window, and discovery completion still waits
  for the slowest provider. Real provider scan times were not measured.
- Status events are emitted synchronously during Start; manual Scan and watcher
  refreshes do not update discovery status; automatic reconnect keeps the
  connection generation (the frontend resets on phase change).
- Not executed: native GUI window/paint, Wails transport timing, frontend
  usable-data wall time, real SSH/remote hosts, Windows/WSL disks, kqueue/inotify
  limits, multi-process writer contention, cold OS caches, a representative
  300 MB history, and Windows packaging.
- M1 remains 12/15 with PF-07 partial/blocked and PF-08/PF-10 blocked; Milestone 2
  is not started.

### Reproduction and checks

```sh
AS_STARTUP_BASELINE=1 go test ./internal/engine -run '^TestStartup' -count=1 -v
go test ./... -count=1
go vet ./... && go build ./... && GOOS=windows go vet ./...
go test -race -count=1 ./internal/engine ./internal/scan ./internal/index ./internal/watch ./internal/app ./internal/rpc
AS_STARTUP_BASELINE=1 go test -race ./internal/engine -run '^TestStartup' -count=1
go test -race -count=3 ./internal/engine ./internal/rpc -run 'TestStartupPublicationQueryableDuringWatcherSetup|TestDiscovery|TestStartupRunnerPublishesFastProvider'
go tool golangci-lint run --new-from-rev=0d734ad --max-issues-per-linter=0 --max-same-issues=0
(cd frontend && npm test && npm run check && npm run build)
git diff --check
```

Recorded on the final tree: `go test ./...` **PASS** (36 packages ok, 3 without
tests; engine 16.635 s); `go vet ./...`, `go build ./...`, `GOOS=windows go vet
./...` **PASS** (exit 0). Race: engine 171.826 s, scan 2.146 s, index 37.046 s,
watch 3.913 s, app 2.960 s, rpc 9.153 s, all **PASS**; opt-in startup race
**PASS** (28.510 s); focused barrier tests `-count=3` **PASS** (engine 4.540 s,
rpc 1.291 s). Changed-code lint (`golangci-lint` via `go tool`, repo
`.golangci.yml`, since startup-package base `0d734ad`) initially reported
5 errcheck and 3 gofumpt issues, all in ST-01…ST-04 startup/discovery test
files; fixed (deferred `Close`/`Unlock` errors discarded explicitly, `0o` file
modes, blank lines), now **0 issues**. The whole repository still has preexisting
lint findings outside this package; they were not changed. Frontend (no frontend
changes in ST-05): `npm test` **697/697 PASS** (0 failed/skipped/cancelled,
9.99 s), `npm run check` **0 errors**, 508 files, 1 existing `ignoreDeprecations`
warning; `npm run build` **PASS** (1.42 s, existing >500 kB chunk warning).
No frontend lint script is configured. `git diff --check` **PASS**.


## ST-06A — Large-index startup baseline (2026-10-24)

Base revision `f568075`, Go 1.26.8 darwin/arm64, generated fixtures only.
`AS_STARTUP_BASELINE=1 go test ./internal/index -run '^TestStartupLargeIndexOpen$' -v -count=1 -timeout 10m`
produced a **300.7 MiB** checkpointed database: 170 sessions, one with 3,000
messages, others with 200–599, deterministic pseudo-random text from 30,000 words.
Metadata uses CommitScan; messages use the production replaceFTSDocs path including
FTS triggers/revision checks. Generation (48.522 s) excluded from timings. Clean
close removed WAL; this is NOT a WAL-recovery or cold filesystem-cache benchmark.
Five opens use a warm OS page cache, fresh SQL handles; timings are advisory.

| Phase | Median | Max |
|---|---:|---:|
| index.Open | 5.358406 s | 5.928286 s |
| Ping (separate handle) | 445 µs | 487 µs |
| quick_check(1) | 5.327165 s | 5.368377 s |
| runMigrations including its own check | 5.297570 s | 5.530944 s |
| LoadCatalog | 778 µs | 1.004 ms |

Measurements of substeps are independent, not additive or subtracted. An earlier
266.3 MiB fixture measured Open 4.680 s / check 4.663 s median. The generator was
corrected to target checkpointed database size rather than double-counting DB+WAL.
This reproduces a large pre-publication cost, not the user's exact >10 s delay or
native GUI paint. Private indexes/logs were never opened. Local degraded discovery
is a separate unresolved issue; provider failure details were not inspected.

Selected ST-06B intervention: read and publish current-schema cached metadata under
the writer-role lock before mandatory full integrity verification. Keep original
synchronous engine startup serialization, reader snapshots, promotion, and recovery.
Wails 2.14.0 already runs OnStartup on a goroutine (darwin frontend.go:248–253);
a desktop lifecycle rewrite is not required. Tests: `go test ./internal/index`,
`go vet ./internal/index`, changed-code golangci-lint and `git diff --check` pass.
The opt-in large test passed (129.742 s); ordinary test execution skips generation.


## ST-06B — Early writer catalog hints (2026-10-24)

Based on `99f9b6f`, the initial writer now opens the current-schema cache read-only,
loads metadata atomically, closes that handle, and publishes provisional rows before
mandatory full writer verification. Discovery remains incomplete. Existing writer
publication replaces hints, including empty after recovery. Read-only failure falls
through unchanged; writer failure clears hints before fallback. Promotion and real
reader snapshot/state ordering are unchanged; Start/Close serialization is retained.

Repeat ST-06A command/fixture (300.7 MiB, 170 sessions, five runs, warm OS page cache,
no WAL at reopen; 57.533 s generation excluded):

| Phase | Median | Max |
|---|---:|---:|
| Early read-only open + catalog + close | 1.336 ms | 1.709 ms |
| Mandatory verified index.Open | 5.583755 s | 5.970426 s |
| quick_check(1), separately measured | 5.783932 s | 6.020599 s |
| LoadCatalog after verification | 738 µs | 956 µs |

The early read timing is not full Engine.Start, Wails transport, or native paint.
Channel-held verified-open tests prove actual engine groups/counts/sessions and a
catalog event are available before verification can complete. Cases cover verified
rows, empty rebuild, a real CHECK-constraint integrity failure recovered by Open,
malformed metadata, missing cache, writer-open failure and canceled verification.
A held watcher proves authoritative replacement before bootstrap. Existing reader
promotion and Close/restart tests pass. Search/index progress can still wait behind
engine startup; this fix makes the list queryable, not all actions immediately ready.

Checks: full Go tests, vet, build and Windows cross-vet passed. Race tests engine /
index / app / rpc passed (184.395 / 46.041 / 3.481 / 11.694 s); later added real
integrity-recovery case passed in focused barriers under race `-count=3` (4.076 s).
Changed-code lint: 0 issues; diff check passed. No frontend changes; frontend tests
were not rerun. Native GUI, real remote/SSH/WSL and Windows execution were not run.
No private logs/index content accessed, packaging, app replacement, installs or push.
The exact user-device >10 s and degraded provider outcome remain unverified.


## ST-07 — Remote early catalog handshake (2026-10-24)

Based on `b41456d`, Go 1.26.8 darwin/arm64. Production CLI now prepares the
backend and diagnostics identity/capabilities before launching a Serve-owned startup
task. Initialize returns when the new discovery lifetime publishes CatalogUsable,
or when Start finishes. Discovery completion still means reconciliation, not socket
readiness. Existing synchronous starters and wire protocol version 1 are unchanged.

Catalog/status/ping calls can run early. All other RPC methods wait on a cancelable
startup gate before entering engine code. Serve cancels and joins startup before
expiring diagnostics; CLI closes the engine last. This preserves local synchronous
Start, integrity/recovery and avoids a late Start reviving a closed engine.

### Generated evidence and limits

`AS_STARTUP_BASELINE=1 go test ./internal/rpc -run '^TestRemoteStartupMeasurements$' -v -count=1 -timeout 60s`
uses nine trials per mode, in-process pipes and a fake discovery backend. It publishes
readiness then performs **150 ms of artificial startup work**. Fixture/setup excluded.
This is a scheduling/handshake measurement, NOT a large-database or SSH benchmark.

| Initialize → first successful catalog response | Median | Max |
|---|---:|---:|
| Synchronous starter baseline | 151.484 ms | 153.172 ms |
| Prepared early-catalog startup | 467 µs | 547 µs |

The measured fake response is empty: this does not assert usable nonempty data.
Separately, the real-engine generated-cache integration holds watcher registration
under engine.mu and reads a nonempty cached list, groups/counts and discovery over
RPC before releasing startup. It also verifies remote diagnostics identity/capability
survives pre-start preparation. ST-06 engine tests hold writer integrity opening and
verify early cached hints (including malformed/corrupt recovery cases); ST-07 observes
the same CatalogUsable state. No full 300 MiB RPC/SSH end-to-end measurement was run.

Observers are opt-in/context-local, contain phase names/timings/success only, and have
no default logging or wire payload. Client probe/bundle selection/deploy/preface/
initialize, server preparation/identity/catalog-ready/full Start and first successful
catalog responses are independently observable. Empty success is not a usable-row
metric. No cross-process clock subtraction. Fake SSH tests verify handshake phases;
real authentication/network/deployment costs remain unknown.

### Verification and retesting

Full `go test ./...`, `go vet ./...`, `go build ./...`, `GOOS=windows go vet ./...`
PASS. RPC/remote race suites PASS (9.329 / 20.082 s), CLI/app earlier race suites PASS
(2.121 / 2.891 s). Final focused engine/RPC/app/CLI readiness, discovery, connection and
serve tests `-race -count=3` PASS (4.417 / 4.684 / 2.569 / 3.174 s). Changed-code lint
0 issues; diff check PASS. Tests cover early data, stale lifetimes, no-watcher
fallback, failed startup, shutdown/cancellation, late failure disconnect, gated RPC
cancellation, duplicate initialize, env roots and retained backend optional interfaces.
Frontend unchanged; frontend tests/check/build not rerun. Real SSH/WSL, native GUI,
Windows execution and remote deployment unperformed. No private data or cache read.

To retest, rebuild **both desktop and bundled remote servers** through the existing
packaging target (`make package-macos` for macOS), not just the desktop executable.
Use only inspected/approved build destinations. For isolated bundle verification use
`make remote-servers REMOTE_DIR=<fresh temporary directory>`; no bundle build or app
replacement was performed here. The launcher matches bundle checksums even for the
same app version; without a fresh bundle the old server may still be selected.
SSH setup/probe/deploy, full verification, gated actions and final reconciliation can
still take time. These changes remove catalog visibility's dependency on full remote
startup; they do not establish a measured user-host or native responsiveness result.

## ST-08 — Full-text index bloat and the startup check (2026-10-25)

Based on `ee3ef76`. A user cache showed `Updating...` for about 20 s on every launch.
With the user's approval a temporary copy of it was profiled. Only sizes and timings
were recorded. The copy held 209 sessions and 14,336 FTS docs, with 6 MiB of text.
The file was 444 MB, and `fts_messages_data` alone took 410 MiB.

| Startup phase, real cache copy | Time |
|---|---:|
| db_open_integrity | 18.526 s |
| catalog_hints | 98 ms |
| bootstrap | 6 ms |

98% of CPU went to FTS5's `xIntegrity`. A whole-database `quick_check` calls it, and
it runs the full index check even in quick mode. The index was mostly tombstones:
`replaceFTSDocs` deleted and reinserted every doc of a session on each change, and
FTS5 deletes stay in the index until a merge. A growing session added all of its
messages again on every append. After `optimize` and `VACUUM` the copy held 7.4 MiB
of FTS data in 19 MB, and `quick_check` took 235 ms.

Changes:

- `replaceFTSDocs` diffs against the stored docs. It inserts new ones, updates
  changed ones by rowid and deletes removed ones, so unchanged messages cost nothing.
- `checkIntegrity` runs `quick_check('sqlite_schema')`, which covers the freelist,
  then `quick_check(<table>)` on every ordinary table, FTS5 shadow tables included.
  It no longer runs FTS5's own check. Lost coverage: the check that every page is
  in use somewhere.
- At the start of its first run the indexer calls `maintainFTS`. It rebuilds the
  index from `fts_docs` when used pages exceed 16× the text plus 64 MiB, or when
  `quick_check('fts_messages')` fails. This cleans up caches already bloated, and
  repairs FTS damage that the startup check used to catch. There is no schema bump
  and no new database file.

Synthetic churn (same docs rewritten repeatedly):

| Write pattern | Write time | FTS data |
|---|---:|---:|
| Delete and reinsert all (before) | 1.28 s | 0.84 MiB |
| Same, with `secure_delete` | 15.6 s | 0.21 MiB |
| Diff (now) | 0.20 s | 0.38 MiB |

3,000 streaming updates to one session with the diff pattern ended at 1.08 MiB, against
0.62 MiB fully optimized: growth stays bounded. `secure_delete` was rejected for being
12× slower to write.

On the bloated copy, `quick_check('fts_messages_data')` took 1.8 s and
`quick_check('fts_messages')` took 17 s. A rebuild took 1.1 s, after which the FTS check
took 208 ms. The rebuild leaves a 433 MB WAL, which the `wal_checkpoint(TRUNCATE)`
after it removes. Known limits:

- The file keeps its size as free pages, because nothing runs `VACUUM`.
- The first launch after upgrading still spends about 1.8 s checking the bloated
  shadow table's b-tree before the rebuild runs.
- Full startup was not re-measured on the real cache after the change.

Tests in `internal/index/fts_maintain_test.go` cover:

- Only changed docs are written.
- `Open` keeps the catalog when the FTS index is damaged, and `maintainFTS` repairs it.
- A sound small index is not rebuilt, and a bloated one is.

Checks: full `go test ./...`, `go vet ./...` and `GOOS=windows go vet ./...` passed.
Race tests for index and engine passed (41.4 / 172.7 s). Lint was not run (not installed).

## IT-01 — Append and resolution baselines (2026-10-25)

Environment: Apple M5, 32 GiB RAM, macOS 27.0.1 (26A434), Go 1.26.8
darwin/arm64, default GOMAXPROCS=10. Generated fixtures under testing temporary
roots only; no private stores, app launches or native GUI measurements.

Command (three repeated one-operation observations per workload):

```sh
go test ./internal/engine -run '^$' \
  -bench '^(BenchmarkTranscriptAppend|BenchmarkCodexResolution|BenchmarkTranscriptRetention)$' \
  -benchtime=1x -count=3 -timeout=20m
```

Fixture generation, initial cache fill, append and payload encoding are outside
timing. Timed append work is invalidation plus the 50-message reload. Each sample
starts at the same source size and appends exactly 1, 10 or 100 JSONL records
(not turns); a partial final turn is intentional. A leading newline terminates
the fixture's existing valid tail and is included in append bytes. Counts exclude
initial loads. Every reload called the provider once. Resolution bytes are
`ScanWork.RecordBytes` (JSONL payload bytes, excluding delimiters), separate from
`LoadWork.SourceBytes` (transcript read). Times are warm-filesystem synthetic
observations, not disk-cold or user-session latency promises.

### Append measurements

Three values are runs 1 / 2 / 3. B/op is total allocation, not retained heap.

| Agent / MB / records | ms/op (three runs) | B/op (three runs) | Appended B | Transcript source B/op | Resolution record B/op |
|---|---|---|---:|---:|---:|
| claude/38MB/1records | 463.376 / 463.194 / 465.870 | 202478016 / 202474928 / 202478048 | 113 | 38155666 | 0 |
| claude/38MB/10records | 464.591 / 467.045 / 464.512 | 202490560 / 202493760 / 202491024 | 1505 | 38157058 | 0 |
| claude/38MB/100records | 465.885 / 454.542 / 469.675 | 202652648 / 202654016 / 202651696 | 15451 | 38171004 | 0 |
| claude/100MB/1records | 1214.233 / 1204.119 / 1210.761 | 506802992 / 506802688 / 506804040 | 113 | 100155794 | 0 |
| claude/100MB/10records | 1210.450 / 1208.551 / 1223.151 | 506818512 / 506818864 / 506818528 | 1505 | 100157186 | 0 |
| claude/100MB/100records | 1231.947 / 1219.491 / 1221.647 | 506978656 / 506980776 / 506978944 | 15451 | 100171132 | 0 |
| codex/38MB/1records | 828.792 / 836.039 / 788.004 | 244282120 / 244279688 / 244283192 | 168 | 38198401 | 38197118 |
| codex/38MB/10records | 772.758 / 830.119 / 824.048 | 244297016 / 244299112 / 244296360 | 1569 | 38199802 | 38198510 |
| codex/38MB/100records | 819.434 / 849.694 / 828.311 | 244482104 / 244482280 / 244482152 | 15681 | 38213914 | 38212532 |
| codex/100MB/1records | 2152.354 / 2181.555 / 2115.174 | 609420376 / 609420424 / 609420296 | 168 | 100198529 | 100197246 |
| codex/100MB/10records | 2126.759 / 2143.486 / 2128.943 | 609442136 / 609438200 / 609441544 | 1569 | 100199930 | 100198638 |
| codex/100MB/100records | 2163.249 / 2124.022 / 2139.309 | 609624296 / 609628344 / 609624168 | 15681 | 100214042 | 100212660 |

### Codex resolution scaling

Target: 38 MB. Extra rollouts: 64, each ≥1 MiB of prompts plus JSONL overhead.
`resolution-scan` measures the cold Scan used by Load's resolver independently;
`load` includes both resolution and the target transcript read. This isolates
the scan cost without introducing production timing instrumentation. The path
recheck in Load is included only in the full-load measurement.

| Extra rollouts / mode | ms/op (three runs) | B/op (three runs) | Resolution record B/op | Transcript source B/op |
|---|---|---|---:|---:|
| 0extra/resolution-scan | 165.559 / 164.147 / 166.419 | 81312368 / 81300184 / 81296920 | 38196812 | 0 |
| 0extra/load | 957.792 / 818.927 / 831.403 | 244270712 / 244270056 / 244270024 | 38196812 | 38198233 |
| 64extra/resolution-scan | 223.838 / 226.876 / 212.603 | 319805152 / 319817952 / 319761792 | 117909836 | 0 |
| 64extra/load | 892.247 / 850.860 / 854.634 | 482705632 / 482722504 / 482809136 | 117909836 | 38198233 |

### Retention and payload

Retention is a process-wide HeapAlloc delta after forced GC around one cache
fill, keeping the service and transcript reachable. It includes cache overhead
and is noisy. `model.TranscriptBytes` is deterministic structural accounting,
not a heap measurement: fixed Go layouts plus nested content/capacity, without
allocator rounding or sharing deduplication. All model string fields and nested
storage have generated unit tests. The older PF-02 test-only estimate is unchanged.

| Agent / MB | Measured retained B (three runs) | Estimated B (three runs) | Estimate / heap (three runs) |
|---|---|---|---|
| claude/38MB | 40400312 / 40400312 / 40400664 | 38232581 / 38232581 / 38232581 | 0.9463 / 0.9463 / 0.9463 |
| claude/100MB | 101692856 / 101692856 / 101692856 | 100232710 / 100232710 / 100232710 | 0.9856 / 0.9856 / 0.9856 |
| codex/38MB | 40403824 / 40404392 / 40403416 | 38233527 / 38233528 / 38233528 | 0.9463 / 0.9463 / 0.9463 |
| codex/100MB | 101696440 / 101694520 / 101694520 | 100233656 / 100233657 / 100233656 | 0.9856 / 0.9856 / 0.9856 |

| Agent / MB, one appended record | Full 50-message page JSON B | Changed message suffix JSON B |
|---|---:|---:|
| claude/38MB | 3574523 | 174 |
| claude/100MB | 9387035 | 174 |
| codex/38MB | 3574623 | 178 |
| codex/100MB | 9387135 | 178 |

Payload values match in all three runs. The changed suffix is determined by
deep comparison in absolute message coordinates; its JSON is a message array,
not the future delta envelope. The full page includes today's page envelope.
Long generated user prompts explain these large windows; these are not typical
production-message size claims. Metadata/title changes are not included in this
message-only comparison.

Decision: retain the planned 256 MiB estimated cache budget for IT-08. Two 100 MB
snapshots fit; four do not. The estimate is ~95–99% of measured retained heap
for these fixtures; it is an accounting weight, not a hard RSS ceiling. Provider
state will add its own accounting and the oversized-MRU exemption remains needed.

Verification: full `go test ./...`, `go build ./...`, `go vet ./...`,
`GOOS=windows go vet ./...` PASS. Focused `go test -race` for generated append,
transcript equivalence and concurrent-miss tests PASS. Estimator/fixture tests,
changed-file gofumpt and diff checks PASS. A separate `-benchtime=2x` append
check reports one load/op, verifying multi-iteration work accounting. Frontend
unchanged; frontend checks, native GUI, SSH/WSL and Windows execution unperformed.
No production loading behaviour changed; only the estimate helper was added.

## IT-02 — Incremental contract and continuity (2026-10-25)

Added the optional in-process incremental loader contract, consuming opaque
state once, immutable publications, conservative FirstChanged and full-load
fallback. Continuity marks validate the opened descriptor (or a path convenience
wrapper), file identity, committed length, same-size mtime rule and SHA-256
head/tail windows of at most 4096 bytes. Hash reads preserve the descriptor offset
and honor cancellation before and after bounded I/O. No production caller uses
the contract yet.

Generated continuity tests cover no change, growth, truncation, replacement,
same-size rewrite, head/tail edits, rename, deletion, short/empty committed
prefixes, invalid offsets, cancellation and completing an uncommitted tail.
The explicit interior-rewrite limit is tested with growth and with unchanged
length/mtime; both can evade the windows and are not content proofs.

The reusable equivalence harness compares a cold provider, checks conservative
FirstChanged and retains deep copies of every publication, including nested
raw JSON, tools, files and child refs. A fake visits each generated line boundary
and the harness rejects high FirstChanged, mutated publications and stale state
after truncation. Read-error/cancellation tests verify state is not reused on
the next attempt. Concrete provider split-point matrices follow in IT-04/IT-05.

Verification: `go test -race ./internal/provider ./internal/provider/providertest
-count=1` PASS; full `go test ./...`, `go build ./...`, `go vet ./...`,
`GOOS=windows go vet ./...` PASS. Changed-code gofumpt and diff checks PASS.
Frontend unchanged; native GUI/SSH/WSL and Windows execution unperformed.

## IT-03 — Copy-on-write message builder (2026-10-25)

Added shared transcriptbuild storage: Append, read-only At, Touch, ToolLoc/Tool,
clipped Freeze with earliest-change tracking, and independent Fork writers.
The first mutation per generation detaches the message array; each touched
frozen message clones parts, raw tool input, tools, child refs, attachments and
file-name slices. Appends detach too, preventing fork writers from sharing
spare array capacity. Parser-owned maps remain the provider's responsibility.

Message weights are reconciled only for touched/appended messages at Freeze;
Bytes excludes transcript metadata and spare top-level capacity. Deterministic
randomized generations match a full MessageBytes recompute. Deep equivalence
tests cover old snapshots, repeated touches, appended/forked generations and
refreezes. Eight concurrent frozen readers plus a writer pass the race detector.

Allocation check (Apple M5 / Go 1.26.8, same IT-01 environment):

```sh
go test ./internal/provider/transcriptbuild -run '^$' \
  -bench BenchmarkTouchGeneration -benchtime=1x -count=3
```

Empty-message generations isolate top-level copying from nested payload clones.
100 touches: 11.209 / 12.791 / 15.917 µs, 16,384 B/op and **1 alloc/op**.
10,000 touches: 879.209 / 932.458 / 953.458 µs, 1,441,792 B/op and
**1 alloc/op**. This is one O(n) array copy per publication generation, not
an O(n) copy per touched record. Nested mutable content still allocates when
first touched, by design.

Verification: full Go test/build/vet and Windows cross-vet PASS; final focused
builder race tests, randomized accounting, vet and Windows cross-vet PASS.
Changed-code gofumpt and diff checks PASS. Frontend unchanged; native GUI,
SSH/WSL and Windows execution unperformed. Providers are not migrated yet.


## IT-04 — Claude resumable loading (2026-10-25)

Claude now implements the optional incremental contract. Complete lines advance
parser/checkpoint state; a valid unterminated tail is applied only to a disposable
copy-on-write fork. Each publication refreshes live/ended tool status, child links,
subagent metadata and CWD/repository metadata. FirstChanged includes earlier tool
results, split assistants, compaction summaries, discarded tails and changed
finalization. State beyond the published snapshot is capped at 64 MiB; diagnostics
are not retained. Shared text is not charged twice. A file whose size/mtime changes
during parsing/continuity marking is not admitted as resumable state. As with the
shared continuity contract, rewrites preserving identity, relevant timestamps and
hash windows are outside detection guarantees.

Synthetic evidence only, Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
`go test ./internal/provider/claude -run '^$' -bench BenchmarkIncrementalAppend
-benchtime=1x -count=3 -timeout=10m` excludes fixture generation, initial cold
load, append and reset from timing. Raw output: `/tmp/as-it04-bench-final.txt`.

| Source / appended records | Three provider refresh measurements (ms) | Transcript bytes read | Allocated bytes/op |
|---|---|---:|---:|
| 38 MB / 1 | 0.186 / 0.226 / 0.525 | 295 | 1,349,344 |
| 38 MB / 10 | 0.425 / 0.259 / 0.255 | 1,701 | 1,365,264 |
| 38 MB / 100 | 0.545 / 0.563 / 0.580 | 15,782 | ~1,526,100 |
| 100 MB / 1 | 0.209 / 0.206 / 0.363 | 295 | 1,349,344 |
| 100 MB / 10 | 0.172 / 0.246 / 0.231 | 1,701 | ~1,365,280 |
| 100 MB / 100 | 0.582 / 0.649 / 0.582 | 15,782 | ~1,526,130 |

Every case additionally reads 16,384 continuity bytes (validate + mark).
The transcript read includes the prior valid unterminated tail, not just new
bytes. Estimated additional state is ~145–146 KB. These provider-only timings do
not measure engine paging, transport, frontend or native GUI responsiveness;
IT-07 must integrate resume into the cache before app refreshes benefit.

Validation:
- Cold output compared byte-for-byte with pre-IT-04 HEAD for all six local JSONL
  fixtures and a generated 4 MiB/512-message fixture; identical.
- Incremental harness visits every complete-line boundary and valid pre-newline
  tail of those fixtures; generated append 1/10/100, earlier error tool result,
  image/sidechain, invalid tail/line, tail replacement, >1 MiB buffered record,
  sidecar addition/removal/rename and child metadata all match cold loads.
- Darwin generated live evidence for the current test PID verifies live → ended;
  concurrent frozen readers and cap/accounting regressions pass. No app launched.
- Focused race suites, `go test ./...`, `go build ./...`, `go vet ./...`, and
  `GOOS=windows go vet ./...` pass. One overlapping verification run encountered
  a transient reflect import error while source edits were in progress; the
  subsequent full run passed. Frontend unchanged; native GUI/SSH/WSL/Windows
  execution unperformed. No private session data or QA perf script used.

## IT-05 — Codex resumable loading (2026-10-25)

Codex now implements the optional incremental contract on `transcriptbuild`.
Resumable state holds:
- the assistant ID map, so a split or replayed assistant can reach any earlier
  message;
- the current turn and the model;
- tool locations;
- the line base;
- the continuity mark.

The DeepEqual replay suppression compares against the stored message unchanged.
A valid unterminated tail is applied only to a disposable fork. Each call still
resolves through the cold Scan (IT-06 replaces it). A different canonical path
rebuilds. Unpaired calls become unknown at publish unless `meta.Live` is set.
FirstChanged includes the earliest message any parser touch, discarded tail or
finalization change reached. State beyond the published snapshot is capped at
64 MiB. Diagnostics are not retained. A parse whose size or mtime changed during
reading or marking is not kept. The retained-storage helper shared with Claude
moved to `transcriptbuild.Unshared`.

Synthetic evidence only, Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
`go test ./internal/provider/codex -run '^$' -bench BenchmarkIncrementalAppend
-benchtime=1x -count=3 -timeout=20m` excludes fixture generation, the initial cold
load, the append and the reset from timing. Raw output:
`/tmp/as-it05-eq.ASEGJY/bench.txt`.

| Source / appended records | Three provider refresh measurements (ms) | Transcript bytes read | Resolution record bytes | Allocated bytes/op |
|---|---|---:|---:|---:|
| 38 MB / 1 | 156.4 / 156.6 / 156.1 | 309 | 38,197,119 | ~82,650,000 |
| 38 MB / 10 | 157.1 / 153.5 / 158.2 | 1,719 | 38,198,520 | ~82,665,000 |
| 38 MB / 100 | 158.6 / 159.3 / 160.8 | 15,921 | 38,212,632 | ~82,854,000 |
| 100 MB / 1 | 426.1 / 421.1 / 412.4 | 309 | 100,197,247 | ~204,521,000 |
| 100 MB / 10 | 416.0 / 418.3 / 421.0 | 1,719 | 100,198,648 | ~204,537,000 |
| 100 MB / 100 | 417.0 / 416.3 / 422.2 | 15,921 | 100,212,760 | ~204,724,000 |

Every case also reads 16,384 continuity bytes. Estimated additional state is
~153–155 KB.

The transcript read dropped from the whole file to the appended bytes plus the
prior tail. The time left is almost entirely resolution: IT-01 measured the cold
resolution scan alone at ~165 ms for 38 MB. IT-01's engine reloads, with the same
appends, took ~0.8 s (38 MB) and ~2.1 s (100 MB). Those figures also include
engine paging, so they are context, not a like-for-like comparison. IT-06 removes
the resolution scan. IT-07 must integrate resume into the cache before app
refreshes benefit. No engine, transport, frontend or native GUI timing was
measured.

Validation:
- **Cold output.** Compared byte-for-byte (JSON, plus pass diagnostics) with the
  pre-change tree. The fixtures were a synthetic conversation covering every
  record family, an invalid-only rollout, a generated 4 MiB/512-message rollout
  and a missing ID. All were identical.
- **Harness boundaries.** Every complete-line boundary and every valid pre-newline
  tail of the synthetic conversation matches a cold load. The conversation
  covers:
  - split and replayed assistants, plus an assistant reached again after a user
    turn;
  - reasoning summaries and raw content;
  - earlier and much later tool outputs, including a truncated large output;
  - unknown-call notices;
  - compaction and turn aborts;
  - the historic envelope-less message and session_meta forms;
  - images, unknown types and an invalid line.
- **Other harness cases.** Valid, invalid and replaced tails, generated append
  1/10/100, a buffered record over 1 MiB, cancellation, a canonical path move and
  concurrent frozen readers all pass.
- **Work counts.** A one-record resume makes one provider call and one open, and
  reads exactly the appended bytes plus ≤16 KiB continuity.
- **Internal tests.** Live → ended → live, the state cap and dropped diagnostics
  are covered.
- **Mutation checks.** Each of these deliberate breakages made a test fail:
  - a high FirstChanged;
  - committing the tail;
  - forgetting the old tail's range;
  - dropping the finalization diff;
  - finalizing on the committed store.
- **Repo checks.** Race suites for the provider packages, `go test ./...`,
  `go build ./...`, `go vet ./...` and `GOOS=windows go vet ./...` pass. Frontend
  unchanged. No native GUI, SSH, WSL or Windows execution was performed. No
  private session data or QA perf script was used.

## IT-06 — Incremental Codex resolution (2026-10-25)

Each Codex `Provider` now owns a private resolver
(`internal/provider/codex/resolve.go`). Before IT-06, every `Load`, `LoadIncremental`
and `Blob` ran a cold `Scan(ScanState{})` over every rollout. The resolver keeps
the last private `ScanState` and runs `Scan(ctx, prev)`, so unchanged rollouts
are stat-checked and not read. The `state_N.sqlite` index is still read on every
resolution, so overlays, edges and WAL updates are always current.

Design notes:

- **Full canonical set, not a fold.** The brief proposed applying Changed and
  Removed to a retained map. The scan already computes every canonical session,
  so an internal `scan` returns that set and `Scan` wraps it unchanged. A fold
  would compare against previous metadata whose `CWDMissing` and `RepoRoot` are
  recomputed. Reading the code, a deleted cwd on an unchanged rollout would then
  never be reported, and the fold would keep `CWDMissing=false`. Taking the full
  set makes every resolution equal a cold scan's Changed metadata by
  construction.
- **Serialized and coalescing.** The lock is a one-slot channel, so a waiting
  caller can abandon on cancellation. Each caller records the scan sequence when
  it arrives. It reuses a result only from a scan that began after that, so it
  always observes writes made before the call. Concurrent callers share one scan.
- **Failures.** A failed warm scan drops the state and retries cold. One example
  is a previously scanned rollout that can no longer be proven removed, such as
  one replaced by a directory. Cancellation returns without touching the
  retained state. State is replaced only after a successful scan.
- **Cap.** Checkpoints plus canonical metadata are capped at 32 MiB. Over the cap
  the call still answers, then drops its state, so the next load is cold, as
  before IT-06.
- **Unchanged.** The `indexRolloutPath` recheck, the error texts, the PF-07C
  production bypass and the `SourceLoader` paths are unchanged:
  - `source.go` is untouched;
  - declined `LoadFromSource`/`BlobFromSource` calls still perform no scan;
  - `establishSource` still uses its own cold scan;
  - the fallback `Load` now benefits from warm resolution.
- **Known limitations.** These are shared with the incremental catalog scan, not
  introduced here:
  - a same-size rewrite that keeps the same mtime is not detected;
  - a checkpoint keeps the cwd normalized at parse time, so if a symlinked cwd
    is deleted later, the stored form stays resolved.

Synthetic evidence only: Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
Raw output is in `/tmp/as-it06-bench.LbWXBv/{resolution,append}.txt`.

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkCodexResolution$' -benchtime=1x -count=3 -timeout=20m
go test ./internal/provider/codex -run '^$' -bench BenchmarkIncrementalAppend -benchtime=1x -count=3 -timeout=20m
```

`BenchmarkCodexResolution` now has four modes:

- `resolution-scan`: a cold scan alone, unchanged.
- `cold-load`: a new provider per load, which is what every load cost before
  IT-06.
- `warm-load`: a repeated load of unchanged sources.
- `warm-append`: append one record outside timing, then `LoadIncremental` with
  the previous state, so both resolution and transcript continue.

The first `warm-append` run also completes the fixture's valid unterminated
final record. That is why its bytes are 309 rather than 169.

| Extra rollouts / mode | ms/op (three runs) | B/op (run 1) | Resolution opens/op | Resolution record B/op | Transcript source B/op |
|---|---|---:|---:|---:|---:|
| 0extra/resolution-scan | 157.4 / 157.5 / 154.1 | 81,303,240 | 1 | 38,196,812 | 0 |
| 0extra/cold-load | 750.4 / 774.4 / 801.6 | 244,442,984 | 1 | 38,196,812 | 38,198,233 |
| 0extra/warm-load | 594.4 / 569.8 / 624.7 | 163,433,864 | 0 | 0 | 38,198,233 |
| 0extra/warm-append | 0.81 / 0.79 / 0.83 | 2,736,048 | 1 | 309 / 169 / 169 | 311 / 170 / 170 |
| 64extra/resolution-scan | 192.5 / 186.9 / 191.4 | 319,228,024 | 65 | 117,909,836 | 0 |
| 64extra/cold-load | 839.5 / 848.6 / 761.1 | 482,535,960 | 65 | 117,909,836 | 38,198,233 |
| 64extra/warm-load | 625.9 / 628.0 / 663.2 | 179,782,408 | 0 | 0 | 38,198,233 |
| 64extra/warm-append | 16.6 / 17.5 / 18.1 | 19,097,024 | 1 | 309 / 169 / 169 | 311 / 170 / 170 |

Compared with IT-01's scaling table (`load`, ~0.83–0.96 s at 0 extra and
~0.85–0.89 s at 64 extra):

- A warm load of an unchanged session reads no rollout records during
  resolution. It allocates 163–180 MB rather than 244–483 MB, and its time is now
  the full transcript parse.
- Continuing after a one-record append takes ~0.8 ms with no other rollouts, and
  ~17 ms with 64 others.

The remaining 64-extra cost is not file reading. Each warm scan decodes and
re-encodes every rollout checkpoint (285k allocations, 19 MB). That scales with
the number of rollouts and the size of the prompts in their checkpoints. A
future scanner change could skip re-encoding unchanged checkpoints. IT-06 leaves
the scan format as is.

IT-05's provider benchmark (`BenchmarkIncrementalAppend`, which uses no extra
rollouts) now resumes in 0.7–1.5 ms at both 38 MB and 100 MB. IT-05 measured
~156 ms and ~420 ms. Resolution reads 307, 1,708 and 15,820 record bytes for 1,
10 and 100 appended records, down from the whole file. Transcript bytes (309,
1,719 and 15,921), continuity (16,384) and estimated state (~153–155 KB) are
unchanged from IT-05.

Validation:

- **Equivalence matrix (`TestResolverMatchesColdScan`).** One provider steps
  through a sequence of changes. After each step, the resolver's canonical map
  must DeepEqual a fresh cold `Scan(ScanState{})`. Steps marked "no read" must
  open and read no rollout. The steps, in order:
  - initial (an active and an index-selected archived duplicate, a child with
    an edge, and an orphan with a real cwd);
  - unchanged (no read);
  - a title overlay (no read);
  - an invalid index row (no read);
  - an edge change (no read);
  - a WAL-only update, after checking the `-wal` file is non-empty (no read);
  - a higher-numbered `state_7.sqlite` (no read);
  - a cwd removed (no read; `CWDMissing` matches cold);
  - an append;
  - an added newer archived duplicate;
  - a replaced file;
  - truncation;
  - removal of the index-selected archived source (no read; the active copy
    takes over);
  - removal.
- **Warm work (`TestResolverWarmWork`).** With six rollouts, a cold resolution
  opens 6. An unchanged warm resolution opens 0 and reads 0 record bytes. After
  a one-record append it opens 1 and reads exactly the appended record, and the
  load equals a new provider's load.
- **Cap and fallback.** With the cap at 0, the call still resolves, retains
  nothing, and reopens every source on each load. A rollout replaced by a
  directory fails the warm scan; resolution falls back to cold, matches a cold
  scan, and reports the session as not found.
- **Cancellation.** A caller waiting on the lock abandons with its deadline. A
  cancelled scan leaves state and canonical map unchanged, and the next warm
  load opens 0 sources.
- **Coalescing.** A caller that arrived before a scan reuses it, with zero
  checkpoint decodes. A caller arriving after a rewrite rescans and sees the
  rewrite. 16 concurrent resolutions agree, and the race detector is clean.
- **Updated counters.** These existing tests counted one cold scan per load and
  were updated without weakening what they prove:
  - `TestSourceProductionBypass` still requires declines to do no scan work.
    Fallbacks now open only the appended rollout: 3 opens for 3 appends, where
    the old cold scans made 6.
  - `TestSourceAcceptsWithoutUnrelatedParsing` proves the counters are live
    through a new provider's cold load, which opens 2.
  - The PF-07C mutation matrix, which compares fallback loads with a new
    provider after 15 kinds of index/source mutation, passes unchanged.
- **Mutation checks.** Each of these deliberate breakages made a test fail:
  - reusing a scan that did not start after arrival (`>=`);
  - always scanning cold;
  - an empty canonical set;
  - no warm-error fallback;
  - dropping state on cancellation;
  - ignoring the cap;
  - a wrong freshness sequence.
- **Repo checks.** All of these pass:
  - race suites for `internal/provider/codex` and `internal/engine`;
  - `go test ./...`, `go build ./...`, `go vet ./...` and
    `GOOS=windows go vet ./...`;
  - `git diff --check`.

  gofumpt reports nothing for the changed lines. Its remaining findings, in
  `source_policy_test.go`, were already there. Frontend unchanged. No native
  GUI, SSH, WSL or Windows execution was performed. No private session data or
  QA perf script was used.

## IT-07 — Resumable engine cache entries (2026-10-25)

The engine transcript cache now keeps provider state with each entry, and
`loadTranscript` goes through `provider.LoadIncremental`. An append no longer
forces a cold parse in the app's refresh path.

Behaviour:

- **Stale, not evicted.** `EvictTranscripts(changed, …)` from `onChanged` marks a
  changed entry stale and detaches its flight. A stale entry is never served, so
  the PF-04 guarantee holds: no snapshot older than an invalidation is returned
  after it. The next load continues its state. An entry without state (a
  non-incremental provider, or a state over the provider cap) is evicted as
  before.
- **Dropped.** Removed refs, `Forget` (`EvictTranscript`), `ClearTranscripts`
  (bootstrap and rebuilds) and engine stop drop entries and state, so the next
  load is cold. The same happens to flights in progress.
- **One owner per key.** State is consumed once, so each key has at most one
  owner flight, which holds the state or will produce it. A new flight continues
  a detached predecessor's result:
  - it waits without a load slot or provider call, and its wait counts as a
    waiter, so the predecessor is not cancelled when its own callers leave;
  - if the predecessor fails, the successor loads cold;
  - if the successor is cancelled first, ownership returns to the predecessor
    under the same lock hold, and the predecessor's result is kept as stale state;
  - a cancelled predecessor publishes nothing, so a new flight does not wait for
    it. It takes only the state that predecessor never passed to the provider.
    An uncooperative provider therefore cannot delay a retry; this is what the
    PF-05 orphan test needs.
  - non-incremental providers never wait.
- **Unused state survives.** A flight cancelled before the provider consumed its
  state (for example, while queued for a slot) returns it to the stale entry.
- **Limits.** Stale entries still count towards the four-entry LRU, so heavy use
  of other sessions can evict them; IT-08 replaces this with a byte budget. A
  predecessor that is cancelled after the provider consumed its state loses it,
  and the next load is cold.

Synthetic evidence only: Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
Raw output is in `/tmp/as-it07-bench.Gd0xSU/append.txt`.

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscriptAppend$' -benchtime=1x -count=3 -timeout=20m
```

This is IT-01's engine benchmark: invalidate as `onChanged` does, then reload the
50-message live window. The fixture, initial load and append are outside timing.
The benchmark now calls `EvictTranscripts(changed, nil)`, as `onChanged` does,
instead of the dropping single-key eviction.

| Agent / MB / records | ms/op (three runs) | IT-01 ms/op (run 1) | B/op (run 1) | Transcript source B/op | Resolution record B/op |
|---|---|---:|---:|---:|---:|
| claude/38MB/1 | 0.215 / 0.241 / 0.243 | 463.4 | 1,358,456 | 294 | 0 |
| claude/38MB/10 | 0.274 / 0.192 / 0.242 | 464.6 | 1,374,344 | 1,686 | 0 |
| claude/38MB/100 | 0.518 / 0.511 / 0.558 | 465.9 | 1,534,792 | 15,632 | 0 |
| claude/100MB/1 | 0.131 / 0.238 / 0.206 | 1,214.2 | 1,358,456 | 294 | 0 |
| claude/100MB/10 | 0.236 / 0.247 / 0.261 | 1,210.5 | 1,374,344 | 1,686 | 0 |
| claude/100MB/100 | 0.569 / 0.570 / 0.533 | 1,231.9 | 1,534,808 | 15,632 | 0 |
| codex/38MB/1 | 0.780 / 0.728 / 0.735 | 828.8 | 2,744,792 | 308 | 306 |
| codex/38MB/10 | 0.804 / 0.783 / 0.753 | 772.8 | 2,763,224 | 1,709 | 1,698 |
| codex/38MB/100 | 1.039 / 1.001 / 1.347 | 819.4 | 2,964,808 | 15,821 | 15,720 |
| codex/100MB/1 | 0.788 / 0.759 / 0.689 | 2,152.4 | 2,741,400 | 308 | 306 |
| codex/100MB/10 | 0.797 / 0.829 / 0.720 | 2,126.8 | 2,762,936 | 1,709 | 1,698 |
| codex/100MB/100 | 0.999 / 1.002 / 1.018 | 2,163.2 | 2,964,936 | 15,821 | 15,720 |

Every reload made one provider call. The transcript read is the appended bytes
plus the fixture's previously unterminated tail, independent of file size.
Allocation fell from 202–610 MB to 1.4–3.0 MB per reload. The remaining allocation
is mostly building the page. The window and changed-suffix JSON sizes are
unchanged from IT-01; IT-10 and IT-11 address the payload. These are
engine-level timings: they include paging, but no RPC, transport, frontend or
native GUI work.

Validation:

- **Barrier tests.** `providertest.IncrementalBarrier` announces each attempt's
  `Prev` and accepts an `IncrementalResult` reply. New engine tests cover:
  - a stale resume, twice in a row;
  - a failed resume, after which the retry is cold;
  - a queued flight cancelled before using its state, which returns the state;
  - invalidation during a resume, then predecessor success, failure and
    cancellation. The successor waits with no provider call and no slot, and
    continues from the predecessor's state or goes cold;
  - successor cancellation returning ownership to a predecessor that is still
    running, which keeps its detached result as stale state;
  - removal, Forget, clear and stop/restart, each with a resident entry and with a
    flight in progress, dropping state, and a dropped flight not replacing the
    reload;
  - a non-resumable provider not waiting for a detached predecessor, and its
    stateless entry being freed on invalidation.
- **PF-04/PF-05 regressions.** The ten existing barrier tests now each run twice:
  with the plain barrier, and with a stateful incremental gate that attaches
  state to every reply. Together they cover publication after invalidation,
  shared success and cancellation, the load bound, lifetime, orphans,
  uncooperative shutdown and the last waiter. All pass in both modes.
- **Work counts (`TestTranscriptAppendBurstResumes`).** The real Claude and Codex
  providers run through `Service` with a 2 MB fixture. There is one cold parse,
  then eight append/invalidate/reload rounds. Each round makes one provider call
  and reads at most the appended bytes plus 4 KiB. Each served snapshot
  DeepEquals a new provider's cold load.
- **Mutation checks.** Each of these deliberate breakages made a test fail:
  - not taking a stale entry's state;
  - not waiting on a predecessor;
  - not restoring the owner;
  - losing unused state;
  - never recording a stale entry;
  - keeping stateless stale entries;
  - waiting on a cancelled predecessor;
  - eviction keeping the owner;
  - a successor ignoring its predecessor's state;
  - not keeping a detached result.

  An earlier version had a window in which a predecessor could finish between its
  successor giving up and finalizing. No test could reach that window, so the two
  steps now share one lock hold and the branch is gone.
- **Repo checks.** All of these pass:
  - five race runs of the cache tests;
  - race runs of `internal/provider/providertest`;
  - `go test ./...`, `go build ./...`, `go vet ./...` and
    `GOOS=windows go vet ./...`;
  - gofumpt on the changed files;
  - `git diff --check`.

  Frontend unchanged. No native GUI, SSH, WSL or Windows execution was
  performed. No private session data or QA perf script was used.

## IT-08 — Byte budget and revisions (2026-10-25)

The engine transcript cache is now bounded by estimated bytes as well as by
count, and every published snapshot has a revision that can say which messages
changed since an earlier one. IT-10 builds the delta API on this.

Behaviour:

- **Entry bytes.** An entry weighs its snapshot estimate plus the provider
  state's `RetainedBytes`. The estimate is updated incrementally:
  `model.TranscriptBytesSince` subtracts the previous snapshot's suffix from
  `FirstChanged` and adds the new one, plus metadata and the message array. A
  rebuild, or a load without state, recomputes it in full. A resume point (state,
  the snapshot it was produced with, its estimate, and the earliest change since
  the entry's latest revision) moves as one value between the entry, the flight
  resuming it and that flight's successor.
- **Budget.** `transcriptByteBudget` is 256 MiB, as IT-01 decided, and the
  four-entry count cap stays. After each publish, and whenever a detached result
  is kept as stale, least-recently-used entries are evicted until the total fits.
  Stale entries count. The most recently used entry is never evicted for size, so a
  session larger than the budget stays resident while it is being read.
- **Revisions.** Revisions are opaque `epoch.generation.sequence` strings. The
  epoch is random and changes on clear and on restart. A publish into a stale
  entry continues its generation; any other publish starts a new one, from a
  process-wide counter. `Service.loadTranscriptRevision` returns the revision with
  the snapshot. A reader of a detached flight gets the snapshot with no revision.
- **History.** Each entry keeps a 16-slot ring of the `FirstChanged` of its
  publishes after the generation's first, indexed by sequence.
  `changedSince(key, base, target)` returns the minimum over `(base, target]`, or
  `math.MaxInt` when nothing changed. It returns `ok = false` for:
  - an unknown or malformed revision;
  - another epoch, generation or key;
  - a base after the target, or a target that was never issued;
  - a base older than the ring.

  A rebuild records 0. A detached result's `FirstChanged`, and a predecessor's
  passed to its successor, are carried into the next published revision.
- **Invalidation.** An entry without state is still evicted on invalidation,
  except while an owner flight is running. That owner holds the entry's state, so
  the entry stays as stale and keeps its history for the owner's result. If the
  owner then fails, the entry is dropped: there is nothing left to resume.

Synthetic evidence only: Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
Raw output is in `/tmp/as-it08-bench.XZkJfS/append.txt`.

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscriptAppend$' -benchtime=1x -count=3 -timeout=20m
```

| Agent / MB / records | ms/op (three runs) | IT-07 ms/op (run 1) | B/op (run 1) | Transcript source B/op |
|---|---|---:|---:|---:|
| claude/38MB/1 | 0.147 / 0.235 / 0.245 | 0.215 | 1,358,536 | 294 |
| claude/38MB/10 | 0.258 / 0.197 / 0.277 | 0.274 | 1,374,424 | 1,686 |
| claude/38MB/100 | 0.534 / 0.488 / 0.489 | 0.518 | 1,534,872 | 15,632 |
| claude/100MB/1 | 0.138 / 0.215 / 0.200 | 0.131 | 1,358,536 | 294 |
| claude/100MB/10 | 0.255 / 0.232 / 0.251 | 0.236 | 1,374,424 | 1,686 |
| claude/100MB/100 | 0.497 / 0.578 / 0.571 | 0.569 | 1,534,888 | 15,632 |
| codex/38MB/1 | 0.747 / 0.755 / 0.752 | 0.780 | 2,744,392 | 308 |
| codex/38MB/10 | 0.763 / 0.775 / 0.770 | 0.804 | 2,759,128 | 1,709 |
| codex/38MB/100 | 0.973 / 1.012 / 0.994 | 1.039 | 2,961,528 | 15,821 |
| codex/100MB/1 | 0.678 / 0.743 / 0.747 | 0.788 | 2,745,320 | 308 |
| codex/100MB/10 | 0.718 / 0.704 / 0.789 | 0.797 | 2,763,096 | 1,709 |
| codex/100MB/100 | 1.026 / 0.956 / 0.973 | 0.999 | 2,961,656 | 15,821 |

The reload is still independent of file size, with one provider call each.
Keeping the estimate and the revision adds about 80 B per reload, and no
measurable time. A full `TranscriptBytes` walk of a 100 MB snapshot is not
repeated on resume.

Validation:

- **Budget (`TestTranscriptByteBudget`).** Mixed sizes evict several LRU entries
  until the rest fit. A cache hit protects an entry. An oversized MRU entry stays
  and is served without reloading. An exact fit keeps both entries. Stale entries
  and kept detached results count.
- **Estimates (`TestTranscriptEstimateAndChangesAfterRandomResumes`).** 400
  seeded rounds of resumes, rebuilds, detached results and failures, with random
  truncations, appends, spare capacity and metadata changes. After every round the
  entry's estimate equals `TranscriptBytes` plus `RetainedBytes`. Each published
  revision reports exactly the minimum `FirstChanged` since the previous one, and
  never more than the first real difference. A lost generation reports a reset.
  `TestTranscriptBytesSince` checks the model helper directly over 500 random steps.
- **History (`TestTranscriptRevisionHistory`).** Every pair of 21 and then 23
  revisions is checked against the expected minimum, across ring overflow. A
  reset base works, a rebuild reports 0, and malformed, unissued, other-key and
  evicted revisions are rejected.
- **Epoch (`TestTranscriptEpochChanges`).** Clear and stop/start change the epoch,
  and earlier revisions are rejected.
- **Unpublished results (`TestTranscriptRevisionsAcrossDetachedResumes`).** These
  run through the barrier:
  - invalidation during a resume keeps the entry, and the detached reader gets no
    revision; the next revision reports the detached `FirstChanged`;
  - a predecessor's change carries through its successor;
  - a failed predecessor makes the successor's revision report 0;
  - a failed owner drops the entry and its history.
- **Real providers.** `TestTranscriptAppendBurstResumes` now also checks, after
  each of eight Claude and Codex appends, that the estimate equals a full
  recompute. The reported change is never past the first real difference, and
  never earlier than the last two messages.
- **Mutation checks.** Each of these 17 deliberate breakages made a test fail:
  - always starting a generation;
  - an estimate from too high an index;
  - not carrying unpublished changes;
  - no MRU exemption;
  - ignoring state bytes;
  - no fit after keep;
  - invalidation ignoring the owner;
  - no ring-overflow check;
  - clear keeping the epoch;
  - keeping a stateless stale entry;
  - an off-by-one ring index;
  - not resetting the pending change;
  - a rebuild keeping `FirstChanged`;
  - not checking the generation;
  - accepting an unissued target;
  - accepting a base after the target;
  - giving detached readers a revision.
- **Repo checks.** All of these pass:
  - five race runs of the cache tests;
  - `go test ./...`, `go build ./...`, `go vet ./...` and
    `GOOS=windows go vet ./...`;
  - gofumpt on the changed files;
  - `git diff --check`.

  golangci-lint reports nothing new in the changed code. Frontend unchanged. No
  native GUI, SSH, WSL or Windows execution was performed. No private session
  data or QA perf script was used.

## IT-09 — Shared FTS loading (2026-10-25)

FTS jobs now load transcripts through the engine's cache, so a job for a live
session resumes from where the last load stopped instead of parsing the whole
file again. The cache entries for sessions the user has open are not affected.

Behaviour:

- **Injected loader.** `index.TranscriptLoader` has one method,
  `LoadForIndex(ctx, ref)`. `StartIndexer` and `IndexPending` take a loader
  instead of a provider set. `index.ProviderLoader(providers)` is the old
  behaviour: a full provider `Load`, and the same "no provider" error. The engine
  passes its `Service`. `index` still does not import `engine`, and
  `replaceFTSDocs` and its revision checks are unchanged.
- **Always validated.** `Service.LoadForIndex` never returns a cached snapshot and
  never joins a flight already running: either could have read the file before
  the scan that queued the job. Instead it marks the entry stale, detaches any
  current flight and starts a new one. That flight resumes the latest state (the
  entry's, the detached flight's result, or the background slot's) and reads to
  the current end of the file. A foreground flight detached this way still
  serves its readers, without a revision.
- **Foreground entries.** If the session has a foreground entry, or a foreground
  flight was detached, the result is published to that entry as a new revision
  with the usual `FirstChanged` history. The entry's recency is unchanged, so
  indexing never promotes a session over ones the user opened more recently.
- **Background slot.** Otherwise the resume point goes to one background slot,
  outside the LRU and its byte budget. The slot is capped by
  `transcriptBackgroundBudget`, 128 MiB, which fits a 100 MB session. A larger
  result is not kept, and that session's next job parses cold. A job for another
  session replaces the slot. The slot is never served. A foreground load of its
  session takes its state and resumes. Eviction and clear drop the slot;
  invalidation keeps it, because index loads resume to the end of the file
  anyway.
- **Other providers.** For a provider without incremental loading,
  `LoadForIndex` calls `Load` directly and leaves the cache alone.

Synthetic evidence only: Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
Raw output is in `/tmp/as-it09-bench.N9LwcD/fts.txt`.

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkFTSLoadAppend$' -benchtime=1x -count=3 -timeout=30m
```

`BenchmarkFTSLoadAppend` loads a session that is not open, appends one turn, and
then times the next FTS load. It runs once with the provider loader and once with
the service.

| Agent / MB | Provider ms/op (three runs) | Service ms/op (three runs) | Provider source B/op | Service source B/op | Provider B/op | Service B/op |
|---|---|---|---:|---:|---:|---:|
| claude/38MB | 452 / 448 / 454 | 0.176 / 0.154 / 0.155 | 38,156,172 | 800 | 202,623,672 | 1,356,208 |
| claude/100MB | 1,194 / 1,171 / 1,204 | 0.219 / 0.227 / 0.247 | 100,156,300 | 800 | 506,949,776 | 1,356,240 |
| codex/38MB | 649 / 649 / 650 | 0.707 / 0.737 / 0.732 | 38,199,018 | 925 | 164,517,616 | 2,744,536 |
| codex/100MB | 1,702 / 1,697 / 1,704 | 0.706 / 0.780 / 0.988 | 100,199,146 | 925 | 407,783,952 | 2,740,488 |

Both make one provider call per job. With the service, a job reads only the
appended bytes, and its time does not depend on file size. A 100 MB session
indexes about 2,000 to 5,000 times faster, and allocates about 150 to 370 times
less.

Validation:

- **Always current (`TestLoadForIndexResumesToCurrentSource`).** This uses the
  real Claude and Codex providers on a 2 MB generated fixture. Each check appends
  a turn and does not invalidate the cache, as if the scanner had not reported it
  yet. The index snapshot must equal a cold load, and the load may read only the
  appended bytes plus 4 KiB. The test covers three phases:
  - **Not open.** After one cold parse, three jobs resume from the background
    slot, and no LRU entry is created.
  - **Opened.** The first foreground load promotes the slot and resumes.
  - **Open.** Three jobs advance the fresh foreground entry. Each following
    foreground hit returns the index snapshot, with a revision whose change is
    within the appended messages.

  The run makes eight provider calls in all.
- **Detaching (`TestLoadForIndexDetachesForegroundFlight`).** These run through
  the barrier:
  - an index load during a foreground flight waits for that flight and resumes
    its state; the foreground reader gets no revision, and the index result is
    published and then hit;
  - a foreground reader joining a background index flight makes its result a
    foreground entry, which becomes the most recent.
- **Foreground isolation (`TestLoadForIndexKeepsForegroundOrder`).** Four
  foreground entries total 80 of a 100-unit budget. Background jobs of 80 units
  each, for three other sessions, leave the entries and their order unchanged.
  Advancing the least recently used entry resumes its state and keeps it last.
  It also publishes a revision that the next foreground load hits.
- **Background slot (`TestLoadForIndexBackgroundSlot`).** The test covers:
  - an exact fit is kept, and one byte over is not;
  - another session replaces the slot;
  - eviction and clear drop it, and invalidation keeps it;
  - a failed load leaves no state;
  - no LRU entries are created.
- **Other providers (`TestLoadForIndexNonResumable`).** Index loads parse in full
  every time. They do not replace a foreground entry, and they create no flights
  or background state. An unknown agent returns `ErrUnknownProvider`.
- **Index package (`TestFTSInjectedLoader`).** Jobs use the injected loader, and
  the default loader keeps the missing-provider error. All existing index tests
  pass with `index.ProviderLoader`.
- **Mutation checks.** Each of these 14 deliberate breakages made a test fail:
  - ignoring the resident entry when choosing foreground;
  - ignoring the detached flight when choosing foreground;
  - a joining foreground reader not marking its flight foreground;
  - background publishes touching recency;
  - no promotion from the background slot;
  - no background budget;
  - a background budget ignoring state bytes;
  - eviction keeping the background slot;
  - clear keeping the background slot;
  - background results going to the LRU;
  - index loads serving a cached hit;
  - index loads joining a running flight;
  - the service bypassing the cache for incremental providers;
  - the service caching non-incremental providers.
- **Repo checks.** All of these pass:
  - five race runs of the cache tests;
  - `go test ./...`, `go build ./...`, `go vet ./...` and
    `GOOS=windows go vet ./...`;
  - gofumpt on the changed files;
  - `git diff --check`.

  golangci-lint reports nothing new in the changed code. Frontend unchanged. No
  native GUI, SSH, WSL or Windows execution was performed. No private session
  data or QA perf script was used.

## IT-10 — Transcript delta API (2026-10-25)

A client that already holds a window of a transcript can now ask for a page
without the leading messages it holds unchanged. Older hosts and App bindings
keep working: the client falls back to `GetMessages`.

Behaviour:

- **Revisions on pages.** `MessagesPage.Revision` carries the IT-08 revision of
  the snapshot a page was cut from. It is omitted when empty, so pages from older
  hosts and unpublished snapshots look as before.
- **`GetMessagesSince`.** The request is a page (`offset`, `limit`, with -1 for
  the latest) plus the client's window: `baseRevision`, `haveOffset` and
  `haveCount`. The reply is that page with its first `reused` messages removed;
  the client's messages `[offset-haveOffset, offset-haveOffset+reused)` stand in
  for them. `reused` is the overlap of the page with the client's window that
  `changedSince(base, revision)` reports unchanged. It is 0, which makes the
  reply a full page, when:
  - the base is empty, unknown, from another generation or epoch, or older
    than the 16 retained publishes;
  - the page starts before the client's window.

  A reply never holds more than `limit` messages, reused ones included, and
  validation matches `GetMessages`, plus non-negative `have` fields.
- **Compatibility.** `engine.TranscriptDeltaBackend` is an optional interface,
  following the `discoveryStatus` pattern:
  - The RPC server advertises the `transcriptDelta` capability when its backend
    implements it, and answers `getMessagesSince` with `CodeMethodNotFound`
    otherwise.
  - The client sends nothing without the capability. It maps both a missing
    capability and `CodeMethodNotFound` to `ErrUnsupportedAction`.
  - `App.GetMessagesSince` returns `{supported, delta, host, generation}`. An
    unsupported backend gives `supported: false` and no error, so the frontend
    can fall back per connection. Other errors surface. An offline host keeps its
    disconnected error.
  - The frontend API reports `supported: false` when the App binding has no
    `GetMessagesSince`. The browser mock does the same until IT-11 adds a
    revision history.
- **Providers without incremental loading.** Every load starts a generation, so
  their deltas after a change are full pages. An unchanged session still reuses
  its whole window.

Synthetic evidence only: Apple M5 / Go 1.26.8 darwin/arm64, default GOMAXPROCS 10.
Raw output is in `/tmp/as-it10-bench.F2hQL4/delta.txt`.

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkMessagesDeltaAppend$' -benchtime=1x -count=3 -timeout=30m
```

`BenchmarkMessagesDeltaAppend` loads the latest 50-message window, appends 1 or
10 records, then times invalidation (as `onChanged` does) plus
`GetMessagesSince` with the old window. It checks that the old window's reused
messages plus the reply equal a fresh page, then reports the JSON of both.

| Agent / MB / records | ms/op (three runs) | Reused | Delta JSON B | Full page JSON B |
|---|---|---:|---:|---:|
| claude/38MB/1 | 0.156 / 0.175 / 0.205 | 48 | 691 | 3,574,567 |
| claude/38MB/10 | 0.202 / 0.264 / 0.253 | 43 | 1,836 | 3,277,578 |
| claude/100MB/1 | 0.148 / 0.221 / 0.222 | 48 | 691 | 9,387,079 |
| claude/100MB/10 | 0.270 / 0.183 / 0.245 | 43 | 1,836 | 8,605,714 |
| codex/38MB/1 | 0.736 / 0.758 / 0.736 | 48 | 695 | 3,574,667 |
| codex/38MB/10 | 0.795 / 0.785 / 0.780 | 45 | 1,492 | 3,426,238 |
| codex/100MB/1 | 0.676 / 0.734 / 0.718 | 48 | 695 | 9,387,179 |
| codex/100MB/10 | 0.753 / 0.785 / 0.753 | 45 | 1,492 | 8,996,562 |

Byte counts match in all three runs. After one appended record, the reply is
about 700 bytes instead of 3.6 or 9.4 MB. Of the 49 messages the moved window
shares with the old one, 48 are reused. The last one is resent because the
provider reports the open turn as possibly changed. The generated fixtures have
long user prompts, so the full-page sizes are not typical of real sessions.
Times include the resumed provider load and are not end-to-end latencies: RPC,
Wails and rendering are not measured.

Validation:

- **Randomized windows (`TestMessagesDeltaMatchesFullPages`).** A client window
  is updated only by applying deltas, for 600 rounds. Before each round the
  in-memory incremental provider may:
  - append messages or truncate the tail;
  - change an earlier message's tool result;
  - rebuild instead of resuming;
  - report an earlier `FirstChanged` than the real first difference.

  The cache is invalidated or evicted. Requests follow the tail, refresh the
  window in place, or ask for a random page, sometimes with an empty or foreign
  base. After every round the window must equal a fresh `GetMessages` of the
  same revision, with the same page fields, and hold no more than `limit`
  messages. Reuse must only come from the client's window. The run reuses in 275
  rounds, 229 of them across a change, and resets in 325.
- **Reuse bounds (`TestMessagesDeltaReuse`).** Exact counts for an unchanged
  session: a whole window, a later page, a page starting before the window, a
  window longer than the transcript, an empty window, no base, and a page past
  the end. It also covers validation errors and an unknown provider.
- **RPC (`TestMessagesDeltaRoundTrip`, `TestMessagesDeltaOldHostFallback`).**
  Through the real dispatcher with an `Engine`, the capability is advertised, and
  the remote reply equals the local one for a full reuse and for a reset after a
  change. An invalid request returns an error, not `ErrUnsupportedAction`. An old
  host has no capability. Both a missing capability and a forced capability
  answered with `CodeMethodNotFound` fall back, and `getMessages` still works.
  `transcriptDelta` and `revision` are omitted when unset.
- **App (`TestMessagesSinceReplyRoutes`).** Local and remote replies carry the
  route's host and generation. A backend without the method, or one answering
  `ErrUnsupportedAction`, gives `supported: false` with no `delta` field. Other
  errors surface, and an offline host returns `ErrDisconnected`.
- **Frontend (`messages-delta-api.test.mjs`).** The binding receives the request
  and its reply passes through unchanged. Errors stay errors. A missing binding
  and the mock report unsupported.
- **Mutation checks.** Each of these 20 deliberate breakages made a test fail:
  - dropping the check that the page starts inside the client's window;
  - dropping each of the three bounds on reuse: the changed index, the client's
    window and the page;
  - allowing negative reuse;
  - each of the four validation checks;
  - not trimming reused messages;
  - pages without revisions;
  - the client ignoring its capability, or not mapping `CodeMethodNotFound`;
  - the server never, or always, advertising the capability;
  - `transcriptDelta` or `revision` without `omitempty`;
  - the App surfacing `ErrUnsupportedAction`, or not setting `supported`;
  - the offline backend reporting unsupported instead of disconnected.

  An "empty revision" guard did not change any result, because `changedSince`
  already rejects empty revisions, so it was removed.
- **Repo checks.** All of these pass:
  - race runs of the engine, RPC and App packages;
  - `go test ./...`, `go build ./...`, `go vet ./...` and
    `GOOS=windows go vet ./...`;
  - gofumpt on the changed files;
  - `git diff --check`;
  - frontend `npm test` (743 tests), `npm run check` and `npm run build`.

  `TestTranscriptBaselineEquivalence` now expects an evicted reload to have a
  new revision, and compares the rest of the page as before. No native GUI, SSH,
  WSL or Windows execution was performed. No private session data or QA perf
  script was used.

## IT-11 — Frontend delta refresh (2026-10-25)

A live transcript refresh now asks only for the messages that changed since the
window's revision. Unchanged messages keep their objects, so their keyed rows
are not re-rendered. Hosts and App bindings without IT-10 keep the full-page
refresh.

Behaviour:

- **Refresh.** `TranscriptView.reloadMessages` hands a snapshot of its window
  (`messages`, `pageOffset`, `hasMore`, `revision`) to `TranscriptRefresher`
  (`frontend/src/lib/transcriptRefresh.ts`). It requests the same page as
  before: the latest `max(window, 50)` messages while following, or the
  window's own offset while reading history. The window's messages
  `[offset-haveOffset, offset-haveOffset+reused)` plus the reply's become the
  new window. These are taken from the snapshot, so later splices cannot leak
  in. Following, anchoring, load generations, the pending-load wait and the
  "scrolled into history meanwhile" retry are unchanged. Superseded reloads are
  still dropped by generation. Search jumps and every other page load record
  the page's revision. Translations are keyed by message in their store, so
  they are unaffected.
- **Resets.** A reply with `reused: 0` replaces the window. A delta whose reused
  range is not inside the snapshot also falls back to `getMessages`; the engine
  never sends one.
- **Fallback.** An unsupported reply switches the connection to `getMessages`.
  The connection is identified by the data epoch, which moves on every
  (re)connect, so a reconnect to an upgraded host tries deltas again. A window
  without a revision, from an older host, never sends a delta request, so old
  hosts pay no extra round trip.
- **Mixed revisions.** "Load earlier" and "load more" now also drop a page when
  the window moved while it was in flight. A page whose revision differs from
  the window's is still spliced in, but the window keeps its older revision and
  a refresh follows. That refresh's delta resends everything changed since the
  older revision, including inside the new page. "Load more" refreshes only
  after its pending load clears.
- **Mock.** `MockBackendAPI` keeps the last 16 revisions of each transcript and
  answers `getMessagesSince` with the engine's reuse formula. A page gets a new
  revision whenever the serialized messages change. `transcriptDelta = false`
  makes it answer like an old host.

Validation (`frontend/test/transcript-refresh.test.mjs`, against the mock):

- a tail append while following: 2 new messages are sent and 48 reused, as the
  same objects; an unchanged refresh sends nothing;
- a reading-history window refreshed in place;
- an earlier tool-result change, resent from that message;
- a rewrite and an unknown base revision, both resets;
- an unsupported host: one delta request, then full pages until a reconnect,
  after which deltas resume; a window without a revision never asks;
- superseded reloads answered newest first: each is a consistent page of its
  own window and revision;
- mixed-revision pages: detected, then repaired by one delta;
- deltas whose reused range does not fit the window are rejected, falling back
  to `getMessages`;
- 300 randomized rounds of appends, deletions, edits and cleared histories, with
  following, in-place and random pages. The window, updated only through the
  refresher, must equal a fresh `getMessages` every round. Partial reuse and
  resets must both occur often, and no full page may be fetched.

`messages-delta-api.test.mjs` now also checks that the mock answers deltas and
validates requests. All 12 deliberate breakages of `transcriptRefresh.ts` made
a test fail:

- the reused slice;
- each of the three range checks;
- recording, or honouring, an unsupported connection;
- requesting deltas without a revision;
- the revision or `hasMore` of the result;
- both mixed-revision conditions;
- the `haveOffset` sent.

`npm test` (754), `check` and `build` pass. The Svelte wiring is covered by
`check` and the build only: component tests render on the server. Native GUI
behaviour was not tested.

## IT-12 — Integrated evidence and milestone gate (2026-10-25)

Milestone 2 gate. Revision `33fb3a7` on `feature/improve-performance`, plus
this section and the new `BenchmarkTranscriptCacheBudget`. Apple M5 / 32 GiB,
Darwin 27.0.0 arm64, Go 1.26.8, Node 22.23.2, default GOMAXPROCS 10, the same
host as IT-01. Only generated fixtures in test temporary roots were used.
Historical sections above are unchanged. Raw output is in
`/tmp/as-it12-bench.SF76pt/`.

### Deterministic gates

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | pass |
| `GOOS=windows go vet ./...` | pass |
| `go test -count=1 ./...` | pass: 37 packages |
| `go test -race -count=1` engine, `provider/...` (8 packages), files, index, rpc, app | pass (13 packages) |
| `gofumpt -l` on every Go file changed since the M2 design commit | two files listed, both for lines M2 did not touch: missing blank lines between `offlineBackend` methods in `internal/app/offline.go`, and `0600` literals in `internal/provider/codex/source_policy_test.go`. Left as they are. |
| `npm test` / `npm run check` / `npm run build` | 754/754 pass / 0 errors (existing `ignoreDeprecations` warning) / pass (existing chunk-size advisory) |

Not performed: native GUI QA of any M2 change, SSH or WSL hosts, Windows or
Linux test execution (Windows sources were vetted only), cold-disk or peak-RSS
profiling, real session stores, and `scripts/qa-perf-bench.sh`.

### Before/after: append then reload (IT-01 → now)

```sh
go test ./internal/engine -run '^$' \
  -bench '^(BenchmarkTranscriptAppend|BenchmarkCodexResolution|BenchmarkTranscriptRetention)$' \
  -benchtime=1x -count=3 -timeout=40m
```

Median of three one-operation runs. Timed work is invalidation, as `onChanged`
does, plus the latest 50-message page. Source bytes are the transcript read.

| Agent / MB / records | ms/op before → now | B/op before → now | Source B before → now | Resolution record B before → now |
|---|---:|---:|---:|---:|
| claude/38/1 | 463.4 → **0.156** | 202.5 MB → **1.36 MB** | 38,155,666 → **294** | 0 → 0 |
| claude/38/10 | 464.6 → **0.193** | 202.5 MB → **1.37 MB** | 38,157,058 → **1,686** | 0 → 0 |
| claude/38/100 | 465.9 → **0.526** | 202.7 MB → **1.53 MB** | 38,171,004 → **15,632** | 0 → 0 |
| claude/100/1 | 1,210.8 → **0.144** | 506.8 MB → **1.36 MB** | 100,155,794 → **294** | 0 → 0 |
| claude/100/10 | 1,210.5 → **0.189** | 506.8 MB → **1.37 MB** | 100,157,186 → **1,686** | 0 → 0 |
| claude/100/100 | 1,221.6 → **0.532** | 507.0 MB → **1.53 MB** | 100,171,132 → **15,632** | 0 → 0 |
| codex/38/1 | 828.8 → **0.728** | 244.3 MB → **2.75 MB** | 38,198,401 → **308** | 38,197,118 → **306** |
| codex/38/10 | 824.0 → **0.799** | 244.3 MB → **2.76 MB** | 38,199,802 → **1,709** | 38,198,510 → **1,698** |
| codex/38/100 | 828.3 → **0.988** | 244.5 MB → **2.96 MB** | 38,213,914 → **15,821** | 38,212,532 → **15,720** |
| codex/100/1 | 2,152.4 → **0.687** | 609.4 MB → **2.74 MB** | 100,198,529 → **308** | 100,197,246 → **306** |
| codex/100/10 | 2,128.9 → **0.744** | 609.4 MB → **2.76 MB** | 100,199,930 → **1,709** | 100,198,638 → **1,698** |
| codex/100/100 | 2,139.3 → **1.007** | 609.6 MB → **2.96 MB** | 100,214,042 → **15,821** | 100,212,660 → **15,720** |

Every reload still makes one provider call. The bytes read are the appended
records plus the fixture's earlier unterminated tail, independent of file size.
Continuity checks add at most 16 KiB of hash-window reads, not counted above.

**Codex resolution, 38 MB target.** `resolution-scan` is the cold scan alone;
`cold-load` uses a new provider, as every load did before IT-06.

| Extra rollouts / mode | ms/op (median) | Resolution opens/op | Resolution record B/op | Transcript source B/op |
|---|---:|---:|---:|---:|
| 0 / resolution-scan | 163.9 | 1 | 38,196,812 | 0 |
| 0 / cold-load | 726.0 | 1 | 38,196,812 | 38,198,233 |
| 0 / warm-load | 573.6 | **0** | **0** | 38,198,233 |
| 0 / warm-append | 0.714 | 1 | 169–309 | 170–311 |
| 64 / resolution-scan | 183.8 | 65 | 117,909,836 | 0 |
| 64 / cold-load | 769.2 | 65 | 117,909,836 | 38,198,233 |
| 64 / warm-load | 590.0 | **0** | **0** | 38,198,233 |
| 64 / warm-append | 16.6 | 1 | 169–309 | 170–311 |

IT-01's `load` row (a cold load, 64 extras) was 854.6 ms with 117.9 MB of
resolution reads per load. A warm load now reads no unchanged rollout. The
first `warm-append` run also completes the fixture's unterminated tail, which
explains its 309 bytes. These match IT-06.

**Retention.** One cold `loadTranscript`: the estimate is 94.3% (38 MB) and
98.4% (100 MB) of the measured live-heap delta, as in IT-01. The snapshot
estimate excludes provider state, which IT-04 and IT-05 measured at ~145–155 KB.

### The other M2 workloads

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkFTSLoadAppend$' -benchtime=1x -count=3 -timeout=30m
go test ./internal/engine -run '^$' -bench '^BenchmarkMessagesDeltaAppend$' -benchtime=1x -count=3 -timeout=30m
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscriptCacheBudget$' -benchtime=1x -count=3 -timeout=30m
```

**FTS job after one appended turn**, for a session not open in the UI. The
provider loader is the pre-IT-09 path.

| Agent / MB | Provider loader ms (median) | Service ms (three runs) | Source B: provider → service |
|---|---:|---|---:|
| claude/38 | 460.1 | 0.252 / 0.239 / 0.183 | 38,156,172 → **800** |
| claude/100 | 1,218.7 | 0.248 / 0.232 / 0.216 | 100,156,300 → **800** |
| codex/38 | 653.4 | 0.782 / 0.769 / 0.751 | 38,199,018 → **925** |
| codex/100 | 1,594.8 | 0.698 / 0.706 / 0.712 | 100,199,146 → **925** |

**Live window refresh payload.** After one appended record, the
`GetMessagesSince` reply for the 50-message window is 691 B (Claude) or 695 B
(Codex), against full pages of 3,574,567–9,387,179 B, with 48 messages reused.
After ten records it is 1,836 B or 1,492 B, with 43 or 45 messages reused. Times
are 0.19–0.88 ms. Byte counts are identical in all three runs and to IT-10.

**Cache budget (new: `BenchmarkTranscriptCacheBudget`).** One service opens
four 100 MB sessions in turn. One record is appended to the last, which is then
reloaded; only that reload is timed. Then the benchmark reads what the cache
keeps.

| Agent | Resident entries | Estimated B | Budget B | Live heap delta B | Reload ms (three runs) |
|---|---:|---:|---:|---:|---|
| claude | 2 | 200,686,489 | 268,435,456 | 203,275,008–203,294,680 | 0.227 / 0.214 / 0.205 |
| codex | 2 | 200,705,054–200,705,056 | 268,435,456 | 202,182,928–202,227,048 | 1.526 / 1.440 / 1.505 |

M1 kept all four (~400 MB). The two most recent fit within the 256 MiB budget,
and the append still resumes (one provider call) after the evictions. The
benchmark fails if the estimate exceeds the budget.

### PF-02 transcript benchmark (PF-02 → now)

```sh
go test ./internal/engine -run '^$' -bench '^BenchmarkTranscript$' -benchmem -benchtime=1x -count=3 -timeout=40m
```

| Workload | PF-02 median ms | Now median ms | B/op PF-02 → now | Notes |
|---|---:|---:|---|---|
| claude/38 cold-page | 403.1 | 448.3 | 202.47 → 202.62 MB | see A/B below |
| claude/100 cold-page | 1,019.0 | 1,141.6 | 506.79 → 506.95 MB | see A/B below |
| claude/100 invalidated | 1,040.8 | 1,134.3 | 506.79 → 506.95 MB | `EvictTranscript` drops state, so this stays a full parse |
| codex/38 cold-page | 596.0 | 538.0 | 244.26 → **163.43 MB** | warm resolution: 0 resolution bytes |
| codex/100 cold-page | 1,663.5 | 1,425.0 | 609.42 → **406.70 MB** | warm resolution: 0 resolution bytes |
| codex/100 invalidated | 1,680.4 | 1,421.3 | 609.42 → **406.69 MB** | warm resolution |
| codex/100 same-key-4 | 1,867.7 | 1,418.8 | 2,437.7 → **406.7 MB** | one load (PF-05), warm resolution |
| codex/100 independent-key-4 | 1,897.4 | 1,949.7 | 2,437.7 → 2,235.5 MB | four new providers, so four cold resolutions |
| warm page, any | 0.002–0.004 | 0.003–0.006 | 8,240 → 8,352 B; 2 → 5 allocs | the revision string |

The benchmark reuses one provider across iterations. For Codex, resolution is
therefore warm after the first load, which removes the second full read that M1
noted. Pages now carry `revision`: +44 bytes of JSON. Claude cold rows are higher
than PF-02's, which was run a day earlier. An interleaved A/B against the pre-M2
tree (`a985a9a`, extracted with `git archive`), on the same host and alternating
which ran first, shows no regression:

| Claude cold-page, six alternating runs | Pre-M2 ms (sorted) | Now ms (sorted) | Median pre-M2 → now |
|---|---|---|---:|
| 38 MB | 418.4 419.1 430.9 439.1 443.0 451.7 | 408.8 416.6 430.9 439.1 455.1 461.3 | 435.0 → 435.0 |
| 100 MB | 1078.2 1128.1 1149.2 1170.6 1191.4 1205.9 | 1085.9 1110.5 1128.9 1198.2 1207.5 1213.1 | 1159.9 → 1163.6 |

Six alternating `claude/38MB/provider-cold` runs of three iterations each agree:
medians of 441.9 (pre-M2) and 432.1 ms (now). Absolute times on this host vary
by ~10% between sessions. Cold-load allocation rises by ~155 KB, the
resumable state.

A repeat after the IT-12 commit (`83166e5`) used prebuilt test binaries of
both trees and six pairs of one `claude/38MB/cold-page` run each, alternating
which ran first. It agrees: pre-M2 397.4 408.4 414.1 421.0 421.7 435.7 ms
(median 417.6) against now 393.8 402.6 415.2 420.6 427.9 432.2 ms (median
417.9). The per-pair difference (now − pre-M2) ranges from −33.1 to +17.7 ms
and averages −1.0 ms. B/op is 202.47 → 202.62 MB.

### Acceptance gates

Measurements (design § Measurements):

| Workload | Expected | Result |
|---|---|---|
| Claude/Codex 38/100 MB, append 1/10/100 records then reload | appended bytes only | **met**: the appended records plus the prior unterminated tail, and ≤16 KiB of continuity reads; 0.14–1.0 ms |
| Codex resolution, 64 extra rollouts, cold vs warm | warm reads 0 unchanged rollout bytes | **met**: 0 opens and 0 record bytes for a warm load; an append reads only the appended record |
| FTS job after an append | resume | **met**: 800/925 B read, 0.18–0.78 ms |
| Live refresh payload, 50-message window plus 1 append | ~1 message | **met**: 691/695 B, 48 of 49 shared messages reused; the open turn is resent |
| Retained cache bytes, four 100 MB sessions | within budget | **met**: 2 entries, 200.7 MB estimated ≤ 256 MiB |

Correctness gates (design § Correctness gates):

- **Split-point equivalence: met.** The IT-04 and IT-05 harnesses visit every
  complete-line boundary and every valid pre-newline tail. Cases include local
  and synthetic fixtures, and generated 4 MiB/512-message files. Each compares
  against a cold `Load` and rejects a too-high `FirstChanged`.
- **Immutability: met.** The IT-02 harness deep-copies every publication. The
  IT-03 builder runs eight concurrent frozen readers plus a writer under the race
  detector. Both providers run concurrent frozen readers, and all M2 packages
  pass the race suite above.
- **Continuity matrix: met, with the documented limit.**
  - IT-02 covers truncation, replacement, same-size rewrite, head/tail window
    edits, rename and deletion.
  - IT-04 covers sidecar addition, removal and rename, and live → ended.
  - IT-05 covers live → ended → live and a canonical path move.
  - IT-06 covers duplicates, index overlays, edge changes, WAL updates, a
    removed index-selected source and removal.
  - An interior rewrite that keeps identity, length, mtime and both hash windows
    is not detected (see Limits).
- **Cache: met.** Barrier tests cover a stale resume, predecessor chaining and
  cancellation, and the PF-04/PF-05 races in both stateless and stateful modes
  (IT-07). The budget, with the MRU entry exempt, and the revision ring and epoch
  reset are covered by IT-08. Background-slot isolation is covered by IT-09.
- **Wire: met.** Capability gating, old-host and `CodeMethodNotFound` fallback,
  and 600 randomized delta rounds equal to full pages are covered by IT-10. 300
  frontend rounds through the refresher are covered by IT-11.

Not met or not measured:

- Native GUI behaviour of the live refresh. The frontend is verified by unit
  tests, `check` and `build` only.
- SSH and WSL payloads. The payload sizes above are engine replies, not
  measured on a transport.

### Limits

- **Undetected same-length in-place rewrite.** A rewrite inside the file that
  preserves identity, length, mtime and the 4 KiB head and tail hash windows
  resumes from stale state. The scanner checkpoints share this limit.
- **O‍penCode** keeps its full `Load`. Every change re-parses it, and it has no
  resumable state; its deltas after a change are full pages.
- **FTS still rewrites a session's documents** after every job. Only the
  transcript load is incremental (M4 owns FTS batching).
- **First opens are full parses:** ~0.44 s (Claude) and ~0.73 s (Codex, new
  provider) for 38 MB; ~1.15 s and ~1.4–1.9 s for 100 MB.
- **`Forget`, removal, clear and a provider without state** still drop the entry,
  so the next load is cold.
- **Stale entries count toward the four-entry LRU and the budget**, so with
  heavy multi-session use the oldest state is still dropped.

### Remaining bottlenecks

1. **First open of a large session.** It parses the whole file and allocates
   ~5× its size: 507 MB (Claude) and 407–610 MB (Codex) for 100 MB. A
   tail-first or windowed load would let the latest page appear before the full
   parse finishes.
2. **Codex warm resolution with many rollouts.** An append costs 16.6 ms and
   19 MB of allocation with 64 extra rollouts, against 0.7 ms with none. The
   cost is per-rollout checkpoint decoding and stat work, not reading. It grows
   with the store and is shared with the PF-10 checkpoint issue.
3. **FTS document rewrites** on each live job (M4).
4. **Full first page payload.** The first open still sends the whole 50-message
   window: 3.6–9.8 MB with these long generated prompts. The UI renders every
   loaded message (virtualization is M5).
5. **Cold start and catalog** costs are unchanged from PF-15.

### Milestone 2 outcome

**12/12 complete** (IT-01…IT-12). A live append now costs work proportional to
the appended bytes in the provider, engine, FTS and wire paths. Memory is
bounded by the byte budget. Cold first loads are unchanged. Native GUI, SSH and
WSL verification remain unperformed and should be done when an app run is
requested.

## PF-10A — Decode once per scan (2026-10-25)

First half of PF-10, reopened after Milestone 2
([brief](tasks/performance-m1.md#pf-10a--decode-once-per-scan-skip-unchanged-re-encodes-3-h-pf-01)).
Same host as IT-12: Apple M5 / 32 GiB, Darwin 27.0.0 arm64, Go 1.26.8,
GOMAXPROCS 10. Generated fixtures in test temporary roots only.

### Change

`scanSource` already decodes an unchanged rollout's previous checkpoint before
its size/mtime skip. The scan now keeps that decode for the rest of the scan:

- **Overlay pass.** It reuses the decode, and keeps the existing bytes when the
  canonical overlay is unchanged or there is none. Encoding decoded bytes
  reproduces them, so the bytes are what the old re-encode produced. It encodes
  only when the overlay changes, from a copy, so the shared decode keeps the old
  overlay.
- **Previous-state pass.** It reuses the same decode: for an unchanged source the
  previous and new bytes are the same slice.

Changed, appended and removed sources take the old path. The decoded checkpoints
are dropped from the outcomes before `scan` returns, so callers that keep `byID`
(the IT-06 resolver) retain nothing new. No checkpoint schema, version or
persisted byte changes.

### Equivalence

`TestScanCheckpointReuseMatchesFullRecode` runs two providers over the same
root. One reuses checkpoints. The other has a test-only switch that takes the
pre-PF-10A path (decode and re-encode everything).

- **Cases.** The IT-06 mutation matrix, now shared with
  `TestResolverMatchesColdScan` and extended with two steps:
  - an index archived-flag change and a deleted index row;
  - an ID-less `rollout-.jsonl`, which has no canonical entry;
  - a final unchanged scan.
- **Checks.** At every step the whole `ScanResult` must be deep-equal, including
  Changed, Removed, diagnostics and every checkpoint byte. Steps that read no
  rollout bytes must decode exactly once per previous source, and encode exactly
  as many checkpoints as changed bytes.
- **Mutants.** Three were killed: always skip the reused re-encode; re-encode
  when there is no canonical entry; write the overlay into the shared decode.
  The last makes Changed miss index edits.

Updated baselines: `TestSyntheticScanBaseline` (Codex warm 192/64 → **64/0**
decodes/encodes for 64 sources; append 192/65 → **66/2**) and
`TestScanIndex_ChangedTitleOnUnchangedRolloutAndFallback` (3/1 → **1/1**).

### Before/after

Prebuilt test binaries of `5d270d4` (before) and the working tree (after); six
alternating rounds, each one `-count=1` run.

```sh
go test ./internal/provider -run '^$' -bench '^BenchmarkSyntheticScan$/^codex$/^(unchanged|one-source-append)$' -benchmem
go test ./internal/engine -run '^$' -bench '^BenchmarkCodexResolution$/^(0|64)extra$/^warm-append$' -benchmem -benchtime=20x
```

| Workload | ms/op before → after (median of 6) | Range after | B/op before → after | allocs/op before → after | decodes / encodes per op |
|---|---:|---|---:|---:|---|
| PF-01 Codex scan, 64 sources unchanged | 3.06 → **2.21** | 2.20–2.25 | 1.36 → **1.02 MB** | 10,748 → **7,727** | 192/64 → **64/0** |
| PF-01 Codex scan, one source appended | 3.21 → **2.39** | 2.38–2.40 | 2.42 → **2.10 MB** | 10,769 → **7,812** | 192/65 → **66/2** |
| Codex warm append, 0 extra rollouts | 0.61 → 0.62 | 0.60–0.63 | 2.59 → 2.59 MB | 5,651 → 5,651 | — |
| Codex warm append, 64 extra rollouts | 16.74 → **3.41** | 3.37–3.47 | 18.96 → **7.83 MB** | 285,001 → **81,031** | — |

Before ranges: 3.01–3.22, 3.19–3.45, 0.60–0.70 and 16.55–18.17 ms. Bytes read
are unchanged (one 169-byte record per append, 0 for unchanged scans).

The 64-extra append gains most because its 1 MiB rollouts carry larger
checkpoints (assistant and call ID sets), and each scan used to decode them
three times and re-encode them once. That append is now 5.5× the 0-extra case,
down from 27×.

### Verification

- `go build ./...`, `go vet ./...`, `GOOS=windows go vet ./...`: pass.
- `go test -count=1 ./...`: pass (37 packages).
- `go test -race` on `provider`, `provider/codex` and `provider/all`: pass.
- `gofumpt -l` on the changed files: clean. The package's only listed file is
  the untouched `source_policy_test.go`.

Not performed: native GUI, SSH, WSL, Windows execution.

### Limits and what remains

- **Peak memory within a scan.** The decodes of unchanged sources now live
  until the scan's end, instead of one at a time. Allocation per scan falls, but
  the transient peak holds every unchanged source's decoded checkpoint at once.
  Nothing is retained after the scan.
- **One decode per unchanged source per scan remains.** That is PF-10B's cache.
- An appended source still decodes three times (resume, overlay, previous state)
  and encodes twice, as before. Only unchanged sources take the reuse path.
- Each unchanged source still pays its `CWD` existence check and Git root lookup
  twice per scan (scan and previous-state pass). This is not checkpoint work and
  is not changed here.

## PF-10B — Cross-scan checkpoint cache (2026-10-25)

Provider-private immutable decoded checkpoints shared by catalog scans and the
IT-06 resolver. Exact path, checkpoint bytes and current version authorize a
hit; source size/mtime alone cannot. Mutable append parsing still decodes its
own maps. Canonical overlays copy the checkpoint and preserve the old overlay.
Only a successful scan publishes a replacement snapshot; concurrent publication
can evict useful entries, but cannot authorize different checkpoint bytes.

Admission is deterministic discovery order, at most 1,024 sources and a 16 MiB
retained-state **estimate**. The estimate includes owned JSON, decoded strings,
struct/slice storage, conservative map-entry overhead and path/bookkeeping.
It is not a measured heap bound. A small generated source accounts for 4,291
bytes. Oversized or evicted sources fall back to PF-10A. Reset scans still parse
sources; empty successful scans discard entries. No schema/version changes.

### Repeated synthetic measurements

Same host/toolchain as PF-10A. Prebuilt binaries of `515edcf` and the working tree,
six alternating rounds, `-benchtime=300ms`, generated temporary roots only:

```sh
go test ./internal/provider -run '^$' -bench 'BenchmarkSyntheticScan/codex' -benchmem -benchtime=300ms
go test ./internal/engine -run '^$' -bench 'BenchmarkCodexResolution/.*/warm-append' -benchmem -benchtime=300ms
```

| Workload | ms/op before → after (median of 6) | After range (ms) | B/op before → after | allocs/op before → after |
|---|---:|---:|---:|---:|
| Codex scan, cold 64 sources | 6.07 → 7.03 | 5.94–9.79 | 68.78 → 68.84 MB | 17,878 → 17,967 |
| Codex scan, unchanged 64 sources | 2.24 → 2.57 | 2.05–3.53 | 1.02 → 1.00 MB | 7,727 → 6,588 |
| Codex scan, one source appended | 2.44 → 2.19 | 2.15–3.86 | 2.10 → 2.08 MB | 7,811 → 6,689 |
| Codex warm append, 0 extras | 0.57 → 0.50 | 0.48–0.59 | 2.71 → 2.66 MB | 5,646 → 4,588 |
| Codex warm append, 64 extras | 3.47 → 2.36 | 2.34–3.33 | 7.83 → 3.59 MB | 81,027 → 11,168 |

Unchanged scan decodes/encodes: **64/0 → 0/0**. A chained one-source append
now requires **2/2**, rather than 66/2. The scan benchmark deliberately reuses
its original previous state instead of chaining results, so the appended source
misses the cache on subsequent iterations and reports approximately 3/2.
Read work is unchanged: no records for unchanged scans, one record per append.
Warm resolution reads one 169-byte record, with one source open.

Unchanged entries reuse owned raw bytes and decoded values; new canonical
overlays are JSON-normalized into owned strings so short metadata cannot pin
large source buffers. Small-scan timing is noisy: unchanged and cold medians
regress in this run despite lower warm allocations; no small-scan latency win is
claimed. Larger resolution checkpoints show a clear allocation/work reduction.
The remaining 64-extra penalty includes discovery/stat, index validation and
CWD/Git checks twice per source. Cold parsing work remains unchanged, with added
admission/overlay ownership overhead. Beyond the cap, sorted discovery admission
favors earlier paths (often archives); zero-decode claims apply only to admitted
sources, not arbitrarily large stores.

### Verification

- Differential mutation matrix against PF-10A compares complete results and state
  bytes, including index/edge/archive changes, duplicates, append, rewrite,
  truncation and source removal. Unchanged steps assert zero decodes.
- Cold/new-provider/reset/version/raw-identity, admission/eviction, canceled and
  failed publication, immutable previous bytes and concurrent scan tests pass.
- `go test ./...`, affected provider race tests, `go build ./...`, native vet
  and Windows cross-vet pass. Cross-vet is not native Windows execution.
- Frontend `npm test`, `npm run check` and `npm run build` pass.
- Native GUI, SSH, WSL and Windows execution remain unperformed.

Raw alternating-run results: `/tmp/as-pf10b.etXnGu/{scan,res}-final.txt` (temporary,
not a durable artifact).

## PI-01 — Separate path enumeration and status (2026-10-25)

Added `files.ListPaths` and `files.ListingStatus`; `files.List` remains the
compatibility entry point. Its composition preserves Git command order and
starts nested-repository status as each repository is discovered, retaining the
existing four-worker bound and post-walk grace/cancellation behavior. Path-only
enumeration starts no status goroutines. Search and content/path security are
unchanged; no cache or wire changes are included.

`TestListingPartsMatchList` compares every Listing field for Git roots and
subdirectories, including ignored paths, deletion status and a >20k truncated
fixture. Path work performs two ls-files and directory check-ignore commands,
with no status/rev-parse; status work adds exactly one status and one rev-parse,
without more enumeration. `TestWalkPartsMatchList` covers non-Git parents with
nested Git status. Error/cancellation and existing bounded nested-status,
wide-directory, ignore and uncapped search regressions pass.

Verification: full backend tests, files race tests, native and Windows cross-vet,
Go build, frontend tests/check/build pass. No latency improvement is claimed for
this structural split; repeated timing/retention evidence belongs to PI-09.
Native GUI, SSH, WSL and Windows execution remain unperformed.

## PI-02 — Bounded coalesced listing flights (2026-10-25)

Service-owned project flights now share overlapping same-cleaned-root ListFiles
requests. Completed results are not cached: a subsequent request reconciles paths
and status again. The compatibility files.List loader retains PI-01's nested
status overlap. Each caller gets cloned slices/maps and its original root spelling.
Authorization runs before admission and after waiting; catalog removal denies a
pending result. No optional snapshot API or retained path cache is enabled yet.

Limits: four active loaders, twenty total owned flights (four active plus sixteen
queued), two-minute flight lifetime. Queue overflow returns an explicit retryable
error. The first caller does not own the shared context; waiters cancel independently,
and the last departure detaches/cancels work. Canceled loaders count toward the
bound until they exit. Engine stop cancels and drains owned flights, and restart
uses a fresh epoch. Superseded epochs retry at most once. Mutation hooks follow
in PI-04/05; PI-02 does not claim mutation invalidation is already wired.

Deterministic channel/condition tests cover one-loader coalescing, leader and last
waiter cancellation, independent result ownership, distinct-root admission and
four-worker bound, queue saturation, shutdown/restart, loader failure, epoch retry,
and denied/revoked authorization. Focused race tests pass across ten repeats.
Full backend tests and engine race suite pass; final engine/app/RPC tests, build,
native/Windows cross-vet and frontend tests/check/build pass. No native execution
or GUI responsiveness claim; repeated workload timing remains PI-09.

## PI-03 — Bounded immutable path snapshots (2026-10-25)

Successful current flights retain path-only snapshots (four roots, 16 MiB estimated
owned data). Status is discarded; strings and slices are owned, public results
cloned. Oversized results bypass admission; truncated snapshots keep their flag.
Errors, cancellation and detached/superseded flights cannot publish. Access touches
LRU order; reset/start/stop drop snapshots. A bounded global epoch avoids root
tombstone growth and eviction ABA. Every authoritative request still reconciles.

Snapshot tests cover ownership, status omission, truncation, entry/byte admission,
LRU touch/eviction, reset/restart, sequential reconciliation and epoch changes.
Existing flight failure/cancellation tests and focused engine race tests pass;
engine/files suites and engine vet pass. No public snapshot route enabled yet;
wire/frontend integration follows PI-06/07. Retained bytes are an estimate, not a
measured allocator bound; heap/timing evidence follows PI-09.

## PI-04 — Mutation generation invalidation (2026-10-25)

File create/directory/add/upload/save/replace/rename/delete routes invalidate
before and after every authorized operation, including failure/uncertainty.
Overlapping parent/child snapshots are evicted; sibling snapshots remain. A global
epoch conservatively supersedes all in-flight work, with one bounded retry.
Existing revision checks and mutation locks remain unchanged. Generated route
fixtures verify authorization refusal has no bump, authorized operations have two
bumps, successful file operations and canceled/error paths, plus a barrier test
proving a pre-create listing cannot publish stale paths. Save/replace retain the
same deferred invalidation on every return. Focused mutation/project race tests
pass. Explicit uncertain-write fault injection remains part of PI-08 integration.

## PI-05 — Git, catalog and lifecycle invalidation (2026-10-25)

Stage/unstage/discard/commit/switch/sync conservatively invalidate every root before
and after authorized operations, including errors. Forget and catalog removal
callbacks discard snapshots; authorization is always rechecked, including optional
snapshot hits. Stop/restart cancels work and clears retention. Existing route and
lifecycle regressions plus focused race tests pass. Successful worktree/Git-sync
integration fixtures remain PI-08; native hosts are not claimed.

## PI-06 — Optional snapshot transport (2026-10-25)

Optional `ProjectSnapshotBackend` leaves Backend/ListFiles compatibility unchanged.
Local App and RPC expose an explicit unverified paths envelope. RPC advertises the
capability only for supporting backends; absent capability/method falls back to
an empty envelope. Status is omitted by the service, authorization checked on
all hits. Engine and Service satisfy the optional interface. RPC tests cover
parity/ownership, denied roots, absent/missing method, cancellation and disconnect;
focused transport/app/engine suites and race checks pass. Wails bindings were
produced by the installed bindings generator (temporary bindings-mode executable,
no UI startup/store opening); no native GUI check was performed.

## PI-07 — Unverified frontend snapshot integration (2026-10-25)

The files store requests the optional backend hint concurrently with mandatory
ListFiles. A hint fills only an empty current same-root listing and remains
unverified. An authoritative reply, error, host/root/epoch change or later list
sequence prevents late hints from replacing state. Refresh errors preserve visible
unverified paths with an error; status arrives from the authoritative request.
Existing frontend LRU, deleted-status merge and draft/editor lifecycles remain.
Tests cover both response orders, errors, host changes and older/missing API
fallback alongside the existing PF-11/12 matrix. Frontend tests/check/build pass.
Native focus/paint/scroll checks remain unperformed.

## PI-08 — External change and concurrency matrix (2026-10-25)

Generated Git and non-Git roots, isolated Git config, no project watcher. Every
authoritative `ListFiles` reconciles, so unknown, overflowed or missed external
events are equivalent by construction: enumeration runs on every request.

| Case | Coverage |
|---|---|
| External create/delete, tracked deletion, `.gitignore` and nested ignore edits | `TestProjectIntegrationExternalReconciliation`: each result equals an uncached `files.List`; the retained snapshot stays at its old generation until the next authoritative request replaces it |
| Status-only change | `TestProjectIntegrationSnapshotStatusSeparation`: paths unchanged, status refreshed, snapshot never carries status |
| Nested repositories, subdirectory caps | 20,001-file Git/non-Git subdirectories return exactly 20,000 paths with `Truncated`, equal to the oracle |
| Concurrency | Twelve same-key clients share one real loader; four independent roots run four loaders with no cross-root results |
| File mutations | Save/Replace success, conflict, cancellation and refusal; create/add/upload/rename/delete routes; a barrier proves a pre-create flight cannot publish stale paths, locally and over in-process RPC (`linux \|\| darwin` Git wrapper barrier) |
| Uncertain writes | `withProjectMutation` (used by Save and Replace) invalidates before and after `ErrSaveIncomplete` and a panic, and drops a listing taken mid-write |
| Git mutations | Successful stage, unstage, commit, discard, push, local pull, fetch and branch create/switch on a linked worktree with a local bare remote; each invalidates every root twice |
| Lifecycle and authorization | Forget and catalog withdrawal deny retained snapshots; real-engine RPC parity of `ProjectSnapshot` with direct calls; stop/restart drop retention |

Review fixes landed with this matrix: superseded shared flights are now
canceled, so they free their loader slot at once. After two superseded shared
attempts a request runs one private flight that mutations do not cancel and
whose result is never retained, so heavy editing elsewhere no longer surfaces
`ErrProjectChanged`. Stop waits at most one second for loaders that ignore
cancellation, and Git path validation checks cancellation every 1,024 entries.
Oversized listings are rejected before being copied under the index lock. The
frontend now renders an accepted hint immediately (spinner cleared, refresh
indicator on) and requests a hint only for an empty view.

An earlier run timed out after 10 minutes in `TestProjectFlightAdmissionAndShutdown`:
its fake loader blocked sending a start notification after stop canceled it.
The fake now selects on cancellation; production loaders were not affected.

Known limits: a request that joins an in-progress flight can return a result
started before a concurrent external edit (bounded by that flight). A listing
that runs between a mutation's two invalidations can show its intermediate
state, as before this milestone. Windows case-variant roots and native
Windows, WSL, SSH and GUI runs are untested.

## PI-09 — Repeated workload and retention evidence (2026-10-25)

Native Darwin/arm64, Apple M5; generated explicit temporary roots only. No app
launch, private-store QA or installs. Fixture construction is outside timing.
Raw evidence is temporary, not checked in; the distributions below preserve the
important outcomes. Benchmarks remain in `project_index_bench_test.go`.

### Shared manager versus its uncached compatibility loader

Prebuilt `engine.test`, six alternating List/Manager rounds, three iterations per
sample, in `/tmp/as-pi0809-evidence.PiJsSM`. Both use current `files.List`; this
comparison isolates sharing/retention, not historical code changes. Measurements
precede the final cancellation/lifecycle review fixes; those do not remove path
or status work. Medians in milliseconds, four-client columns are whole batches:

| Fixture | List | Manager | List same-root ×4 | Manager same-root ×4 | List distinct roots ×4 | Manager distinct roots ×4 |
|---|---:|---:|---:|---:|---:|---:|
| Non-Git 1k | 8.149 | 8.204 | 10.329 | 7.635 | 9.845 | 10.209 |
| Non-Git 20,001 | 22.732 | 23.414 | 32.780 | 23.575 | 35.456 | 36.058 |
| Git 1k | 60.233 | 60.614 | 76.548 | 61.153 | 83.714 | 81.843 |
| Git 20,001 | 155.936 | 158.574 | 282.451 | 157.123 | 287.967 | 302.211 |

Large Git sequential samples (List / Manager, ms):
`171.887,155.835,154.570,155.172,156.037,165.294` /
`155.891,155.680,156.776,171.251,160.372,189.920`.
Same-root four-client samples:
`310.842,277.613,268.752,275.812,289.962,287.289` /
`155.273,156.115,156.467,157.778,163.176,171.807`.

Loader counts are invariant: sequential **1→1**, same-root batch **4→1**,
distinct-root batch **4→4**. This counts real compatibility List calls (each
collects current status), not separately instrumented manager subprocesses.
Existing generated files accounting reports six Git processes per Git listing:
two ls-files, two check-ignore, one rev-parse and one status. No sequential
Git-work reduction is claimed. Large Git same-root batch latency falls about
44%; sequential median overhead is 1.7%, distinct-root overhead 4.9%, and small
changes are noisy. Memory copying is the deliberate price of immutable results.

Median allocation bytes (List→Manager), sequential / same-root ×4 / distinct ×4:

| Fixture | Sequential B/op | Same-root batch B/op | Distinct-root batch B/op |
|---|---:|---:|---:|
| Non-Git 1k | 337,280→388,069 | 1,355,674→440,232 | 1,398,709→1,608,978 |
| Non-Git 20,001 | 6,739,008→7,715,914 | 26,960,173→8,701,466 | 27,919,253→31,831,744 |
| Git 1k | 1,487,109→1,619,218 | 5,941,468→1,916,154 | 6,041,866→6,571,049 |
| Git 20,001 | 25,538,829→27,828,570 | 102,155,877→32,749,728 | 104,076,816→113,237,613 |

### Historical path/status compatibility cost

Separately prebuilt `files.test` from pre-PI-01 `e637a62` and PI-08 code plus the
final seven-line files cancellation fix; `/tmp/as-pi09-historical.MXUejT`.
Six alternating base/current rounds, **one iteration** per sample to bound wide
reference cost. The identical non-Git harness was added only to both temporary
archives. Cold one-shot values are not comparable to the warmed manager rows.
Medians (ms; base→current), with unchanged Git work counts:

| Workload | Median ms | Git/op | Median B/op base→current |
|---|---:|---:|---:|
| PF-03 Git 1k | 65.543→62.601 | 6→6 | 1,455,240→1,456,168 |
| PF-03 Git 20,001 | 212.849→208.366 | 6→6 | 24,901,016→24,901,816 |
| Non-Git 1k | 46.248→47.062 | not instrumented | 311,280→311,272 |
| Non-Git 20,001 | 60.069→59.622 | not instrumented | 6,411,224→6,411,224 |
| Wide PerChunk reference | 6,728.417→6,748.202 | 1,010→1,010 | 26,305,536→26,319,784 |
| Wide Frontier128 reference | 151.116→162.731 | 16→16 | 5,224,168→5,224,408 |
| Wide production Frontier | 56.271→56.738 | 2→2 | 4,960,224→4,960,224 |

Large Git samples base:
`160.841,194.213,162.572,256.594,231.486,232.365`; current:
`213.338,192.837,203.395,291.133,249.372,188.596`.
Wide production base: `59.010,55.953,55.316,56.147,56.396,57.440`;
current: `56.856,56.388,56.621,54.249,57.703,59.275`.
Low samples/high variance and fixed internal subbenchmark order limit attribution.
Frontier128's ~7.7% regression is recorded, not hidden; the production frontier
and subprocess counts are essentially unchanged. No universal speedup claim.

### Reachable retention and limits

Four snapshots reachable after loading five roots, oldest evicted; three heap
samples with `runtime.KeepAlive(manager)` and no retained returned Listing:

| 20,001-file roots | Owned estimate B | Live process heap delta samples B |
|---|---:|---|
| Non-Git | 2,561,640 | 2,597,507 / 2,594,685 / 2,594,717 |
| Git | 2,563,992 | 2,607,595 / 2,607,763 / 2,601,379 |

Heap deltas include GC/process noise and bookkeeping; the 16 MiB limit is an
owned-data estimate, not an allocator/RSS guarantee. Four 1k roots estimate
~133 kB with measured heap ~134–142 kB. Tests separately enforce four entries,
16 MiB admission, oversized bypass, immutable copies, error/cancel non-publication
and zeroed evicted backing slots. Snapshot access performs no filesystem/Git
work; it clones path slices and is explicitly **not an authoritative refresh**.
Four active and sixteen queued owned flights bound admitted work, including
canceled loaders until they return. Individual OS filesystem calls can hang;
shutdown stops waiting after one second, rather than claiming to interrupt them.

Final-tree snapshot-only benchmark (`/tmp/as-m3-final.NKKAxE/snapshot.txt`),
three samples ×100 iterations: non-Git 1k 1.898/1.664/1.525 µs, Git 1k
1.633/1.484/1.674 µs; non-Git 20k 44.072/42.450/39.371 µs, Git 20k
44.978/42.287/41.046 µs. All report zero loader calls and zero status entries;
~16.7 kB at 1k or ~328 kB at 20k per returned path-slice copy. This is local
manager access, not RPC serialization, transport, frontend rendering or freshness.

## PI-10 — Integrated milestone gate (2026-10-25)

**Milestone 3 automated gate complete, PI-01…PI-10.** Final production tree
`423c5cf` passed full backend tests (`-timeout 600s`), engine/files/RPC race tests,
`go vet ./...`, Windows/amd64 CGO-disabled cross-vet and `go build ./...`.
Frontend passed all **759 tests**, check (zero errors; existing tsconfig
ignoreDeprecations warning) and production build (existing large-chunk warning).
Raw gate logs: `/tmp/as-m3-final.NKKAxE/*-final.txt` and `frontend.txt`.
The earlier full race ×3 attempt hit its 300-second package timeout while running
transcript baseline equivalence; a single full final race run with a 600-second
budget passed. Focused final project/mutation race tests passed five repetitions.
No failure is presented as a successful gate.

Independent review identified hidden snapshot rendering, redundant transport,
superseded work holding slots, unbounded shutdown waiting and copying before
size admission. All were repaired with regressions. Continued mutation churn
now uses a bounded private unretained compatibility fallback instead of exposing
ErrProjectChanged; queue saturation still returns explicit ErrProjectBusy.
Uncertain Save/Replace coverage injects ErrSaveIncomplete at their shared mutation
wrapper, not a real failing disk; lower-level files tests own actual write-failure
semantics. Mandatory enumeration proves unknown/overflow/missed-event equivalence
without inventing project watcher coverage. Linked worktree and local-bare-remote
fixtures cover Git success paths; real engine RPC covers parity and in-flight
create. Other routes have engine-level coverage, not a duplicate RPC matrix.

Retained data is bounded by four entries/16 MiB estimated owned data; admitted
flights by four active plus sixteen queued. No new unbounded tombstone, watcher
or Search index was introduced. OS calls can ignore cancellation and remain
owned until returning; shutdown no longer waits indefinitely for them. Paths
remain unverified until reconciliation; status is never retained as current.

**Unperformed, not waived into claims:** native GUI/focus/paint/scroll, SSH, WSL,
Windows execution and case-variant Windows roots. Cross-vet is not native Windows
testing. No installs, application launch/replacement, process killing, Windows
packaging, private-store QA script or push occurred. Watcher acceleration and
uncapped Search index reuse remain deferred by the design. Sequential enumeration
and immutable-copy overhead remain measured bottlenecks. Milestones 4/5 still
need their own design and ≤4h briefs before implementation.

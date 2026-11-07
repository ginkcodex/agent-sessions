# ST-06A — Reproduce large-index startup cost

- **Status:** ✅ Done — complete 2026-10-24; [evidence](../../perf.md#st-06a--large-index-startup-baseline-2026-10-24)
- **Estimate:** ≤4 h
- **Depends on:** ST-05
- **Plan:** [performance-startup.md](../../plan/performance-startup.md)

The user still observes >10 s of empty discovery locally, with a session containing
2,952 messages, while a remote host loads in <3 s. Earlier synthetic fixtures were
too small to test database-size scaling. Do not infer exact device latency from them.

- Add an opt-in generated approximately 300 MB FTS index including a 3,000-message
  transcript. No private database or logs may be read. Measure `Open`, Ping,
  `quick_check(1)`, migrations and `LoadCatalog` separately across repeated opens.
- Use real metadata commits and FTS replacement logic; record actual final size,
  page-cache/WAL conditions and fixture-generation exclusions honestly. No CI timing
  thresholds or machine-dependent duration subtraction.
- Record evidence and select the next safe intervention. Commit independently.

**Done when:** repeatable large-index evidence explains a demonstrable pre-publication
cost; no claim that it fully reproduces the user's device or native UI timing.

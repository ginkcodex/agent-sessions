# M0 — Foundation & Claude Code provider (≈ 42 h) — tasks

- **Plan:** [M0.md](../../plan/M0.md)
- **Progress:** 15/16 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M0-01](M0-01.md) | ✅ Done | Scaffold repo: `git init`, `go mod`, folder layout per SPEC §3, `Makefile` | 2 h | — |
| [M0-02](M0-02.md) | ✅ Done | `internal/model`: `SessionMeta`, `Message`, `Part`, `TokenUsage`, `MessageCounts`… | 2 h | M0-01 |
| [M0-03](M0-03.md) | ✅ Done | `internal/provider`: `Provider` interface, `ScanState`, `SessionRef`, explicit ordered… | 2 h | M0-02 |
| [M0-04](M0-04.md) | ✅ Done | `internal/paths` | 2 h | M0-01 |
| [M0-05](M0-05.md) | ✅ Done | Path utils: `NormalizeDir` | 3 h | M0-01 |
| [M0-06](M0-06.md) | 🟡 Partial | Fixture tooling: `cmd/fixture-sanitize` copies a real session and redacts message… | 3 h | M0-01 |
| [M0-07](M0-07.md) | ✅ Done | Claude: `Detect()` and file discovery | 2 h | M0-03, M0-04 |
| [M0-08](M0-08.md) | ✅ Done | Shared JSONL reader: handles huge lines | 3 h | M0-01 |
| [M0-09](M0-09.md) | ✅ Done | Claude metadata scan: `cwd`, first non-meta user prompt, title priority | 3 h | M0-07, M0-08 |
| [M0-10](M0-10.md) | ✅ Done | Claude message counting per SPEC §6 | 3 h | M0-09 |
| [M0-11](M0-11.md) | ✅ Done | Claude `Load`: user/assistant records → `Message`/`Part` | 4 h | M0-08, M0-02 |
| [M0-12](M0-12.md) | ✅ Done | Claude `Load` extras: `compact_boundary` → compaction part, `isMeta`/`attachment` →… | 3 h | M0-11 |
| [M0-13](M0-13.md) | ✅ Done | Claude subagents: expose `subagents/*.jsonl` as child sessions with `ParentID` | 2 h | M0-09 |
| [M0-14](M0-14.md) | ✅ Done | Claude live detection: read `sessions/<pid>.json` and check the pid via `/proc/<pid>`… | 2 h | M0-07 |
| [M0-15](M0-15.md) | ✅ Done | Claude hardening: fuzz test for the record parser, and a diagnostics count of unknown… | 3 h | M0-12 |
| [M0-16](M0-16.md) | ✅ Done | Headless CLI `agent-sessions-cli`: `scan [--agent] [--json]` | 3 h | M0-10, M0-11 |

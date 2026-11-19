# Antigravity CLI — tasks

- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)
- **Progress:** 8/10 done

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [AGY-01](AGY-01.md) | ✅ Done | Live `agy acp` + storage spike, plan, task docs, index rows | 1 h | None |
| [AGY-02](AGY-02.md) | ✅ Done | `AgentAntigravity` ID, OS roots, binary lookup, cache key, protected edit root, RPC env allowlist | 2 h | AGY-01 |
| [AGY-03](AGY-03.md) | ✅ Done | `internal/provider/antigravity`: `Detect`, SQLite + JSONL discovery & incremental `Scan`, `docs/formats.md` | 4 h | AGY-02 |
| [AGY-04](AGY-04.md) | ✅ Done | `internal/provider/antigravity`: `Load`, `LoadIncremental`, `Blob`, XML prompt cleanup, tool pairing by `step_index` & fixtures | 4 h | AGY-03 |
| [AGY-05](AGY-05.md) | ✅ Done | Provider registration (`provider/all`), file watcher (`watch.go`), terminal commands, group/panel labels, search filter | 3 h | AGY-04 |
| [AGY-06](AGY-06.md) | ✅ Done | Built-in `agy acp` launcher, ID-less live-tail reconciliation & `rawOutput.output` extraction in `chatTail.ts` | 3 h | AGY-05 |
| [AGY-07](AGY-07.md) | ✅ Done | Frontend integration: `AgentID`, `ALL_AGENTS`, theme color, `AgentIcon`, sidebar/list/header badges, `Badge`, empty state, `HostEnvDialog`, mock API | 2.5 h | AGY-05 |
| [AGY-08](AGY-08.md) | 🟡 Partial (desktop, terminal and chat QA not run; Windows ⛔) | End-to-end QA: CLI, desktop transcript, PTY new/resume/link, ACP chat new/resume/permission/cancel, remote bundle | 2 h | AGY-06, AGY-07, AGY-09 |
| [AGY-09](AGY-09.md) | ✅ Done | Background task results, subagent reports, system notices and `CHECKPOINT` compactions | 3 h | AGY-04 |
| [AGY-10](AGY-10.md) | ⬜ Not started | Handoff into Antigravity launches a missing `antigravity` command: support `agy -i` or hide the target | 1 h | AGY-05 |

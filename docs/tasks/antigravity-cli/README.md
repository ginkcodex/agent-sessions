# Antigravity CLI — tasks

- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)
- **Progress:** 19/20 done

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
| [AGY-10](AGY-10.md) | ✅ Done | Handoff into Antigravity: `agy -i <prompt>` with `--add-dir`, adapter and source names, resume command | 1 h | AGY-05 |
| [AGY-11](AGY-11.md) | ✅ Done | Live-tail review fixes: failed prompt vs. identical retry, file-only prompts, bounded `sent`, mock label map | 2 h | AGY-06 |
| [AGY-12](AGY-12.md) | ✅ Done | Provider review fixes: capped report Blob keys, late child links, scan prefix digest, renamed transcripts | 2 h | AGY-03, AGY-09 |
| [AGY-13](AGY-13.md) | ✅ Done | `/mcp` in an Antigravity chat: answer it in the app from `agy mcp list` instead of sending it as a prompt | 3 h | AGY-06 |
| [AGY-14](AGY-14.md) | ✅ Done | Spike: every file and SQLite row a delete must remove, and what agy does afterwards | 2 h | AGY-03 |
| [AGY-15](AGY-15.md) | ✅ Done | Delete Antigravity sessions in `internal/manage`, as the spike decides | 4 h | AGY-14 |
| [AGY-16](AGY-16.md) | ✅ Done | Tool cards: agy's `Path` / `StartLine` / `CommandLine` arguments for the file link and summary; real argument names in fixtures | 2 h | AGY-04 |
| [AGY-17](AGY-17.md) | ✅ Done | Effort picker says "Extra" where agy says `xhigh` | 1 h | AGY-06 |
| [AGY-18](AGY-18.md) | ✅ Done | Share-safe export and import: `antigravity` missing from the bundle's `safeAgent` list | 1 h | AGY-10 |
| [AGY-19](AGY-19.md) | ✅ Done | Complete export: capture agy's native records (list from AGY-14); restore stays unsupported and is said so | 3 h | AGY-14, AGY-18 |
| [AGY-20](AGY-20.md) | ✅ Done | Transcript `file:///` links (agy's link form) open as files | 1 h | AGY-16 |

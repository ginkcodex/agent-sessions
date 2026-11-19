# AGY-01 — Live ACP and storage spike, plan, task docs and index rows

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** None
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

- Probe `agy` CLI flags, version output, on-disk storage layout
  (`~/.gemini/antigravity-cli/`), and a live `agy acp` lifecycle (`initialize`,
  `session/list`, `session/new`, `session/prompt`, `session/resume`,
  `session/set_config_option`, `session/request_permission` allow/deny,
  `session/cancel`, disk flush timing, and stdin-close exit).
- Write `docs/plan/antigravity-cli-spike.md`, `docs/plan/antigravity-cli.md`,
  this task group (`docs/tasks/antigravity-cli/README.md`, `AGY-01.md`..`AGY-08.md`),
  and register the feature in `docs/plan/README.md`, `docs/tasks/README.md`,
  and `TASKS.md`.

## Acceptance criteria

- Every ACP protocol assumption (`protocolVersion`, capabilities, `sessionId`
  parity with disk, `configOptions`, permissions, cancel, live-tail IDs) is
  backed by real measurements, and untested areas are listed explicitly.
- Plan covers goal, verified facts, design, decisions, non-goals, risks, and
  release criteria.
- Every follow-up task is scoped to ≤ 4 h with clear dependencies, likely
  files, acceptance criteria, and tests.
- All three index tables (`docs/plan/README.md`, `docs/tasks/README.md`,
  `TASKS.md`) are updated.

## Evidence

- Verified locally on Linux (WSL2) with `agy` `1.5.12` (`Antigravity ACP Server`
  `1.5.12`):
  - `initialize` returned `protocolVersion: 1` (0.09 s), `loadSession: true`,
    `sessionCapabilities: {resume, close, load, list, additionalDirectories}`,
    `promptCapabilities: {image: true, audio: false, embeddedContext: true}`,
    4 `authMethods`, and made zero `fs/*` or `terminal/*` requests when omitted
    from `clientCapabilities`.
  - `session/list` matched the active CLI session UUID 1:1; `session/new`
    flushed `brain/<id>/.system_generated/logs/transcript_full.jsonl` and
    `conversation_summaries.db` before process exit; `session/resume` (1.10 s)
    continued without history replay in a fresh process.
  - `session/set_config_option` (`thought_level: "low"`),
    `session/request_permission` (both `allow` and `deny`, string request ID
    `"1"` → `stopReason: "end_turn"`), `session/cancel` (`stopReason:
    "cancelled"`), and clean stdin-close shutdown (exit 0 in 0.92 s) all passed.
  - Identified two live-tail requirements for `AGY-06`: streamed chunks omit
    `messageId` (and `transcript_full.jsonl` omits ACP `toolCallId`), and
    `run_command` completed `tool_call_update` carries output in
    `rawOutput.output`.
- Plan, spike record, task group, and index tables written.
- **Second pass (review, 2026-11-19):** read-only checks of all 6 sessions
  in the store and one more live `agy acp` session, recorded in the spike's
  "Second pass" section. Corrected:
  - the first pass's "one `GENERIC` per tool call, positionally" claim: denied
    and cancelled calls write no result step, so results pair by
    `step_index` (`s + 1 + k`, 211/211 matched);
  - `ANTIGRAVITY_CLI_DIR` is our own variable (agy reads none);
  - the Windows paths are unverified.
  Found and planned: background tasks finished by a later `SYSTEM_MESSAGE`,
  subagent reports, `CHECKPOINT` compactions, the `error` field, result
  header lines, the `CANCELING` status, the ignorable `chunks/` mirror, and
  the touchpoints `internal/index/search.go`, `Badge.svelte` and
  `GroupTree.svelte`. That added [AGY-09](AGY-09.md).
  Confirmed again live: no `messageId` or `_meta` on any frame, ACP
  `toolCallId` not saved, `rawOutput.output` for `run_command`,
  `configOptions`; also seen: identical tool `args` in ACP and on disk, and
  `read_file` completing with no output.
- Not checked: Windows and macOS installs, remote hosts, `session/load`
  (not used: chat resume uses `session/resume`), `elicitation/create`,
  image prompts, logged-out flow.

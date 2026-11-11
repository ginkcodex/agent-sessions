# CGA-01 — Link Codex Guardian sessions to their parent

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** None
- **Plan:** [codex-guardian-ancestry.md](../../plan/codex-guardian-ancestry.md)

## Scope

Update Codex rollout parsing/reconciliation and checkpoint version. Add
provider regressions and engine/manage integration checks.

## Acceptance criteria and tests

- Explicit Guardian ancestry survives an absent DB spawn edge and warm scans.
- Old checkpoints rescan; invalid/missing parents do not form DB-backed links.
- Valid edges take precedence; ordinary removed edges remain removed.
- Session listing, group/agent counts and hide-subagent filters agree.
- Parent deletion includes Guardian first using a fake executor only.
- Affected backend suites and frontend session-tree checks pass.

## Evidence

- Codex consumes top-level `parent_thread_id` only for structured subagent
  sources. Guardian ancestry is stored separately in checkpoints and used
  when no spawn edge exists. Existing edges take priority; fallback requires
  a different parent present in the scanned catalog. No title/time/CWD matching.
- Checkpoint version 5 reparses unchanged files from earlier versions. Tests
  cover cold/warm scans with/without SQLite, pre-fix cache migration, missing
  and self parents, conflicting/dangling edges, parent removal and legacy
  nested ancestry. Ordinary edge removal remains authoritative.
- Engine integration scans synthetic Guardian metadata into the real catalog:
  flat/directory/agent grouping nests it under its parent; group totals stay
  at one main session. Agent totals retain existing all-matching-record
  semantics (two normally, one when hiding subagents). Has-subagents filtering
  returns the parent.
- Codex deletion test now scans real-shaped synthetic rollouts with an empty
  spawn-edge table before planning and executing through a recording executor.
  The preview includes both files and CLI calls delete Guardian before parent.
  No real sessions or provider state were deleted.
- Passed `go test ./internal/provider/codex ./internal/engine ./internal/manage
  ./internal/group ./internal/scan` with Go 1.26.10. Initial sandbox run hit
  existing Git/socket restrictions; affected suites passed outside it.
- Passed the new expanded engine grouping/count integration separately after
  adding checks for both directory grouping modes.
- Passed frontend `node --loader ./test/resolver.mjs --test
  test/session-tree.test.mjs`; `git diff --check` clean.
- Not checked: a rebuilt/installed desktop app or actual destructive deletion.

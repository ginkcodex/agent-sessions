# CLF-02 — Verify OpenCode chats in the Live filter

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CLF-01
- **Plan:** [chat-live-filter.md](../../plan/chat-live-filter.md)

## Scope

Exercise the shared Live overlay with OpenCode agent/session identity, ACP
load fallback, fresh-session catalog arrival and lifecycle cleanup. Likely
file: internal/engine/chat_live_test.go. No external terminal detection.

## Acceptance criteria and tests

OpenCode open/idle chats pass Live-only lists/groups/counts and metadata.
Close/crash releases that state and emits invalidation. A new chat is Live
as soon as its provider record arrives. Existing Codex regressions still pass.
Run these Go regressions normally and with the race detector, plus OpenCode
provider tests. Record fake versus real adapter verification accurately.

## Evidence

- Reviewed OpenCode provider v1/v2 scans: catalog references use the stored
  session ID directly. ChatManager claims the ACP sessionId under the same
  opencode agent identifier. No OpenCode-only production change was needed.
- Parameterized shared overlay and engine lifecycle regressions for Codex and
  OpenCode. OpenCode uses ses_live_session and disables fake session/resume to
  exercise session/load, including replay suppression. Verifies idle chats,
  groups/counts/metadata, scan replacement and close/crash invalidations.
- New OpenCode test starts a fresh fake ACP chat, checks it creates no phantom
  catalog entry, then simulates provider discovery and verifies Live appears,
  persists across scans and disappears on close.
- Expanded focused tests pass normally and under Go's race detector.
- Uses fake ACP adapters: no real OpenCode inference, session modification,
  or installed desktop verification is claimed. The shared CLF-01 fix must
  be included in a rebuilt client before manual retesting.
- Engine suite (excluding the previously baseline-reproduced
  TestGitHistoryAuthorizedChild failure) and full OpenCode provider suite pass.
  `git diff --check` is clean.

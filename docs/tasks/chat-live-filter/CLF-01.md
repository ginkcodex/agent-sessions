# CLF-01 — Include open chats in Live filtering

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** None
- **Plan:** [chat-live-filter.md](../../plan/chat-live-filter.md)

## Scope

Service read overlay, engine wiring and ChatManager lifecycle notifications.
Likely files: service.go, engine.go, chat.go and Go regression tests.

## Acceptance criteria and tests

Chat-owned sessions pass Live filtering consistently across groups/counts/list
and metadata, including idle chats. Close/failure releases liveness while
provider-live state survives. Scans do not overwrite the overlay. Lifecycle
notifications reload existing clients. Test using fake adapters/isolated
registries; no real Codex run or installed-desktop verification is claimed.

## Evidence

- Session/group/count reads and GetSessionMeta overlay chat ownership on
  scanner metadata. The catalog stays unchanged, so new scans cannot erase
  chat liveness and releasing ownership restores the scanner's badge.
- Engine-wired ownership notifications invalidate catalog views after chat
  start, close, crash and failed setup, through the existing catalog bus.
- New regressions verify an idle open Codex chat, live-only lists/groups/agent
  counts/metadata, archive/agent/query/path filters, scanner replacement,
  provider-live preservation and lifecycle refresh events. Fake adapters and
  isolated ownership registries are used.
- Targeted Go tests pass normally and with `-race`.
- Frontend session-filters, chat-ui and search test files pass; no frontend
  source changes were needed. `git diff --check` is clean.
- Full engine suite encountered TestGitHistoryAuthorizedChild failing at Git
  worktree discovery, inside and outside the sandbox. The same failure was
  reproduced using a Go overlay of HEAD's engine.go/service.go/chat.go with
  the new test file excluded: it predates this fix.
- The remaining engine suite passes with only that existing Git-history test
  excluded (`go test ./internal/engine -skip ^TestGitHistoryAuthorizedChild$`).
  Scanner and grouping package suites pass.
- Not checked: real Codex or a rebuilt desktop client. New chats still need
  a saved provider record before they can appear in the session catalog.

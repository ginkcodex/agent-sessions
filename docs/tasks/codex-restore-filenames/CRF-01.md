# CRF-01 — Canonical future restore filenames

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** None
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

internal/manage/restore_codex.go and restore tests. Cover normal/copy restore, old exported names and preview stability.

## Acceptance criteria and tests

Meet the corresponding plan release criteria using synthetic data only. Record
what ran, results and unverified real-client behavior in Evidence.

## Evidence

- restore_codex.go now preserves valid native filename timestamps and uses
  the final restored UUID. Old exported restored names derive a stable UTC
  timestamp from session metadata or the bundle creation time. No clock-based
  filename drift between preview and execution.
- Updated normal/copy restore assertions; added timestamp precedence, invalid
  timestamp, no-timestamp and old-export preview-stability regressions.
- `go test ./internal/manage ./internal/provider/codex` passes (Go 1.26.10).
  Manage's existing local HTTP tests ran outside the sandbox.
- Native Codex 0.167.1 on Linux accepted the canonical form even when the
  timestamp's date differs from the enclosing restore-day directory.
- Not run: rebuilt desktop on macOS or a new real user import.

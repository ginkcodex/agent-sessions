# WCI-01 — Remove timing assumptions from Windows CI tests

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** None
- **Plan:** [windows-ci-test-stability.md](../../plan/windows-ci-test-stability.md)

## Scope

Separate repository setup from the Git call-budget measurement, cover expired deadlines independently, and wait for the watcher's initial scans and publication before changing files in live watcher tests.

## Likely files

- `internal/codediagnostics/git_scope_test.go`
- `internal/codediagnostics/project_watch_test.go`
- `internal/codediagnostics/live_watch_test.go`

## Acceptance criteria and tests

- Git call-cap coverage does not depend on completing fixture setup within the production deadline.
- An expired deadline makes no additional Git calls.
- Watcher tests use a channel barrier after startup, not root-watch installation or sleeps.
- Run repeated race-enabled targeted tests, diagnostics package tests and Windows cross-vet.
- Keep production behavior unchanged and report native Windows validation separately.

## Evidence

Implemented on 2026-11-09:

- Confirmed the failed `windows-test-build` in GitHub Actions run 42639054985 stopped at Go tests: `TestGitScopeCallBudget` recorded 12 calls instead of 16, and `TestProjectWatcherRubyOnly` omitted the `lib/b.rb` change.
- Moved all Git fixture initialization before scope creation and gave only the call-cap test a one-minute deadline. Added `TestGitScopeExpiredDeadline` to check that an expired scope neither queries nor skips a nested repository.
- Added a channel barrier to the fake watcher's first `Events()` call. Ruby watcher tests now wait until the initial scans and publication finish, rather than merely waiting for root-watch installation.
- Added `TestProjectWatcherReadyAfterStartupScans`, which blocks startup immediately after installing the root watch, demonstrates that readiness has not fired, and verifies that only the post-startup saved change is reported.
- No production code or CI configuration changed.

Verified on macOS:

- `go test -race -count=5 -run 'TestGitScope(CallBudget|ExpiredDeadline)|TestProjectWatcher(RubyOnly|ReadyAfterStartupScans|LiveApproval)' ./internal/codediagnostics`: pass.
- `go test -race -count=100 -run 'TestProjectWatcher(RubyOnly|ReadyAfterStartupScans|LiveApproval)$' ./internal/codediagnostics`: pass.
- `go test -race ./internal/codediagnostics`: pass (73.644 s).
- `go vet ./internal/codediagnostics`: pass.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...`: pass; cross-vet only, not native Windows execution.
- `git diff --check`: clean.

Not checked:

- Native Windows tests/build or a new GitHub Actions run. Changes are local and uncommitted; the next Windows CI run must confirm the platform result.
- Full repository test suite or frontend tests; only diagnostics test code changed.


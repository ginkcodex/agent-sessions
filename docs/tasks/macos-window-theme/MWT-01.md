# MWT-01 — Sync the native window appearance with the app theme

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** —
- **Plan:** [macos-window-theme.md](../../plan/macos-window-theme.md)

## Scope

Send saved and selected theme modes through the existing window-local router. Validate the mode and invoke a Desktop callback. Set macOS NSWindow appearance on the main thread; clear it for System. Preserve the native frame and controls. Other desktop platforms do nothing. Serialize theme requests and handle rejection without breaking the web UI.

## Likely files

- `frontend/src/lib/stores/theme.svelte.ts`, `api.ts`, `appBinding.ts`
- `internal/app/desktop.go`, `window_theme.go`
- `cmd/agent-sessions/desktop_v3.go`, `windowtheme_darwin.go`, `windowtheme_darwin.m`, `windowtheme_other.go`
- Frontend theme tests and Go window-theme tests

## Acceptance criteria and tests

- Saved light/dark/system modes apply at initialization and explicit changes update the native window.
- System media changes update only the DOM; explicit native appearance cannot trigger a feedback loop.
- Rapid requests retain order; rejection is reported and later requests still work.
- Browsers do not load the native runtime for themes.
- Invalid Go inputs never reach the callback; valid modes dispatch to this window only; missing callbacks do nothing.
- Run targeted frontend/Go tests, frontend check/build, Go suite and lint. Record limitations rather than treating planned macOS QA as passed.

## Evidence

2026-11-16:

- Added `SetWindowTheme` to the per-window App/Desktop bridge and explicit frontend binding contract. Go validates the three modes before native dispatch and propagates native failures; a missing callback is harmless.
- Added a macOS main-thread NSWindow appearance setter (Aqua / Dark Aqua / nil). Other platforms are no-ops. No native frame, geometry, close guard or control changes.
- Theme initialization and explicit changes enqueue native requests in order. Media-query changes update only the web theme, avoiding native feedback. Native failures are logged and do not block later updates.
- `go test ./internal/app -run 'TestSetWindowTheme|Test.*Binding|Test.*Router' -count=1`: passed, covering mode validation, callback failures, window-local dispatch, missing desktop and binding/router contracts.
- Targeted frontend theme/runtime tests: 20/20 passed. Covers saved/default modes, explicit toggles, system transitions, simulated native media changes, request ordering, failure recovery, browser isolation and listener cleanup.
- `make test` (`go test -race ./...`): passed across the repository.
- Full frontend suite: 1,511 passed, 1 skipped, 0 failed (1,512 total).
- `npm run check`: 0 errors, 0 warnings. `npm run build`: passed.
- Linux production desktop build and Windows production desktop cross-build: passed; Linux emitted Wails GTK/X11 deprecation warnings.
- Repository `make lint`: failed with 155 findings in unchanged code (including unchecked closes in `internal/files` and unused platform-specific update helpers). `go tool golangci-lint run --new-from-rev=HEAD`: passed with 0 issues. Unrelated cleanup was not attempted.
- macOS build and visual checks did not run; see MWT-02. Exact CSS color matching and startup flash were not evaluated.


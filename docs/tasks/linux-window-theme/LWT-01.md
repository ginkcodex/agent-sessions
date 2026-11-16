# LWT-01 — Apply the app theme to Linux native chrome

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** —
- **Plan:** [linux-window-theme.md](../../plan/linux-window-theme.md)

## Scope and likely files

Implement a GTK 4 Linux `setWindowTheme` with main-thread dispatch and safe native handle handling. Narrow the other-platform stub to avoid duplicate symbols. Reuse all frontend/router behavior. Add a small native helper test covering nontrivial appearance/reset logic.

Likely files: `cmd/agent-sessions/windowtheme_linux.go`, `windowtheme_linux.c`, `windowtheme_other.go`, native test fixture/Go driver, and this plan's docs. Change callers only if required by GTK's actual scope.

## Acceptance criteria and tests

- Light/Dark request the matching GTK appearance; System restores automatic desktop behavior.
- Preserve the user's GTK theme name and native frame/controls; do not write desktop configuration.
- Preserve macOS behavior and keep Windows a no-op; document GTK scope and compositor limits.
- Native helper regression tests cover explicit modes, reset, repeated changes and theme preservation.
- Run Go tests, theme/runtime frontend tests, production Linux/Windows compilation and changed-code lint. Record actual commands/results and any unverified GTK versions.

## Evidence

2026-11-16:

- Added Linux main-thread `SetWindowTheme` implementation using GTK's `gtk-application-prefer-dark-theme` compatibility setting. System resets the property instead of forcing false or restoring a stale startup value. The configured theme name and native decorations are not changed.
- Kept the existing validated router/frontend and macOS implementation; narrowed the other-platform stub so Windows remains a no-op. Documented GTK process/display-wide, last-request-wins behavior in the Desktop contract and plan.
- Native fixture compiles the actual production C helper with `-Wall -Wextra -Werror` and uses real GTK windows. `GDK_BACKEND=broadway BROADWAY_DISPLAY=:79 GTK_A11Y=none go test -race -tags desktop ./cmd/agent-sessions -run '^TestNativeLinuxWindowTheme$' -v -count=1`: passed on GTK 4.22.4. Verified Dark/Light native widget colors with GTK's bundled Default theme, repeated toggles, System reset, unchanged custom theme selection, shared display scope, null handling and decorated windows. The test intentionally skips with exit 77 if no GTK display exists; it did not skip in this run.
- An intermediate color assertion failed on an unrealized widget; realizing the test window fixed the fixture. The final native test passed. This was a fixture setup issue, not a changed production helper.
- `make test` (`go test -race ./...`): passed. Existing targeted theme/binding Go tests passed; full frontend suite passed (1,511 passed, 1 skipped, 0 failed). No frontend source was changed.
- Linux production desktop build and Windows production cross-build: passed. Wails emitted existing GTK/X11 deprecation warnings.
- Changed-code Go lint: passed, 0 issues. Desktop-tagged changed-code lint initially found a gofumpt issue in the test driver; formatting was corrected and the rerun passed, 0 issues.
- Native visual X11/Wayland testing, live system-setting notifications, actual WebKit media-query behavior, macOS regression run and GTK 4.14 runtime testing were not performed. See LWT-02. A dark-only selected theme may remain dark in Light mode; GTK may fall back when the selected theme lacks a dark variant. No system settings were written.


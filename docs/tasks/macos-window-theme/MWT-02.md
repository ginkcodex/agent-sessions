# MWT-02 — Verify native appearance on macOS

- **Status:** 🟡 Partial — user confirmed theme matching works on macOS; detailed regression checks below are not individually confirmed
- **Estimate:** 1 h
- **Depends on:** MWT-01
- **Plan:** [macos-window-theme.md](../../plan/macos-window-theme.md)

## Scope and likely files

Build the desktop client on macOS and record actual native results here. No extra feature work.

## Acceptance criteria and tests

- `CGO_ENABLED=1 go build -tags desktop,production ./cmd/agent-sessions` succeeds on macOS.
- With macOS dark, app Light has a light native title bar; with macOS light, app Dark has a dark native title bar.
- System follows OS changes live; returning from either explicit mode to System clears the override and updates web content correctly.
- Saved mode works after restart and when a second window opens.
- Rapid theme toggles, fullscreen/restore, minimize/maximize/close and unsaved-file close protection work unchanged.
- Native controls and attached dialogs remain usable; title-bar colors need not exactly equal CSS surface colors.

## Evidence

2026-11-16 (implementation): No macOS build or native visual checks ran in the Linux/WSL implementation environment. Linux desktop compilation and Windows cross-compilation validated only the shared bridge and other-platform no-op, not the Objective-C implementation.

2026-11-16 (user validation): The user tested on macOS and reported: “it works well.” This confirms the native title-bar theme matching works in their test. The macOS version, build command and individual scenarios were not specified. Live OS changes/return to System, restart/second-window persistence, rapid toggles, fullscreen/restore, native controls/dialogs and unsaved-file close protection remain unconfirmed individually; this report is not recorded as a pass for that full matrix.


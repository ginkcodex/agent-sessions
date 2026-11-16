# macOS native window theme

## Goal

Keep the native macOS title bar and controls, but make its light/dark appearance follow the app's existing theme selection instead of always following macOS.

## Design

- The theme store sends its saved mode on initialization and each explicit mode change through the existing per-window App router.
- Add `SetWindowTheme(mode)` and a Desktop callback. Validate `system`, `light`, and `dark` at the Go boundary; missing desktop callbacks are harmless.
- On macOS, set the calling NSWindow's appearance on the AppKit main thread: Aqua for light, Dark Aqua for dark, and nil for system (restore inheritance). Resolve the native handle on that thread to tolerate teardown.
- Serialize frontend theme calls so rapid toggles cannot leave an older native appearance applied last. Report failures in the console without blocking the web theme.
- System-mode media-query changes continue updating the web UI. They do not send native updates: an explicit window appearance can itself change `prefers-color-scheme`.

## Decisions and scope

Reuse the existing theme persistence, router, Desktop injection, and Objective-C integration pattern. Keep the native frame, traffic lights, geometry and guarded close behavior untouched. Windows/Linux callbacks are no-ops. No dependency, new preference, exact color tint, cross-window preference propagation, or About-window redesign.

## Risks

NSWindow appearance affects native child UI and the webview's effective color scheme. Returning to system must clear the override, not copy the last resolved light/dark value. Standard macOS materials can still differ slightly from app CSS colors. A startup title-bar flash before the frontend sends its saved mode is possible. Native compilation and visual checks require macOS and cannot be established by Linux tests.

## Release criteria

- Automated frontend tests exercise saved mode, toggles, system changes, call ordering, browser isolation and failure handling.
- Go tests cover validation, callback dispatch, missing desktop and binding parity; frontend check/build and Go tests pass.
- On macOS, build and verify Light against a dark OS, Dark against a light OS, live System changes, returning to System, restart persistence, a second window, fullscreen/restore and native controls. Record this separately from automated evidence.

## Tasks

See [macOS native window theme tasks](../tasks/macos-window-theme/README.md).

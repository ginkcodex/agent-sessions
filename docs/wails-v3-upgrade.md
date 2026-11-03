# Wails v3 upgrade

The desktop app moved from Wails v2 to Wails v3 in `287fdb9`
("feat: migrate desktop to Wails v3 and GTK4"), followed by macOS fixes for
the app icon and the quit action. This page describes the current state and
lists what has and has not been verified. The earlier plans in
[docs/plan](plan/README.md) and the M1 rows in [TASKS.md](../TASKS.md) still
describe the v2 scaffold; they are kept as history.

## Current state

| Area | Now |
|---|---|
| Go module | `github.com/wailsapp/wails/v3` **v3.1.0** (`go.mod`) |
| Frontend runtime | `@wailsio/runtime` **3.1.0**, pinned exactly in `frontend/package.json` |
| Desktop build | Plain `go build -tags desktop,production` (`make app`, `make gui-build`). No Wails CLI is needed to compile. |
| Dev build | `make dev` drops `production` and points the app at Vite through `FRONTEND_DEVSERVER_URL`. |
| Wails CLI | Only `wails3 generate icons` and `wails3 generate syso` during packaging, run with `go run` at the version `go.mod` pins (`WAILS3=` overrides). |
| `wails.json` | Holds only the app info; `info.productVersion` is the single release version. It is no longer a Wails project file. |
| Linux | GTK 4 and WebKitGTK 6.0 through cgo. `make check-gui-linkage` fails if the binary links GTK 3 or WebKit2GTK. |
| Linux packages | `.deb` depends on `libgtk-4-1` and `libwebkitgtk-6.0-4`; `.rpm` on `gtk4` and `webkitgtk6.0`. |
| Supported Linux | Ubuntu 24.04+, Debian 13+, Fedora 40+. **Ubuntu 22.04 and Debian 12 are dropped**: they do not ship WebKitGTK 6.0. CI and release builds use Ubuntu 24.04. |
| macOS | cgo with Cocoa/WebKit. `make package-macos` builds a universal, ad-hoc signed `.dmg` from `packaging/macos/Info.plist.in`. |
| Windows | Cross-compiled on Linux without cgo (`make windows-app`, `make package-windows`). |

### Code boundaries

- **Go:** only `cmd/agent-sessions` imports Wails (`run_gui.go`,
  `shell.go`, `desktop_v3.go`, `mousenav_native.go`). `internal/app` defines
  an `app.Desktop` struct of callbacks (events, dialogs, window state,
  notifications, close requests); `desktop_v3.go` fills it from the Wails
  application and one window. The CLI and the default build never link Wails
  or GTK.
- **Bindings:** the only bound service is `app.Router`, whose one method
  `Call(method, args)` runs an `App` method on the App of the calling window
  (Wails passes the window in the call's context). The frontend calls it
  through `Call.ByName` (`callApp` in `frontend/src/lib/desktopRuntime.ts`,
  `frontend/src/lib/appBinding.ts`); a Go test keeps `APP_METHODS` in step
  with the exported methods of `internal/app.App`. No generated bindings are
  committed or imported.
- **Runtime:** `frontend/src/lib/desktopRuntime.ts` is the only module that
  imports `@wailsio/runtime`, and loads it with a dynamic import only when the
  page runs in the desktop app. The Go asset server marks that case by
  injecting `window.__AGENT_SESSIONS_DESKTOP__`; the browser demo and Node
  tests never load the runtime.
- **Events:** each App emits to its own window only
  (`WebviewWindow.DispatchWailsEvent`). Dropped files reach only Go in v3, so
  the shell re-emits them to the page as `files:dropped`.
- **Window:** the app runs one process with any number of windows. Each
  window has its own `app.App` in `app.Windows`, created and torn down with
  the window, and New Window opens another one in the same process. The
  close guard runs on the window's closing hook. A quit or restart
  first asks every window about its unsaved files without closing any
  (`internal/app/quit.go`); the app quits once all agree, and a restart
  starts the new copy only after every App has closed.
- **Linux native code:** the mouse side-button hook (`mousenav_linux.c`) uses
  a capture-phase `GtkGestureClick` on each window's `WebKitWebView`, and font hinting is
  turned off through GTK 4 settings (`fonts_linux.go`).

## Verification

### Verified by the user (macOS desktop build)

- Fullscreen: entering and leaving it from the app.
- Common terminal actions in the terminal panel.
- Common window actions.

These were checked by hand on macOS. They are not covered by automated tests
beyond the frontend and `internal/app` unit tests (for example
`frontend/test/fullscreen.test.mjs`, `internal/app/fullscreen_test.go`).

### Automated checks

CI (`.github/workflows/ci.yml`) compiles the macOS desktop binary, builds the
Linux desktop binary on Ubuntu 24.04 and runs `make check-gui-linkage`, and
cross-builds the Windows exe with its resources. These prove the builds link;
they do not exercise the running app.

The Release workflow (`.github/workflows/release.yml`) inspects each package
it builds with `packaging/**/verify-*.sh`: the universal macOS app, its
Info.plist, signature and `.dmg`; the `.deb`/`.rpm` metadata, dependencies
and payload, plus a `.deb` install in a clean Ubuntu 24.04 container; the
`.msi` and setup `.exe` contents (never installed or run); and the server
bundles. A manual run on a branch uploads the whole set with an aggregate
`SHA256SUMS` and a `provenance.json` naming the commit and run, and publishes
nothing. Run 41617377140 on `482a6e6` passed all of these.

During the port, the GTK 4 build was also launched under WSLg on Ubuntu and
showed the local session catalog (checked through the accessibility tree, not
by hand). That shows the app starts and the bridge works; it is not a test
of the native features listed below.

### Native manual gates not yet run

Nothing below has been tested by hand on the v3 build. Treat each as open
until someone records a result here.

- **Linux (GTK 4 / WebKitGTK 6.0), real desktop session:** window shows
  content (including the `WEBKIT_DISABLE_DMABUF_RENDERER` fallback and the
  on-demand GPU policy), mouse side buttons, file drop onto the terminal,
  native open/save dialogs, notifications, printing, font rendering at
  fractional scaling, fullscreen (F11), close guard and quit.
- **Linux packages:** install and launch the `.deb` and `.rpm` on clean
  Ubuntu 24.04, Debian 13 and Fedora 40 systems (amd64 and arm64).
- **Windows (WebView2), real machine:** startup, F11 fullscreen including
  restoring a maximised window, mouse side buttons, file drop, dialogs,
  notifications, the ConPTY terminal, close guard and quit, and the `.msi` and
  setup `.exe` installers.
- **macOS, beyond the user-verified actions:** mouse side buttons and Logi
  swipes, file drop, dialogs, notifications and Apple Events permissions
  granted to a v2 install (the bundle identifier is unchanged), printing,
  window cycling, Open in terminal, Cmd+Q with unsaved files (cancel and
  discard), and the packaged universal `.dmg` on both Apple silicon and Intel.
- **Clipboard, every platform:** copy and paste now go through the v3
  runtime's native clipboard. Under v2 they did; the first v3 build silently
  fell back to the web view's clipboard, which can refuse access. Check copy
  buttons and terminal copy/paste again on each platform.
- **Remote hosts:** WSL and SSH deployment from a packaged build, which loads
  the server bundles from the locations in `internal/remote/locate.go`.

# Idle rendering — QA record

Date: 2026-11-11. Plan: [idle-rendering.md](idle-rendering.md).

## Baseline observation (installed macOS v1.4.0)

- Device GPU utilization reached about 68%. An approximately 8.5 s Metal System Trace attributed 5.519 s fragment execution to WindowServer, with about 120 compositor frames/s. This establishes compositor activity, not a specific CSS root cause.
- Hide/show eventually reset the high-load state. The hide transition itself remained busy for much of its capture; the restored capture was about 1–8% busy. This is not a controlled before/after comparison of the source changes.
- Local temporary captures: `/tmp/agent-sessions-gpu-audit.F557tz/` (`metal.trace`, `hidden.trace`, `visible-restored.trace`). They are not checked in and may be removed by the OS.

## Implementation and automated checks

- Live presence dots/badges remain, without perpetual decorative liveness animations.
- The forced session-list compositing layer is now Linux-only. The existing Linux platform marker is set by `frontend/src/main.ts`.
- Shared Icon spinners animate only after IntersectionObserver reports intersection; leaving the viewport removes the animation. Ordinary icons create no observer. Class changes, queued callbacks after disconnect, missing-observer fallback and teardown are tested.
- IR-02 initially added a document-visibility tracker, a global hidden/stale animation pause and removed the reduced-motion spinner exception. IR-04 reverted those for their side effects: entrance animations frozen at opacity 0, and loading pulses frozen while connecting. Visible spinners now turn slowly under reduced motion again. See [IR-04](../tasks/idle-rendering/IR-04.md).
- The results below are for the IR-02 source. After IR-04: frontend suite **1,221 passed, 1 skipped, 0 failed**, `svelte-check` **0 errors, 0 warnings**, production build **passed**, `git diff --check` **passed**. `make test` and the native compile were not rerun after IR-04, which only touched frontend files.
- `npm --prefix frontend test`: **1,221 passed, 1 skipped, 0 failed** (1,222 total), including the mounted real-Icon action/update test.
- `npm --prefix frontend run check`: **604 files, 0 errors, 0 warnings**.
- `npm --prefix frontend run build`: **passed**. Vite reports chunks over 500 kB after minification; no chunk-size change was attempted in this rendering task.
- `make test` (`go test -race ./...`): **passed**, 42 package results and no failing package; some results were cached. Docker SSH integration and native Windows/Linux execution were not enabled by this command.
- Final source after import/whitespace cleanup: focused rendering tests **7 passed**, frontend check **0 errors, 0 warnings**, production frontend build **passed**, and `git diff --check` **passed**.
- `make lint`: **failed**, reporting 136 issues in unchanged Go files (50 errcheck, 10 errorlint, 8 gocritic, 3 gofumpt, 1 ineffassign, 6 nilerr, 17 staticcheck, 17 unparam, 24 unused). Example: `internal/bundle/read.go:30`, unchecked deferred `f.Close()`. These files were not modified by this frontend patch; the lint backlog was not addressed. Local output: `/tmp/as-idle-rendering-lint.log`.
- Native macOS `go build -tags 'desktop,production'` with version 1.4.0: **passed**. Output: `/tmp/as-idle-rendering-build.Od2Eb5/agent-sessions`. Linker warnings include duplicate `-lobjc` and build-SDK object versions newer than the macOS 11.0 link target. This is build coverage, not a launch, minimum-OS compatibility check or GPU measurement.

## User-reported native result (macOS, IR-01 + IR-02 build)

On 2026-11-11 the user built and ran the IR-02 source on macOS and reported that GPU use "went to 70% but reduced to around 20–30% after a while", a partial improvement. This report came before IR-04. No trace was captured, and the workload and window state are not recorded. The remaining load is tracked in [IR-05](../tasks/idle-rendering/IR-05.md).

## Active coding-agent terminal investigation (IR-05)

2026-11-11: The user reported about 70% GPU again with an active coding-agent terminal. An approximately 6 s capture of the existing app (PID 60531, WebContent 60536) showed:

- Device utilization about **67–77%** during sampling.
- WindowServer fragment intervals totalling about **4.456 s**, roughly **71–76%** in complete one-second buckets.
- **116 WebKit GPU intervals**, about **19/s** and roughly **4%** GPU time. This is not a direct count of xterm repaints.
- Display surface-swap buckets reaching **120/s**.
- A 4 s WebContent sample mostly idle: about **148 of 2,860** main-thread samples in rendering updates.

The mismatch points toward compositor-driven motion, with the AttentionDot working opacity pulse and shared Icon transform spinner as candidates. Neither was isolated by a controlled A/B; terminal rendering and other animations may contribute. Local temporary artifacts: `/tmp/as-terminal-gpu.jQn4HD/` (trace, XML exports and sample). Startup attribution remains open in [IR-05](../tasks/idle-rendering/IR-05.md).

## Low-rate work indicators (IR-06)

Replaced those two CSS loops with visibility-gated, shared interval clocks, with no interpolation: **8 rotation steps/s**, and a working-dot opacity toggle every **800 ms**. Reduced-motion spinners use four steps/s; dots stay steady. Rotation is centered on the SVG view-box. Offscreen/state changes and unmount stop subscriptions; the last subscriber stops its clock. Labels and status semantics remain. No terminal code, cursor policy, output cadence or transport changed.

- Full frontend suite: **1,224 passed, 1 skipped, 0 failed** (1,225 total).
- Final focused rendering tests: **10 passed**. Includes real mounted Icon/AttentionDot updates and cleanup, shared-clock mock-timer tests, reduced motion and CSS policy checks.
- Frontend check: **604 files, 0 errors, 0 warnings**. Initial action-added selector warning fixed before final checks.
- Production frontend build: **passed**, with the existing >500 kB chunk warning.
- `git diff --check`: **passed**.
- Go tests and native desktop compile were not rerun for this frontend-only step. Existing Go lint failure above remains unresolved.

## User-reported native result after IR-06

2026-11-11: After testing the discrete-step changes, the user reported average GPU usage of about **30%**, a further partial improvement. After quitting Agent Sessions and playing YouTube in Chrome, they observed about **0–15%** GPU usage. These are user observations, not controlled trace measurements; video resolution/codec, display state and terminal workload were not recorded. Hardware video decoding can run on a dedicated media engine, so video complexity is not directly comparable to general GPU utilization.

A read-only process check at the time of this follow-up found no running Agent Sessions/WebContent process, so the residual 30% state was not profiled. Source inspection confirms xterm uses its DOM renderer (no WebGL addon): dirty rows have their child spans replaced on render, while xterm already debounces refreshes and supports synchronized output. Agent-TUI repaints, remaining CSS animations, and WKWebView/window compositing are candidates, not attributed causes of the residual load. The earlier trace does not establish which dominates after IR-06.

## Scrolling with the terminal closed (IR-07)

2026-11-11: The user reopened the app and reported **50–60% GPU** when scrolling the workspace list, session list or older transcript, with the terminal closed. This establishes that a visible terminal is not required for the reported symptom.

Read-only, system-wide captures from app PID 63471/WebContent 63476 are in `/tmp/as-scroll-gpu.1LOQQ5/` (temporary/local only):

- `scroll-coordinated.trace`: seconds 9–10 have **111 and 120 display surface swaps**, with summed WindowServer GPU intervals about **65%** in each second. WebKit GPU intervals remain low.
- `scroll-then-rest.trace`: about **9.643 s WindowServer fragment intervals** over approximately 30 s; one-second all-channel sums reach about **69%**. Several surface-swap buckets reach **118–119/s**. WebKit fragment+vertex intervals total **0.237 s**.
- These interval sums are not the device utilization counter and channels can overlap; they are attribution evidence, not interchangeable utilization percentages. Chrome and Ghostty also contributed GPU work. WindowServer is system-wide and does not identify one app's share by itself.
- Despite filenames, **no scroll/rest schedule was communicated or confirmed** and no automation drove scrolling. A burst cannot be assigned to a pane/gesture or described as a controlled scrolling-vs-idle comparison.
- Separate WebContent stack sampling was mostly waiting (3,452/3,550 main-thread samples); it was not aligned to the bursts and does not rule out expensive individual scroll events.
- Display: Apple M5, built-in 120 Hz, reported scaled raster 3600×2338/logical 1800×1169, physical panel 3024×1964. Installed app is universal and the running process sample is **ARM64**, not Rosetta.

### Native opacity hypothesis

Wails v3.1.0 `pkg/application/webview_window_darwin.m:76–85` initializes NSWindow with `clearColor` and `setOpaque:NO`. The app's `cmd/agent-sessions/shell.go` supplies no backing color; the later Darwin color setter changes background color only, and Normal backdrop does not restore opacity (`webview_window_darwin.go:1666–1678,1780–1783,562–565`). The default native backing color therefore remains alpha zero. The ordinary WKWebView path does **not** explicitly make WebKit transparent; do not conflate the two properties.

The initial investigation proposed a public AppKit `setOpaque:` call with an opaque backing color, not a verified optimization. The user subsequently approved implementing the experiment. No dependency-cache sources were modified. Tracked in [IR-07](../tasks/idle-rendering/IR-07.md).

### Implemented opacity experiment

Main-window runtime-ready hook now queues onto Wails' UI thread, checks the native handle, sets adaptive `NSColor.windowBackgroundColor` and `opaque = YES`. No About-window, WKWebView/child-layer, window-alpha, shadow, titlebar or display-setting changes. Windows/Linux hook is a no-op. Unset `AGENT_SESSIONS_MACOS_OPAQUE` or `1` enables it; `0` skips it, preserving the original Wails native configuration.

- Desktop package tests and desktop `go vet`: **passed**. Actual Objective-C helper tested on an unshown AppKit window for nil handle, opaque flag, opaque adaptive backing in light/dark appearance, repeat calls and unchanged child views/style/shadows/alpha. An initial ARC-cast fixture compile failure was fixed before the passing run.
- Isolated macOS ARM64 production executable compile: **passed**, `/tmp/as-opacity-build.4HuxJq/agent-sessions`, version 1.4.0 with uncommitted changes. Final build uses packaging's macOS 12 flags; duplicate `-lobjc` warning remains. It is not a complete app bundle.
- `make test` (`go test -race ./...`): **passed**, some cached results; desktop-tagged tests ran separately as above. Docker SSH integration was not enabled.
- Windows desktop test cross-compile: **passed**, not native testing. Linux native checks not run. Frontend suite was not rerun for this native-only change.
- Desktop `go vet`, gofumpt formatting check and `git diff --check`: **passed**. Existing full Go lint backlog from the earlier validation remains unresolved.
- During implementation, no modified full app was launched by the assistant, no installed bundle replaced, and no controlled GPU A/B or visual window-behavior QA completed. Native property tests do not prove GPU savings.

### User follow-up after opacity change — 2026-11-11

The user tested the change and reported: “now I can see it goes down to 0% when idle when this app still opening.” This is a positive native macOS idle observation after the earlier ~30% average report, not a controlled same-workload benchmark or an assistant-run post-change trace. Post-opacity scrolling/active-terminal GPU behavior, full light/dark/resize/fullscreen/file-drop visual QA and native Windows/Linux behavior remain unverified. IR-07 stays partial. Findings and maintenance cautions are also recorded in [TECH-NOTES.md](../../TECH-NOTES.md).

For a user-run comparison after rebuilding the app bundle: fully quit between runs (preserve any needed terminal work first). Launch its `Contents/MacOS/agent-sessions` executable with `AGENT_SESSIONS_MACOS_OPAQUE=0` for the old behavior, then `AGENT_SESSIONS_MACOS_OPAQUE=1` for the experiment. Use the same window size, pane, content, scroll pattern and background-app state; compare both idle and scrolling, repeat in reverse order, and check light/dark mode, resize/fullscreen and terminal file drop. Do not use `open -a` to reuse an already-running process, which would invalidate the environment comparison.

## Native validation gates still open

Before the opacity change, the user reported about 30% average GPU use plus 50–60% scrolling spikes after IR-06. After the opacity change, they report 0% when idle with the app open. System-wide observations exist above, but no controlled same-workload trace or opacity A/B exists. The active-terminal trace predates the discrete-step implementation. Isolation of blank shell, active agent TUI, terminal-hidden states and individual scroll panes remains outstanding. No controlled percentage reduction is claimed.

For each native platform, compare the same display configuration, window size, session selection, terminal state and workload before/after. Repeat because the original high-load state reset after hide/show. Check:

1. Startup and an idle open session: Live remains visible and idle compositor activity does not stay elevated solely for liveness decoration.
2. Active work and scrolled-out pending tool calls: visible spinners turn in centered discrete steps and working dots blink; offscreen indicators do not move; status text persists and returning to the viewport restores motion. Check the changed motion is legible, including icon-only progress buttons.
3. Hidden/minimized and restored window, stale panes, and reduced motion: dialogs and menus appear normally after restore. Loading/connecting pulses keep moving while connecting. With reduced motion, visible spinners turn slowly and working dots stay steady.
4. Terminal focus/input/output and active streaming remain functional; the terminal cursor policy is unchanged.
5. Linux scrollbar stability near the top and during row hover/scroll, including fractional scaling, with the retained compositing workaround.

| Platform | Automated/build coverage here | Native rendering/GPU result for modified app |
|---|---|---|
| macOS / WKWebView | Frontend suite, type check, production frontend and native desktop compile | IR-02 user report: peak ~70%, settling at 20–30%; after IR-06: ~30% average; after IR-07 opacity: 0% idle reported. Active-terminal trace predates IR-06; no controlled post-opacity trace or scrolling result |
| Windows / WebView2 | Shared frontend tests on macOS only | Not run |
| Linux / WebKitGTK | Shared frontend tests and Linux selector regression on macOS only | Not run; native scrollbar QA outstanding |

Shared perpetual animation costs can affect all three platforms, but the observed macOS ~70% spike is not established on Windows or Linux. The earlier [Linux rendering investigation](linux-gpu-rendering.md) independently documented animation-driven CPU load on a software-composited path; it is historical evidence, not validation of this change.

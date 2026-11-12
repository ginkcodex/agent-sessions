# Idle rendering and GPU usage

## Goal

Reduce unnecessary idle animation and compositing across desktop platforms, and investigate the macOS GPU spike without claiming an unmeasured root cause.

## Evidence and design

On 2026-11-11 the installed macOS v1.4.0 app coincided with 68% device GPU utilization. An approximately 8.5 s Metal trace attributed 5.519 s fragment execution to WindowServer at about 120 compositor frames/s. Hide/show eventually reset the high-usage state; the transition trace remained busy, so this is not a clean causal A/B. Captures: `/tmp/agent-sessions-gpu-audit.F557tz/` (temporary, local only).

- Liveness means an open process, not work: retain static Live badges/dots; remove perpetual decorative liveness animations.
- Keep the verified Linux scrollbar compositing workaround on Linux only. Do not disable hardware acceleration.
- Run shared icon spinners and working attention dots only while intersecting the viewport. IR-06 replaces their perpetual CSS transform/opacity animations with shared discrete timers: 8 rotation steps/s and a dot toggle every 800 ms, without interpolation. Under reduced motion, visible spinners turn at half speed and working dots remain steady. Preserve labels/status and continue receiving backend/terminal output.
- The active-terminal trace in IR-05 shows heavy WindowServer compositing while WebContent is mostly idle; this implicates compositor-driven motion, but is not a controlled isolation of the dot or spinner. Validate IR-06 on the same native workload before claiming GPU savings.
- Do not pause every animation globally for hidden documents or stale panes. IR-02 tried this; IR-04 removed it. Entrance animations start at opacity 0, and stale panes show connecting/loading pulses, so a paused clock can leave UI invisible or frozen. The web views already throttle hidden pages.
- Leave terminal cursor behavior, pending-status semantics, catalog coalescing and port-forward scheduling unchanged in this focused rendering pass.
- IR-07 investigates scrolling spikes with the terminal closed. System-wide captures show WindowServer-heavy bursts at about 120 display updates/s; gestures were not synchronized to the captures. Wails' default macOS NSWindow is configured non-opaque with a clear backing color. Test native opacity as a hypothesis before shipping any window changes; do not infer a transparent WKWebView or a confirmed cause from the NSWindow flag.
- Approved experiment: when each main window's runtime is ready, set its native AppKit backing color to the adaptive, opaque `windowBackgroundColor`, then `opaque = YES`, on the UI thread. Do not change WKWebView layers, alpha, titlebar, shadows or child views. Apply only to main windows, not About. A temporary macOS-only `AGENT_SESSIONS_MACOS_OPAQUE=0` process environment override retains the original Wails behavior for same-binary A/B; unset enables the experiment. No settings UI or persisted preference. The user subsequently reported 0% GPU when idle with the app still open (2026-11-11). This supports an idle benefit on their setup, not a controlled causal benchmark or verified scrolling improvement; retain the comparison switch for further validation.

## Decisions

Use IntersectionObserver for visibility (no per-frame loop); disconnect on component teardown. Treat lack of IntersectionObserver conservatively with static icons. No document-level visibility tracking (see IR-04). No new settings or dependencies.

## Risks

Linux scrollbar stability needs native verification; retain the existing transform there. Observer setup must handle changing icon classes and teardown. CSS/DOM tests cannot prove GPU reduction. Earlier native macOS tests improved but still peaked near 70% before settling at 20–30%, then averaged about 30% after the low-rate indicators. On 2026-11-11 the user reported 0% idle after the opacity change. Startup attribution, controlled comparisons and post-opacity scrolling/active-terminal behavior remain open; [IR-05](../tasks/idle-rendering/IR-05.md) and [IR-07](../tasks/idle-rendering/IR-07.md) track them.

## Release criteria

Automated tests cover static liveness, platform-scoped compositing, spinner/dot intersection lifecycles and shared-clock cleanup, discrete steps, reduced-motion behavior, and the absence of global animation pauses; frontend checks/build pass. Native validation must also confirm the discrete work indicators remain readable and compare the active-terminal workload. Native macOS measurements are reported honestly; Windows/WebView2 and Linux/WebKitGTK GPU and scrollbar QA remain explicit gates until actually run.

Tasks: [idle-rendering](../tasks/idle-rendering/README.md). Results and outstanding native gates: [QA record](idle-rendering-qa.md).

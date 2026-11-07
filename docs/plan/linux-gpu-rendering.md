# Linux: keep WebKit's GPU renderer

Planned 2026-11-07. Tasks: [linux-gpu-rendering](../tasks/linux-gpu-rendering/README.md).

## Goal

The Linux desktop app scrolls and animates smoothly, also on fractional
scales (125 %, 150 %), and its main thread is idle when nothing happens.

## Today

`cmd/agent-sessions/main.go` sets `WEBKIT_DISABLE_DMABUF_RENDERER=1` in
`init()` unless the user exported it. It comes from the Wails v2 scaffold
(M1-15), where it worked around blank windows on NVIDIA and some Mesa
drivers.

Without DMA-BUF, WebKit hands each frame to the UI process in shared
memory. The GTK main thread then copies it (`cairo_surface_flush` →
`memcpy`) and composites it in software (`cairo_paint` →
`pixman_image_composite32`). Any animation repaints all the time, such as
a live session's pulse or a spinner. On a laptop with an AMD GPU at 1.5× scale
(HawkPoint, Mesa, GNOME Wayland with `scale-monitor-framebuffer`), the
main thread sat at 100 % with nobody touching the app, in 1.2.8 and 1.3.1
alike. Input waits behind it, so the app stutters.

Wails v3 already has the workaround, done better: its `init()` disables
DMA-BUF only when WebKit renders on NVIDIA (`isNVIDIAGPU` in
`wails/v3/pkg/application/gpu_linux.go`, which reads sysfs, PRIME offload
and the session's GPU). Its `init()` runs before ours, so ours only ever
switched off DMA-BUF on GPUs that do not need it.

## Design

Remove our `init()`. What stays:

- Wails disables DMA-BUF where WebKit renders on NVIDIA.
- `WEBKIT_DISABLE_DMABUF_RENDERER` exported by the user wins, either way
  (`=1` to force the old path, `=0` to keep DMA-BUF on NVIDIA).
- The web view's hardware acceleration policy stays `ALWAYS` (Wails maps
  `WebviewGpuPolicyOnDemand` to it on WebKitGTK 6).

## Decisions

- Trust Wails' detection rather than keep a list of our own. If a Mesa
  driver shows a blank window, the variable is the fallback, and the Linux
  note in TECH-NOTES says so.

## Risks

- A Mesa driver that cannot import DMA-BUF shows a blank window until the
  user exports `WEBKIT_DISABLE_DMABUF_RENDERER=1`. None is known on current
  Mesa.
- Only an AMD Wayland session was measured. NVIDIA (Wails' path) and X11
  are not checked here.

## Release criteria

- On the AMD 1.5× session, the idle main thread drops from 100 % to about
  10 %, and the window draws normally.
- Exporting the variable still forces either renderer.
- `go vet` and `go test` pass.

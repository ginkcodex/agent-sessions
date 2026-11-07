# Linux GPU rendering — tasks

- **Plan:** [linux-gpu-rendering.md](../../plan/linux-gpu-rendering.md)
- **Progress:** 1/2 done

Planned 2026-11-07. GR-01 stops turning off WebKit's DMA-BUF renderer on
every Linux machine; GR-02 checks it on real desktops.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [GR-01](GR-01.md) | ✅ Done | Leave DMA-BUF to Wails' NVIDIA check | 1 h | — |
| [GR-02](GR-02.md) | 🟡 Partial (AMD Wayland only; NVIDIA and X11 not checked) | Check on Linux desktops | 1 h | GR-01 |

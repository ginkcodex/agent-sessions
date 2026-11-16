# LWT-02 — Verify Linux native theme behavior

- **Status:** ⛔ Blocked — no interactive X11/Wayland desktop available in this environment
- **Estimate:** 1 h
- **Depends on:** LWT-01
- **Plan:** [linux-window-theme.md](../../plan/linux-window-theme.md)

## Scope and likely files

Run the desktop client on Linux and record actual visual results here. No new features.

## Acceptance criteria and tests

- Light against a dark desktop and Dark against a light desktop produce matching native chrome where supported.
- System follows live desktop changes and switching back clears any override.
- Saved mode works after restart; multiple-window behavior matches the documented GTK scope.
- Native minimize/maximize/close, dragging/resizing, fullscreen/restore and unsaved-file close guard remain intact.
- Check GTK client-side and external title bars under X11/Wayland where available; identify ignored compositor hints instead of claiming universal support.
- Exercise the supported older GTK baseline and current GTK; record theme names and versions used.

## Evidence

2026-11-16: No interactive X11/Wayland session, Xvfb or Weston was available. The native helper was exercised with GTK 4.22.4 using Broadway (see LWT-01), including actual widget color changes. This does not verify a real desktop's title-bar appearance, live system-setting events, input controls or compositor hints. All visual acceptance checks above remain pending, as does runtime testing on the GTK 4.14 baseline.


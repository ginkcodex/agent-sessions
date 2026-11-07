# Two-finger swipe on Windows and Linux — tasks

- **Plan:** [swipe-navigation-windows-linux.md](../../plan/swipe-navigation-windows-linux.md)
- **Progress:** 7/10 done

Planned 2026-11-07. Linux spots the swipe natively (SN-01) and sends
`nav:swipe` as macOS does; Windows spots it from the page's wheel events
(SN-02), which also makes text fields count as sideways scrollers. SN-03 and
SN-04 check them on real touchpads. SN-05 came from SN-03: swipes in a row.
SN-06 came after it: one swipe must be one step. SN-07 came from a Linux
trace: GTK hands the swipe each scroll twice. SN-08 came from Windows: one
swipe still went several steps, so the wheel stream no longer splits into
strokes and swipes once at most. That still failed on a real touchpad: the
page's wheel stream changes with the renderer's load. SN-09 reads the
touchpad natively on Windows too, and SN-10 removes the page's detector.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [SN-01](SN-01.md) | ✅ Done | Linux: spot two-finger touchpad swipes, send `nav:swipe` | 2 h | — |
| [SN-02](SN-02.md) | ✅ Done | Frontend: Windows swipe from wheel events; text fields scroll | 3 h | — |
| [SN-03](SN-03.md) | 🟡 Partial (swipes work with SN-09; sideways scrolling and the mouse not reported) | Check on Windows | 1 h | SN-02, SN-09 |
| [SN-04](SN-04.md) | 🟡 Partial (one Wayland trace; criteria not walked) | Check on Linux | 1 h | SN-01 |
| [SN-05](SN-05.md) | ✅ Done | Swipes in a row: split the wheel stream into strokes | 2 h | SN-02 |
| [SN-06](SN-06.md) | ✅ Done | One swipe, one step: stricter strokes; Linux gestures end on lift | 2 h | SN-05 |
| [SN-07](SN-07.md) | ✅ Done | Linux: skip replayed scrolls, so one swipe sends once; trace switch | 2 h | SN-06 |
| [SN-08](SN-08.md) | ➖ Superseded (by SN-09, SN-10) | Windows: one swipe, one step; no strokes, a cooldown after a swipe | 2 h | SN-06 |
| [SN-09](SN-09.md) | ✅ Done | Windows: native swipe from the Precision Touchpad (Raw Input), one per touch | 4 h | SN-08 |
| [SN-10](SN-10.md) | ✅ Done | Frontend: drop the wheel detector; Windows swipes come as `nav:swipe` | 2 h | SN-09 |

# Two-finger swipe on Windows and Linux — tasks

- **Plan:** [swipe-navigation-windows-linux.md](../../plan/swipe-navigation-windows-linux.md)
- **Progress:** 5/7 done

Planned 2026-11-07. Linux spots the swipe natively (SN-01) and sends
`nav:swipe` as macOS does; Windows spots it from the page's wheel events
(SN-02), which also makes text fields count as sideways scrollers. SN-03 and
SN-04 check them on real touchpads. SN-05 came from SN-03: swipes in a row.
SN-06 came after it: one swipe must be one step. SN-07 came from a Linux
trace: GTK hands the swipe each scroll twice.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [SN-01](SN-01.md) | ✅ Done | Linux: spot two-finger touchpad swipes, send `nav:swipe` | 2 h | — |
| [SN-02](SN-02.md) | ✅ Done | Frontend: Windows swipe from wheel events; text fields scroll | 3 h | — |
| [SN-03](SN-03.md) | 🟡 Partial (swipes in a row failed; recheck after SN-06) | Check on Windows | 1 h | SN-02 |
| [SN-04](SN-04.md) | 🟡 Partial (one Wayland trace; criteria not walked) | Check on Linux | 1 h | SN-01 |
| [SN-05](SN-05.md) | ✅ Done | Swipes in a row: split the wheel stream into strokes | 2 h | SN-02 |
| [SN-06](SN-06.md) | ✅ Done | One swipe, one step: stricter strokes; Linux gestures end on lift | 2 h | SN-05 |
| [SN-07](SN-07.md) | ✅ Done | Linux: skip replayed scrolls, so one swipe sends once; trace switch | 2 h | SN-06 |

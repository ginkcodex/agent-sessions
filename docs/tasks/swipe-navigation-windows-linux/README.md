# Two-finger swipe on Windows and Linux — tasks

- **Plan:** [swipe-navigation-windows-linux.md](../../plan/swipe-navigation-windows-linux.md)
- **Progress:** 2/4 done

Planned 2026-11-07. Linux spots the swipe natively (SN-01) and sends
`nav:swipe` as macOS does; Windows spots it from the page's wheel events
(SN-02), which also makes text fields count as sideways scrollers. SN-03 and
SN-04 check them on real touchpads.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [SN-01](SN-01.md) | ✅ Done | Linux: spot two-finger touchpad swipes, send `nav:swipe` | 2 h | — |
| [SN-02](SN-02.md) | ✅ Done | Frontend: Windows swipe from wheel events; text fields scroll | 3 h | — |
| [SN-03](SN-03.md) | ⬜ Not started | Check on Windows | 1 h | SN-02 |
| [SN-04](SN-04.md) | ⬜ Not started | Check on Linux | 1 h | SN-01 |

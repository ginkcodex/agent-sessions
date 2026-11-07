# Two-finger swipe back and forward — tasks

- **Plan:** [swipe-navigation.md](../../plan/swipe-navigation.md)
- **Progress:** 0/3 done

Planned 2026-11-07. The native swipe (SW-01) sends `nav:swipe`; the
frontend (SW-02) drops swipes that scrolled and delivers the rest to the
session list and Go to File. SW-03 checks it on a Mac.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [SW-01](SW-01.md) | ⬜ Not started | macOS: spot two-finger swipes, send `nav:swipe` | 2 h | — |
| [SW-02](SW-02.md) | ⬜ Not started | Frontend: drop swipes that scrolled, deliver the rest | 2 h | SW-01 |
| [SW-03](SW-03.md) | ⬜ Not started | Check on a Mac | 1 h | SW-02 |

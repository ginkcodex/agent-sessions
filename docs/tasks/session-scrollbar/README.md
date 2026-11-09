# Session scrollbar — tasks

- **Plan:** [session-scrollbar.md](../../plan/session-scrollbar.md)
- **Progress:** 8/9 done · 1 partial

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [SS-01](SS-01.md) | ✅ Done — ineffective; reverted | Remove legacy scrolling hint; automated checks and Linux visual confirmation | 1 h | — |
| [SS-02](SS-02.md) | ✅ Done — ineffective; reverted | Disable Linux row highlight animation only | 1 h | SS-01 |
| [SS-03](SS-03.md) | ✅ Done — ineffective; reverted | Remove virtual-row containment only | 1 h | SS-02 |
| [SS-04](SS-04.md) | ✅ Done — ineffective; reverted | Replace row transforms with top positioning | 1 h | SS-03 |
| [SS-05](SS-05.md) | ✅ Done | Measure native scroll geometry and reproduce in the actual desktop app | 2 h | SS-04 |
| [SS-06](SS-06.md) | ✅ Done — ineffective; reverted | Normal-flow virtual rows with spacers | 3 h | SS-05 |
| [SS-07](SS-07.md) | ✅ Done | Give the scroll container its own compositing layer (the fix) | 2 h | SS-06 |
| [SS-08](SS-08.md) | 🟡 Partial — buttons user-confirmed; remaining native checks pending | Transcript Resume/New flashing during rapid Linux list scrolling | 2 h | SS-07 |
| [SS-09](SS-09.md) | ✅ Done — ineffective; reverted | Shared Linux header navigation/badge layer | 1 h | SS-08 |

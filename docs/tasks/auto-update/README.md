# Auto-update — tasks

- **Plan:** [auto-update.md](../../plan/auto-update.md)
- **Progress:** 10/15 done

Planned 2026-11-13. AU-01 signs releases; AU-02…05 are the shared update
flow; AU-06…08 are macOS, which ships first; AU-09…11 are Windows. AU-12
(Windows code signing) is wanted but not needed for the release; AU-13 is
the docs. AU-14 and AU-15, added later the same day, check for updates by hand.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [AU-01](AU-01.md) | ✅ Done | Sign `SHA256SUMS` with minisign in the release job | 2 h | — |
| [AU-02](AU-02.md) | ✅ Done | Find an update from GitHub's latest release; signature check | 3 h | AU-01 |
| [AU-03](AU-03.md) | ✅ Done | Download, check and keep the ready version | 3 h | AU-02 |
| [AU-04](AU-04.md) | ✅ Done | Setting and update state for the frontend | 2.5 h | AU-03 |
| [AU-05](AU-05.md) | ✅ Done | Frontend: ready toast, Later, fallback, setting | 3 h | AU-04 |
| [AU-06](AU-06.md) | ✅ Done | macOS: when possible, mount, check and stage the app | 3.5 h | AU-03 |
| [AU-07](AU-07.md) | ✅ Done | macOS: apply on restart or quit, with rollback | 3.5 h | AU-04, AU-06 |
| [AU-08](AU-08.md) | ⬜ Not started | macOS QA on a Mac | 3 h | AU-05, AU-07 |
| [AU-09](AU-09.md) | ✅ Done | Windows: when possible, download and check new and rollback MSIs | 3 h | AU-03 |
| [AU-10](AU-10.md) | ✅ Done | Windows: apply on restart or quit, with rollback | 4 h | AU-04, AU-09 |
| [AU-11](AU-11.md) | ⬜ Not started | Windows QA on Windows | 3 h | AU-05, AU-10 |
| [AU-12](AU-12.md) | ⬜ Not started | Windows code signing in CI | 3 h | — |
| [AU-13](AU-13.md) | ⬜ Not started | Docs: README, website, Windows packaging notes | 1.5 h | AU-08, AU-11, AU-14 |
| [AU-14](AU-14.md) | ✅ Done | Check for updates by hand in Settings | 3 h | AU-05 |
| [AU-15](AU-15.md) | ⬜ Not started | macOS app menu: Check for Updates… | 2 h | AU-14 |

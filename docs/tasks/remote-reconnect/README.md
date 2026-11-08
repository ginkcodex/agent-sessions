# Remote reconnect — tasks

- **Plan:** [remote-reconnect.md](../../plan/remote-reconnect.md)
- **Progress:** 3/7 done

Planned 2026-11-08. RR-01 measures today's recovery with silent failures.
RR-02…RR-04 make the backend notice a dead link within about 20 s and redial
over a fresh connection without stalling; RR-05 and RR-06 are the backoff,
Retry and same-host reload changes in the UI path. RR-07 measures again and
records native checks.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [RR-01](RR-01.md) | ✅ Done | Phase-timing log; silent-failure sshd scenarios; baseline | 3 h | — |
| [RR-02](RR-02.md) | ✅ Done | `$/ping` on the server; byte-level liveness watchdog in the client | 4 h | — |
| [RR-03](RR-03.md) | ✅ Done | Close the master after a silent drop; drop reason in the banner | 2 h | RR-02 |
| [RR-04](RR-04.md) | ⬜ Not started | 45 s limit on automatic attempts, paused for askpass prompts | 3.5 h | — |
| [RR-05](RR-05.md) | ⬜ Not started | Backoff that stays at 10 s; Retry while reconnecting | 1.5 h | — |
| [RR-06](RR-06.md) | ⬜ Not started | Same-host reconnect keeps the selected session and history | 3 h | — |
| [RR-07](RR-07.md) | ⬜ Not started | Integrated measurement, M7 doc update, native check record | 2.5 h | RR-01…RR-06 |

# M7 — Remote over SSH (≈ 65 h) — tasks

- **Plan:** [M7.md](../../plan/M7.md)
- **Progress:** 20/21 done

Order: docs and engine extraction
(M7-01…07), RPC and server (M7-08…10), SSH/askpass/deploy (M7-11…14),
flows/UI (M7-15…17), packaging (M7-18), and QA (M7-19…20).

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M7-01](M7-01.md) | ✅ Done | SPEC §1/§9/§13, `docs/plan/M7.md`, this table | 2 h | — |
| [M7-02](M7-02.md) | ✅ Done | `internal/version` plus ldflags in Makefile and release.yml | 2 h | M7-01 |
| [M7-03](M7-03.md) | ✅ Done | `Emitter` interface | 3 h | M7-01 |
| [M7-04](M7-04.md) | ✅ Done | Move Service, DTOs, actions and LRUs | 4 h | M7-03 |
| [M7-05](M7-05.md) | ✅ Done | Move the Wails-free handoff/export/import/manage/search logic | 4 h | M7-04 |
| [M7-06](M7-06.md) | ✅ Done | Engine lifecycle | 4 h | M7-05 |
| [M7-07](M7-07.md) | ✅ Done | `Backend` contract and local adapter | 3 h | M7-06 |
| [M7-08](M7-08.md) | ✅ Done | `internal/rpc`: framing, preface, caps, cancel, error registry | 4 h | M7-07 |
| [M7-09](M7-09.md) | ✅ Done | RPC dispatcher, notifications and client backend | 4 h | M7-08 |
| [M7-10](M7-10.md) | ✅ Done | `serve --stdio`, initialize handshake, EOF/idle exit, staging and `transfer` subcommands | 4 h | M7-09 |
| [M7-11](M7-11.md) | ✅ Done | `internal/remote` hosts parser, argv builder, ControlMaster, quoting | 4 h | M7-01 |
| [M7-12](M7-12.md) | ✅ Done | Askpass helper branch in main, secure broker, Wails prompt event and reply | 4 h | M7-11 |
| [M7-13](M7-13.md) | ✅ Done | Probe, OS/arch mapping, server locator, atomic deploy and version verification | 4 h | M7-02, M7-11 |
| [M7-14](M7-14.md) | ✅ Done | Remote session supervisor | 4 h | M7-09, M7-10, M7-12, M7-13 |
| [M7-15](M7-15.md) | ✅ Done | Artifact transfer | 4 h | M7-14 |
| [M7-16](M7-16.md) | ✅ Done | Frontend api/types/mock/connection store | 4 h | M7-14 |
| [M7-17](M7-17.md) | ✅ Done | Host chip and menu, banners, host-aware confirmations, AskpassDialog, per-host env… | 4 h | M7-16 |
| [M7-18](M7-18.md) | ✅ Done | `make remote-servers` | 3 h | M7-02, M7-10 |
| [M7-19](M7-19.md) | ✅ Done | Opt-in SSH integration harness | 4 h | M7-15, M7-18 |
| [M7-20](M7-20.md) | 🟡 Partial | QA and security review | 3 h | all |
| [M7-21](M7-21.md) | ✅ Done | Sharing a host: one server per client, index writer/readers per schema and provider… | 4 h | M7-20 |

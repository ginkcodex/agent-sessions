# Linux repositories — tasks

- **Plan:** [linux-repos.md](../../plan/linux-repos.md)
- **Progress:** 4/20 done

Planned 2026-11-13. LR-01 and LR-02 check the risky assumptions first.
LR-03…06 record releases in the website Worker (`agent-sessions-web`);
LR-11…14 build and serve the repositories there. LR-07…10 change this
repository's CI; LR-16 tests the result in containers; LR-17 is the app's
Linux toast. LR-15 and LR-19 need the maintainer's keys and accounts. LR-20 is a
website test flake found during LR-01.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [LR-01](LR-01.md) | ✅ Done | Spike: Worker code beside static assets, Hyperdrive, CPU budget | 2 h | — |
| [LR-02](LR-02.md) | ⬜ Not started | Spike: redirects, Ed25519 signatures with gpgv/sqv/dnf, nfpm RPM key type | 3 h | LR-01 |
| [LR-03](LR-03.md) | ⬜ Not started | Schema, migrations, local Postgres | 2 h | LR-01 |
| [LR-04](LR-04.md) | ⬜ Not started | GitHub release reader: assets, `SHA256SUMS` checks, classification | 3 h | LR-03 |
| [LR-05](LR-05.md) | ⬜ Not started | `POST /api/ingest`: auth, transaction, idempotency, rebuild hook | 3 h | LR-04 |
| [LR-06](LR-06.md) | ⬜ Not started | Hourly cron catch-up and `npm run backfill` | 2 h | LR-05 |
| [LR-07](LR-07.md) | ⬜ Not started | Release workflow calls ingest | 1.5 h | LR-05 |
| [LR-08](LR-08.md) | ⬜ Not started | Sign RPMs in CI and check them | 2 h | LR-02, LR-15 |
| [LR-09](LR-09.md) | ✅ Done | `repo-meta.json` for `.deb` files; release set includes it | 3 h | — |
| [LR-10](LR-10.md) | ✅ Done | `repo-meta.json` for `.rpm` files | 3 h | LR-09 |
| [LR-11](LR-11.md) | ⬜ Not started | Worker: apt index builder | 3 h | LR-03, LR-09 |
| [LR-12](LR-12.md) | ⬜ Not started | Worker: dnf repodata builder | 3 h | LR-03, LR-10 |
| [LR-13](LR-13.md) | ⬜ Not started | Worker: signing and public key files | 2.5 h | LR-02 |
| [LR-14](LR-14.md) | ⬜ Not started | Worker: generations, rebuild, serving and redirects | 3.5 h | LR-05, LR-11, LR-12, LR-13 |
| [LR-15](LR-15.md) | ⬜ Not started | Keys: generate, store, renewal and rotation procedure | 2 h | LR-02 |
| [LR-16](LR-16.md) | ⬜ Not started | Container checks for Ubuntu, Debian and Fedora | 3 h | LR-14 |
| [LR-17](LR-17.md) | ✅ Done | App: Linux update toast knows the repository | 2.5 h | — |
| [LR-18](LR-18.md) | ⬜ Not started | Install docs: README and website | 2 h | LR-16 |
| [LR-19](LR-19.md) | ⬜ Not started | Production setup, backfill and first release through the repositories | 2.5 h | LR-06, LR-07, LR-08, LR-15, LR-16 |
| [LR-20](LR-20.md) | ⬜ Not started | Website: theme menu test fails now and then | 1 h | — |

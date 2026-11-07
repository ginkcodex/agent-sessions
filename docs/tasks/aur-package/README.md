# AUR package — tasks

- **Plan:** [aur-package.md](../../plan/aur-package.md)
- **Progress:** 4/7 done

Planned 2026-11-08. AUR-01…03 build, check and publish `agent-sessions-bin`
from a release's `.deb`; AUR-04 runs that from CI after each stable release;
AUR-05 documents the install. AUR-06 and AUR-07 need the maintainer's AUR
account: the first push, and the CI secret.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [AUR-01](AUR-01.md) | ✅ Done | PKGBUILD template and render script | 1.5 h | — |
| [AUR-02](AUR-02.md) | ✅ Done | Build, install and remove it in an Arch container; `make aur-package` | 2 h | AUR-01 |
| [AUR-03](AUR-03.md) | ✅ Done | Publish script for an AUR checkout | 1.5 h | AUR-02 |
| [AUR-04](AUR-04.md) | 🟡 Partial (not run on GitHub yet; see AUR-07) | AUR workflow, called after stable releases | 1.5 h | AUR-03 |
| [AUR-05](AUR-05.md) | ✅ Done | README install instructions | 0.5 h | AUR-01 |
| [AUR-06](AUR-06.md) | ⬜ Not started | First push of the current release to the AUR | 0.5 h | AUR-03 |
| [AUR-07](AUR-07.md) | ⬜ Not started | CI secret and the first automated update | 1 h | AUR-04, AUR-06 |

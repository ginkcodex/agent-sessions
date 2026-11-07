# Consent-based managed tools — tasks

- **Plan:** [tool-installation.md](../../plan/tool-installation.md)
- **Progress:** 5/6 done

> **Update 2026-10-23:** TI-03's remote fail-closed route is replaced: `managedTool` is forwarded to the connected SSH/WSL host, which installs into its own app data. The operator-reviewed Go gate is gone; the gopls release follows the host's Go. Not verified on a real remote host or with a real download. See the [editor (Zed) model](../../plan/code-diagnostics-zed-model.md).

> **Update 2026-10-23 (later):** at the user's request downloads are automatic, like Zed: a missing server the project needs is proposed and installed without a click, once per host and tool per app session, with a setting to turn it off (then every download goes through the review card). vtsls reuses the host's Node.js 18 or newer when found. Running a server still needs project approval. Frontend unit tests only.

Date: 2026-10-22. This is independent of diagnostics **19/28** and R01–R20.
No task here clears CD-09, Stage 1/2 HOLDs, or enables live capability.
Each task includes tests/evidence and is at most four developer hours; split again
before exceeding that cap. Design/evidence: [managed tools](../../plan/tool-installation.md).

| Task | Status | Scope | Est |
|---|---|---|---|
| [TI-01](TI-01.md) | ✅ Done | Closed gopls catalog and offline installer core | 4h |
| [TI-02](TI-02.md) | ✅ Done | Backend ownership and trusted Go selection | 4h |
| [TI-03](TI-03.md) | ✅ Done | Optional install contract and bounded jobs | 4h |
| [TI-04](TI-04.md) | ✅ Done | Frontend setup controls | 4h |
| [TI-05](TI-05.md) | ⬜ Not started | Real build and install evidence per platform | 4h per platform cell |
| [TI-06](TI-06.md) | ✅ Done | Managed rust-analyzer and Rust preparation | 4h |

## Scope change authorized by user

The previous blanket no-download rule is superseded **only** for explicit,
host-scoped, user-approved managed installation. Feature implementation does not
approve running an installer on this development host. No real tool installation
or module artifact download is needed in TI-01 tests. Reading official source and
checksum metadata is research, not an install. Project execution approval never
implies installation approval, nor the reverse. Existing R11 preinstalled-server
smoke is a separate work unit, not evidence for installation.

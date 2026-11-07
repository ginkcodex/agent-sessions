# Git history — tasks

- **Plan:** [git-history.md](../../plan/git-history.md)
- **Progress:** 17/18 done

- Progress: **17/18 complete**. GH-01…GH-17 are complete; GH-18 is blocked on native UI access.
- Scope: IntelliJ-inspired History layout inside Go to File. Selection is read-only;
  the 2026-10-27 extension adds tags, explicit confirmed ref actions and saved-disk
  working-tree comparison (see [contract](../../plan/git-history-contract.md)).
- Extension automated coverage includes temporary-repository mutation safety,
  original-workspace authorization, transport/capability gating, comparison
  staleness, frontend orchestration and SSR. Native menu/keyboard/theme/narrow
  layout and Windows/SSH/WSL interaction QA remain pending under GH-18.
- Estimates include focused tests, not elapsed-time commitments. Each package
  is at most four developer hours; split before starting if it grows beyond
  that, rather than dropping acceptance criteria.
- Follow repository Definition of Done. Record implementation, automated tests,
  native UI QA, and unavailable platform coverage separately.

## Task map

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [GH-01](GH-01.md) | ✅ Done | Contracts, limits and fixtures | 3 h | — |
| [GH-02](GH-02.md) | ✅ Done | Authorized repository resolution/discovery | 4 h | GH-01 |
| [GH-03](GH-03.md) | ✅ Done | Read-only runner and branches | 3 h | GH-01, GH-02 |
| [GH-04](GH-04.md) | ✅ Done | Pinned pagination and search | 4 h | GH-03 |
| [GH-05](GH-05.md) | ✅ Done | Commit details and scoped changes | 4 h | GH-04 |
| [GH-06](GH-06.md) | ✅ Done | Immutable historical diffs | 4 h | GH-05 |
| [GH-07](GH-07.md) | ✅ Done | Engine authorization and app routing | 3 h | GH-02…GH-06 |
| [GH-08](GH-08.md) | ✅ Done | RPC capability and compatibility | 4 h | GH-07 |
| [GH-09](GH-09.md) | ✅ Done | Frontend API, mocks and bindings | 3 h | GH-01, GH-08 |
| [GH-10](GH-10.md) | ✅ Done | Stable tab identity | 4 h | GH-01 |
| [GH-11](GH-11.md) | ✅ Done | Immutable sources and viewer safeguards | 4 h | GH-06, GH-09, GH-10 |
| [GH-12](GH-12.md) | ✅ Done | History layout and splitters | 4 h | GH-01, GH-10 |
| [GH-13](GH-13.md) | ✅ Done | History store and stale-response guards | 4 h | GH-09 |
| [GH-14](GH-14.md) | ✅ Done | Repository/branch navigation and commit list | 4 h | GH-12, GH-13 |
| [GH-15](GH-15.md) | ✅ Done | Details, files and bottom diff | 4 h | GH-11, GH-14 |
| [GH-16](GH-16.md) | ✅ Done | Refresh and lifecycle integration | 3 h | GH-15 |
| [GH-17](GH-17.md) | ✅ Done | Automated security/integration gate | 4 h | GH-16 |
| [GH-18](GH-18.md) | 🟡 Partial | Native UI and platform/remote QA | 4 h | GH-17 |

Dependency ranges include all tasks between their endpoints. Do not treat the
map as authorization for multi-agent implementation. GH-10 and backend work can
proceed independently after GH-01, but shared viewer/store edits need a single
owner. GH-12 can use fixtures; production integration must wait for its listed
dependencies. Do not claim remote support validated merely from mocks.

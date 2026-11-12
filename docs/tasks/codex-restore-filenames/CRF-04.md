# CRF-04 — Route repair to the owning host

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CRF-03
- **Plan:** [codex-restore-filenames.md](../../plan/codex-restore-filenames.md)

## Scope

Optional engine backend, active-host desktop binding, RPC capability/client/server. Unsupported hosts must fail without local fallback.

## Acceptance criteria and tests

Automated tests establish the corresponding in-app recovery contract in the
plan. No real-user files are modified. Record commands, results and remaining
manual UI/native-host limits in Evidence.

## Evidence

- Optional CodexRepairBackend, engine scan/refresh after success, active-host
  desktop binding and capability-gated repairCodexSession RPC. Older hosts
  return unsupported and cannot fall back to the local data store.
- Tests cover RPC round-trip/result/capability, old host rejection, active-host
  desktop routing and real engine catalog path refresh.
- Full engine/RPC/app suites pass with the previously baseline-reproduced
  TestGitHistoryAuthorizedChild failure excluded. Targeted repair tests pass
  under Go's race detector. No live SSH/WSL host was used.

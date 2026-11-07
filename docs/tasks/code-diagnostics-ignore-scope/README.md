# Code checks: Git ignore rules in discovery — tasks

- **Plan:** [code-diagnostics-ignore-scope.md](../../plan/code-diagnostics-ignore-scope.md)
- **Progress:** 4/6 done

Planned 2026-11-07. The Git query comes first (IS-01), then the walks that
use it (IS-02, IS-03), then a check against this checkout (IS-04). IS-05 was
found while planning and is independent; IS-06 was found during IS-05. Each
task ends with `go test` for the packages it touches passing and no new
`golangci-lint` findings.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [IS-01](IS-01.md) | ✅ Done | `files.IgnoredDirectories`: hardened Git query | 2 h | — |
| [IS-02](IS-02.md) | ✅ Done | Git scope for walks; gopls and go vet discovery | 4 h | IS-01 |
| [IS-03](IS-03.md) | ✅ Done | TypeScript, Python, Ruby, nested-workspace and Svelte discovery | 3 h | IS-02 |
| [IS-04](IS-04.md) | 🟡 Partial: desktop app not checked | Check discovery on this checkout | 1 h | IS-02, IS-03 |
| [IS-05](IS-05.md) | ✅ Done | Files, Changes and History: Git without `core.fsmonitor` | 2 h | — |
| [IS-06](IS-06.md) | ⬜ Not started | Git reads without repository filter programs | 3 h | IS-05 |

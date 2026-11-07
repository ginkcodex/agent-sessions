# Code checks: choosing languages — tasks

- **Plan:** [code-diagnostics-languages.md](../../plan/code-diagnostics-languages.md)
- **Progress:** 8/8 done

Planned 2026-10-30. Each task is at most four developer hours and ends with
its tests passing: `go test ./internal/...`, `GOOS=windows` and `GOOS=linux`
`go vet ./internal/... ./cmd/...`, and from `frontend/` `npm test` and
`npx svelte-check --threshold error`. Native Windows/Linux runs and real remote
hosts are outside these tasks unless a brief says otherwise.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [CL-01](CL-01.md) | ✅ Done | Language table; hard-coded ID lists use it | 3 h | — |
| [CL-02](CL-02.md) | ✅ Done | One bridge for plain diagnostic pushes | 3 h | CL-01 |
| [CL-03](CL-03.md) | ✅ Done | Live discovery from the table | 4 h | CL-01 |
| [CL-04](CL-04.md) | ✅ Done | Discovery skips languages that are off | 4 h | CL-03 |
| [CL-05](CL-05.md) | ✅ Done | `disabledLanguages` over Engine, App and RPC | 3 h | CL-04 |
| [CL-06](CL-06.md) | ✅ Done | Frontend preference and display | 4 h | CL-05 |
| [CL-07](CL-07.md) | ✅ Done | Settings section "Code checks" | 3 h | CL-06 |
| [CL-08](CL-08.md) | ✅ Done | Contract, docs and gate | 2 h | CL-07 |

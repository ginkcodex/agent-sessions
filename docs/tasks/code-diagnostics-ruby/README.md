# Code checks: Ruby (RuboCop) — tasks

- **Plan:** [code-diagnostics-ruby.md](../../plan/code-diagnostics-ruby.md)
- **Progress:** 7/7 done

Planned 2026-11-07. Each task is at most four developer hours and ends with
its tests passing: `go test ./internal/...`, `GOOS=windows` and `GOOS=linux`
`go vet ./internal/... ./cmd/...`, and from `frontend/` `npm test` and
`npx svelte-check --threshold error`. Native Windows/Linux runs and real remote
hosts are outside these tasks unless a brief says otherwise. Tests built on
fake trees or fake servers are synthetic and are labeled so.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [RB-00](RB-00.md) | ✅ Done | Spike: real `rubocop --lsp` behavior | 3 h | — |
| [RB-01](RB-01.md) | ✅ Done | Ruby interpreter and version | 4 h | — |
| [RB-02](RB-02.md) | ✅ Done | Gems and Bundler | 4 h | RB-01 |
| [RB-03](RB-03.md) | ✅ Done | RuboCop adapter | 4 h | RB-00, RB-02 |
| [RB-04](RB-04.md) | ✅ Done | Language table, bridge, scan, engine | 4 h | RB-03 |
| [RB-05](RB-05.md) | ✅ Done | Frontend | 3 h | RB-04 |
| [RB-06](RB-06.md) | ✅ Done | Smoke test, docs and gate | 3 h | RB-05 |

RB-00 and RB-01 can run in parallel.

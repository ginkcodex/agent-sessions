# Ruby Go to definition (ruby-lsp) — tasks

- **Plan:** [code-diagnostics-ruby-lsp.md](../../plan/code-diagnostics-ruby-lsp.md)
- **Progress:** 1/10 done

Planned 2026-11-07; RL-01…RL-09 are written from the spike's evidence. Each
task is at most four developer hours and ends with its tests passing:
`go test ./internal/...`, `GOOS=windows` and `GOOS=linux`
`go vet ./internal/... ./cmd/...`, and from `frontend/` `npm test` and
`npx svelte-check --threshold error`. Tests built on fake trees or fake
servers are synthetic and are labeled so. RL-07 and RL-08 run the real
ruby-lsp gem and need the user's approval for that. RL-08 is optional for the
first release.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [RL-00](RL-00.md) | ✅ Done | Spike: real ruby-lsp behavior | 4 h | — |
| [RL-01](RL-01.md) | ⬜ Not started | Find ruby-lsp | 3 h | RL-00 |
| [RL-02](RL-02.md) | ⬜ Not started | ruby-lsp adapter: wrapper, launch, approval | 4 h | RL-01 |
| [RL-03](RL-03.md) | ⬜ Not started | Language table, scan, engine | 3 h | RL-02 |
| [RL-04](RL-04.md) | ⬜ Not started | Index freshness: saved files and drafts | 4 h | RL-03 |
| [RL-05](RL-05.md) | ⬜ Not started | External locations into Ruby, gems and RBS | 3 h | RL-03 |
| [RL-06](RL-06.md) | ⬜ Not started | Frontend | 3 h | RL-03 |
| [RL-07](RL-07.md) | ⬜ Not started | Real-server smoke test and evidence replay | 4 h | RL-04, RL-05 |
| [RL-08](RL-08.md) | ⬜ Not started | Indexing progress state (optional) | 4 h | RL-07 |
| [RL-09](RL-09.md) | ⬜ Not started | Docs | 2 h | RL-06 |

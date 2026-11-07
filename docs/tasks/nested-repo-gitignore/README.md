# Git ignore rules in nested repositories — tasks

- **Plan:** [nested-repo-gitignore.md](../../plan/nested-repo-gitignore.md)
- **Progress:** 1/5 done

Planned 2026-11-07. The scope query (NG-01) comes before the walk that uses
it (NG-02). The viewer change (NG-03) is independent. NG-04 measures NG-02;
NG-05 checks everything in the desktop app on the reporting host. Each task
ends with `go test` / `npm test` for what it touches passing and no new
`golangci-lint` findings.

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [NG-01](NG-01.md) | ✅ Done | Git ignore scope for walked repositories | 2 h | — |
| [NG-02](NG-02.md) | ⬜ Not started | Walk lists each repository with its own ignore rules | 4 h | NG-01 |
| [NG-03](NG-03.md) | ⬜ Not started | Blame bar says when Git ignores the file | 2 h | — |
| [NG-04](NG-04.md) | ⬜ Not started | Measure walked listings with repositories | 1 h | NG-02 |
| [NG-05](NG-05.md) | ⬜ Not started | Check in the desktop app on `plgl` | 1 h | NG-02, NG-03 |

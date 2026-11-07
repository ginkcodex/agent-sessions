# M6 — Portable sessions and cross-agent handoff (≈ 60 h) — tasks

- **Plan:** [M6.md](../../plan/M6.md)
- **Progress:** 16/17 done

Order: docs and spikes first
(M6-01, M6-02), then the handoff slice (M6-08 → 11, writes nothing to a tool
store), then export (M6-03…07, M6-12), import (M6-13), restore (M6-14…16) and
QA (M6-17).

| Task | Status | Scope | Est | Depends on |
|---|---|---|---|---|
| [M6-01](M6-01.md) | ✅ Done | SPEC §1/§7/§9/§12/§13, M0 contract note, `docs/plan/M6.md`, this table | 2 h | — |
| [M6-02](M6-02.md) | ✅ Done | Format spikes on disposable roots, recorded in `docs/formats.md`: Claude cwd encoder… | 4 h | — |
| [M6-03](M6-03.md) | ✅ Done | `internal/bundle`: manifest v1, streaming writer | 4 h | M6-01 |
| [M6-04](M6-04.md) | ✅ Done | `internal/redact`: PEM/token/JWT/assignment detectors, `~` path rewrite, per-rule… | 3 h | — |
| [M6-05](M6-05.md) | ✅ Done | manage/capture.go: Claude and Codex file sets incl. descendants, OpenCode session… | 4 h | M6-02 |
| [M6-06](M6-06.md) | ✅ Done | Full-fidelity transcript for bundles: resolve truncated outputs and blobs via `Blob`… | 3 h | M6-03 |
| [M6-07](M6-07.md) | ✅ Done | Export service/bindings | 3 h | M6-03, M6-04, M6-05, M6-06 |
| [M6-08](M6-08.md) | ✅ Done | Handoff core: sections 1–8, working-state extraction, per-agent tool classification | 4 h | M6-01 |
| [M6-09](M6-09.md) | ✅ Done | Handoff budget/trimming, `Report`, target adapters, golden tests per source→target pair | 4 h | M6-08, M6-04 |
| [M6-10](M6-10.md) | ✅ Done | Handoff delivery: per-agent argv, prompt file + full-context file | 3 h | M6-09 |
| [M6-11](M6-11.md) | ✅ Done | `HandoffDialog` and the "Continue in ▾" header menu, store, api/mock | 4 h | M6-10 |
| [M6-12](M6-12.md) | ✅ Done | `ExportDialog` | 4 h | M6-07 |
| [M6-13](M6-13.md) | ✅ Done | `OpenBundle` and `ImportDialog`: summary, checksums, "Continue in…" from the bundle | 4 h | M6-07, M6-11 |
| [M6-14](M6-14.md) | ✅ Done | Restore engine: `allow_restore`, preview token, live/collision/path checks, cwd remap… | 4 h | M6-05, M6-02 |
| [M6-15](M6-15.md) | ✅ Done | OpenCode import restore | 4 h | M6-14 |
| [M6-16](M6-16.md) | ✅ Done | Catalog event and selection after restore, restore UI in `ImportDialog`, directory… | 3 h | M6-14, M6-13 |
| [M6-17](M6-17.md) | ⬜ Not started | QA: round-trips across two temp homes, cross-agent handoff smoke test with real CLIs | 3 h | M6-01…16 |

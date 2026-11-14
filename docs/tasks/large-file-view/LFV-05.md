# LFV-05 — Raise the edit limit to the read limit

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** LFV-02
- **Plan:** [large-file-view.md](../../plan/large-file-view.md)

## Scope

Text files up to `MaxReadBytes` (5 MiB) can be edited, saved and changed by
replace in files.

## Likely files

- `internal/files/edit_text.go`: `MaxEditBytes = MaxReadBytes`, doc comment.
- `frontend/src/lib/fileEditing.ts`: `MAX_EDIT_BYTES`.
- `frontend/src/lib/mock/mockApi.ts`: the demo's save limit; a large demo file
  (`*fingerprints.json`, about 2.5 MB) to try the large view and editing in the
  browser.
- `docs/plan/file-editing.md`: the limit it names.

## Acceptance criteria and tests

- A 2.5 MB text file reads as eligible for editing; a file over 5 MiB stays
  too large. The Go tests that use `MaxEditBytes` still pass at the new value.
- A save of 5 MiB of text, the worst case once JSON-escaped, fits the RPC frame
  (checked by arithmetic against `MaxFrameBytes`, recorded here).
- `go test ./internal/files/... ./internal/rpc/...` and the frontend tests
  pass.

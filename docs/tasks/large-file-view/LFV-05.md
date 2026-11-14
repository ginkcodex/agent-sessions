# LFV-05 — Raise the edit limit to the read limit

- **Status:** ✅ Done
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

## Evidence

- `MaxEditBytes = MaxReadBytes` (`internal/files/edit_text.go`);
  `MAX_EDIT_BYTES = 5 << 20` (`fileEditing.ts`); the mock's save limit uses
  it. The demo lists `.ua/fingerprints.json`: 2,576,493 characters, 107,506
  lines, generated once.
- RPC frame: 5 MiB of text is at most 30 MiB JSON-escaped (6 bytes per byte),
  under `MaxFrameBytes` (64 MiB). Live code checks already cap documents at
  5 MiB (`cd.MaxDocumentBytes`, `maxLiveParamsBytes` 32 MiB). Replace in files
  handles one file at a time.
- Tests changed to the new limit: `edit_text_test.go` (a file at the read
  limit reads as editable; the "over the edit limit but readable" cases are
  gone, since there is no such file now), `file-drafts.test.mjs` and
  `file-save.test.mjs` use `MAX_EDIT_BYTES` / 5 MiB. The other Go tests use
  `MaxEditBytes` symbolically and pass at the new value.
- `go test ./...`: all pass. `npm test`: 1,477 tests, 1,476 pass, 0 fail,
  1 skipped. `npm run check`: 0 errors, 0 warnings.
- Browser demo (Vite + Playwright, headless Chromium): the 2.5 MB file opens
  in 0.2 s with an enabled Edit button; after Edit, typing, then Ctrl+S right
  after the last key and Discard, the read-only view shows the typed text
  including the last key, so the save wrote it.
- Not checked: the desktop app (WebKitGTK, WebView2) and a remote server with
  a 5 MiB save; the remote servers must be rebuilt to accept it.

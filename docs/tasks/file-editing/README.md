# Go to File editing — tasks

- **Plan:** [file-editing.md](../../plan/file-editing.md)
- **Progress:** 11/14 done

## Tasks

| Task | Status | Scope |
|---|---|---|
| [FE-01](FE-01.md) | ✅ Done | Contracts and filesystem safety decisions |
| [FE-02](FE-02.md) | ✅ Done | Revision and eligibility metadata |
| [FE-03](FE-03.md) | ✅ Done | Path safety and Unix safe saves |
| [FE-04](FE-04.md) | 🟡 Partial | Windows safe saves |
| [FE-05](FE-05.md) | ✅ Done | End-to-end save API |
| [FE-06](FE-06.md) | ✅ Done | Draft-aware tab state |
| [FE-07](FE-07.md) | ✅ Done | CodeMirror component |
| [FE-08](FE-08.md) | ✅ Done | Editing controls and shortcuts |
| [FE-09](FE-09.md) | ✅ Done | Tab close coordination |
| [FE-10](FE-10.md) | ✅ Done | Host lifecycle and uncertain saves |
| [FE-11](FE-11.md) | 🟡 Partial | Native close and quit guards |
| [FE-12](FE-12.md) | ✅ Done | Conflict handling and saved-file refresh |
| [FE-13](FE-13.md) | 🟡 Partial | Regression and platform QA |
| [FE-14](FE-14.md) | ✅ Done | Documentation and release review |

## Initial implementation findings

- Reads now return `revision: "sha256:<hex>"` from exact raw text bytes and
  optional `edit: { eligible, reason?, newline?, bom? }`. Text remains unchanged.
  Older servers may omit both fields. Newline values are `lf`, `crlf`, or `none`.
- All files remain ineligible for now: supported text has `safety_pending`;
  over-limit text has `too_large`, and mixed/bare-CR text has
  `unsupported_newlines`. Binary/image/over-viewer-limit reads omit metadata.
  FE-02 is not complete until path/protection/platform eligibility is connected.
- `os.Root` is available and already used by downloads. Use rooted operations
  for saves, but do not mistake root containment for a no-symlink guarantee.
  Windows junctions can report `ModeIrregular`, not `ModeSymlink`.
- Existing atomic-write helpers do not preserve all project-file metadata.
  Windows replacement ACL/stream behavior and macOS ACL detection need explicit
  implementation and tests before either platform advertises edit eligibility.
  Split FE-03 into path/protection and Unix metadata/replacement subtasks before
  starting; split FE-04 similarly if necessary. No unsafe fallback is allowed.
- The service knows permitted session directories, but unusually broad working
  directories can expose tool stores/credentials. Build protected-target checks
  from host-local configured roots and protected names. Existing viewer credential
  exposure is a separate pre-existing issue; do not enable editing around it.
- Same-host reconnect currently calls `files.reset()` and clears all cached tab
  state. FE-10 must separate connection reload from draft disposal, not merely
  add a prompt to explicit host switches.
- Wails supports `OnBeforeClose`, but no hook is wired. On Windows the hook can
  run on the UI thread: never block it waiting for a frontend response. Prefer
  preventing close, emitting a request, then explicitly authorizing a second
  quit after the draft coordinator succeeds. Design out the first-edit/close
  synchronization race before marking FE-11 complete.
- Each app window is a separate process; only its own drafts need a close guard.
  OS session shutdown/forced termination is not reliably covered by ordinary
  Wails close hooks and must remain an explicit limitation.

## Progress and validation

### Current implementation (supersedes replacement-era findings below)

- User explicitly chose **in-place saves** after reviewing the crash/full-disk
  tradeoff. FE-03b's hard-link refusal and metadata-copy gate and FE-03c/FE-04's
  atomic replacement tasks are superseded by the revised save contract above.
- `edit_target.go` opens existing files through rooted handles, checks component
  identities, bounds reads and rejects symlink/irregular paths and unsupported
  text. Canonical roots are checked against the protection policy again.
- `edit_write.go` implements revision-checked in-place saves, exact BOM/newline
  serialization, no-op saves, write/truncate/sync, and a bounded global save lock.
  Global serialization handles hard-link/root aliases without a leaking lock map.
  Waiting for the lock is cancellable. Identity/path/policy are revalidated before
  mutation. External writers remain outside this lock; partial writes remain an
  accepted limitation. Windows runtime metadata/handle behavior still needs QA.
- Engine reads complete eligibility with protected configured **and default**
  tool/app roots plus an O_RDWR open without mutation. Low-level format-only
  reads report `not_checked` rather than the old `safety_pending` reason.
- FE-05 implementation is wired through backend/service/engine, app, offline
  backend, RPC and CLI capability, frontend API/types and browser mock. Old remote
  servers omit `saveFile`; the client refuses saves before transport. Tests cover
  actual RPC saved/conflict results, unknown directories, protected stores and
  remote/offline routing without local fallback.
- Save backend committed on branch `editor` as `11ca1cd`.
- FE-07 implementation added: lazy runtime/language imports, per-tab immutable
  CodeMirror state/selection/scroll in a WeakMap, live wrapping/read-only state,
  search and save keys, and safe async mount cleanup. No inactive editor DOM.
- Native close backend added: editing must arm the guard before mounting; once
  armed, every close is deferred to a frontend decision. One-shot request IDs
  reject stale approvals; the callback never blocks the UI thread. Frontend must
  freeze input throughout close approval. Unit tests cover cancellation, replay
  and concurrent requests. Desktop-tag Go build/test passes on macOS; actual
  native close interaction remains untested.
- FE-06 draft state implemented and tested: immutable save snapshots, later
  typing retained as dirty, read/refresh/reopen guards, hidden-directory saves,
  host/epoch validation, and conservative refusal to reset/close unresolved drafts.
  `fileEditing.ts` handles editor normalization and byte limits. Failed/stale
  reloads preserve drafts and history; uncertain saves are never replayed.
- A serialized accessible file-decision prompt and destructive-action coordinator
  are implemented. Tab and bulk close paths use the coordinator; native request
  subscription and browser unload guard are installed. The coordinator freezes
  editor input via its busy state.
- Lifecycle: `files.connectionChanged(host)` keeps editors, drafts and caches on
  same-host reconnect, invalidating late reads and refreshing clean tabs and the
  listing. Explicit host switches go through `fileActions.changeHost`, which
  resolves drafts and stays busy until routing completes. Uncertain saves are
  reconciled by reading only and comparing exact submitted bytes (captured
  BOM/newline format); a match advances the baseline without touching later
  typing, a mismatch becomes a conflict. Successful saves refresh only the
  visible listing; a listing failure is not reported as a save failure.
- Conflicts: Resolve… offers Reload (confirmed) or Overwrite (confirmed). The
  overwrite retries against the observed revision via normal CAS and may
  conflict again.
- FE-08 viewer controls exposed: Edit (awaits `enableFileEditing()` to arm the
  native close guard and revalidates tab/dir/host/epoch), Save / Cmd-Ctrl+S,
  Discard, Done, dirty/saving status, read-only reasons, and conflict/uncertain
  error bar. Preview, presentation, Git diff and viewer find hide while editing;
  the editor renders ahead of deleted/empty/error states. Typing is frozen only
  during coordinated prompts and quit, not during ordinary saves or disconnects.
- Release review (FE-14): no body/credential logging, no unchecked revision, no
  silent loss through previews, directory changes, dialog hiding or reset.
  Findings fixed:
  - A save failing after writing began (write/truncate/sync) now returns
    `files.ErrSaveIncomplete` (RPC code `save_incomplete`); the frontend marks
    it uncertain and reconciles by reading, so a draft undone to the baseline
    stays protected. An unresolved conflict also counts as unsaved, since after
    a partial write the draft is the only intact copy.
  - A lazily loaded editor no longer takes focus while a prompt is open; the
    prompt keeps focus inside itself (document `focusin`) and blocks the
    browser's Cmd/Ctrl+F/S/W defaults.
  - `App.SaveFile` takes the host the file was read from and refuses the save,
    unwritten (`ErrSaveHostChanged`), unless the same routing snapshot used for
    the call is that host.
  - Escape regression from the fullscreen safeguard (`9e2f01a`) fixed in
    `6e5581a`: handlers use `keyHandled(e)` instead of `defaultPrevented`.
- Earlier validation: 296 frontend tests pass (drafts, coordinator, prompt,
  editor, viewer controls); Svelte check has 0 errors and the existing tsconfig
  warning; production build passes; full Go suite and desktop-tag tests pass.
- Bundle check: CodeMirror lives only in the lazy `fileEditor` chunk
  (~324 kB, ~105 kB gzip) with language modules in separate chunks; the main
  `index` chunk contains no CodeMirror code, so viewing never loads the editor.
  Responsiveness near the 500 KB limit is not yet measured.
- Latest validation (after the review fixes): 299 frontend tests, Svelte check
  (0 errors, existing tsconfig warning), production build, `go vet ./...`,
  `go test ./...`, race tests for app/files/rpc and desktop-tag tests pass.
- Pending: native desktop interaction QA (editing, close/quit prompts, real
  CodeMirror DOM focus/search/scroll), Linux/Windows filesystem behaviour, and
  SSH/WSL disconnect-during-save scenarios.
- Validation: full `go test ./...`; race tests for files/engine/RPC/app; repeated
  file-save race tests; Linux/Windows cross-platform vet; frontend's 239 tests;
  `npm run check` (0 errors, existing ignoreDeprecations warning). No native
  Linux/Windows or desktop close QA claimed. `npm audit` reports an existing
  high-severity source-map-js dev dependency advisory, not a CodeMirror advisory.

### Earlier incremental notes (historical)

- FE-01: audit complete, contract/policy decisions still partial (unchecked below).
- FE-02: raw-byte revision and text-format metadata implemented. Added table,
  read integration, byte-boundary, and JSON compatibility tests.
- FE-03a: standalone protected-path preflight implemented, including broad-root
  and symlink-alias protection, missing configured roots, case variants, and
  parent-traversal regressions. Service integration and write-time checks remain.
- Save contracts and pure serialization implemented in `edit_save.go`: LF editor
  snapshots restore original BOM/CRLF, preserve intended final newline, and
  enforce serialized byte limits. Saved/conflict results are structured; no
  actual disk-write operation or editing UI is enabled.
- Latest checks: `go test ./...`, `go test -race ./internal/files/`,
  `go vet ./internal/files/`, and `git diff --check` passed. Found the installed
  Node toolchain outside PATH; `npm run check` now runs with zero errors and one
  tsconfig warning (`ignoreDeprecations: "6.0"`). Platform save QA is still pending.
- Validation: `go test ./internal/files/`, `go vet ./internal/files/`, and
  `go build ./...` passed using the installed Go 1.27.2 toolchain. Go is not on
  the shell PATH. Frontend type checking could not run because Node/npm are not
  available on PATH. No native platform save/close QA has been performed.

Every task is scoped to at most four engineering hours, including focused tests.
Split a task further before implementation if it exceeds that size. Estimates
are provisional; platform-safe replacement and native close guards are the main
uncertainties. Checkboxes record actual status; unrun manual QA stays unchecked.
Since the user chose in-place writes, "replacement"/"temporary file" items below
are satisfied by the in-place contract (same inode, truncate, sync) instead.

| ID | Task | Est | Depends on | Status |
|---|---|---|---|---|
| FE-01 | Finalize contracts and filesystem safety decisions | 3 h | — | Done |
| FE-02 | Add edit eligibility and revision metadata to reads | 3 h | FE-01 | Done |
| FE-03a | Add protected-target and rooted path preflight | 3 h | FE-01 | Done |
| FE-03b | Add target identity, hard-link and Unix metadata checks | 4 h | FE-02, FE-03a | Done (in-place) |
| FE-03c | Implement revision-checked Unix replacement and locking | 4 h | FE-03b | Done (in-place) |
| FE-04 | Implement and verify Windows safe replacement | 4 h | FE-03 | Code done; Windows QA pending |
| FE-05 | Wire backend, RPC, desktop, capabilities, and mocks | 4 h | FE-03, FE-04 | Done |
| FE-06 | Add draft-aware tab state and save snapshot handling | 4 h | FE-02, FE-05 | Done |
| FE-07 | Integrate lazy-loaded CodeMirror and retained editor state | 4 h | FE-06 | Done; real-DOM QA pending |
| FE-08 | Add editing toolbar, shortcuts, and viewer transitions | 3 h | FE-07 | Done |
| FE-09 | Add dirty-tab and coordinated bulk-close guards | 3 h | FE-06, FE-08 | Done |
| FE-10 | Protect drafts across host transitions and disconnects | 4 h | FE-05, FE-06, FE-09 | Done; SSH/WSL QA pending |
| FE-11 | Guard native window close and normal app quit | 4 h | FE-09 | Done; native QA pending |
| FE-12 | Add conflict UI and targeted post-save refresh | 4 h | FE-05, FE-08, FE-09 | Done |
| FE-13 | Regression coverage and cross-platform QA | 4 h | FE-10, FE-11, FE-12 | Automated done; manual pending |
| FE-14 | Document behavior and complete release review | 2 h | FE-13 | Done; findings fixed |

Total: approximately 50 hours including platform hardening and QA; revise after
FE-01. Independent frontend/backend work can overlap once contracts are settled.

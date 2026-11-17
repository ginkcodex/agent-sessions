# NAV-08 — Pyright definitions inside the project

- **Status:** ✅ Done (2026-11-17)
- **Estimate:** 1 h
- **Depends on:** NAV-01
- **Plan:** [editor-essentials.md](../../plan/editor-essentials.md)

## Scope

Found in manual testing: in a Python project at the workspace root
(`app/`, `.venv`, no `pyrightconfig.json`), Go to Definition did nothing for
symbols from the project's own modules (`from app.config import Settings`),
while definitions in `.venv` packages worked. Code checks showed false
`Import "app.…" could not be resolved` errors on every file.

Cause: `initializeParams` sent `workspaceFolders` only for a server scoped
to a subdirectory (`Directory != ""`). Pyright 1.1.414 resolves project
imports from workspace folders, not from `rootUri`, so with only `rootUri` it
answered `null` for every project-local definition. Fix: always send the one
workspace folder, the same directory `rootUri` names. Only rust-analyzer
sets `Directory`; every other server at the root now receives it too.

## Likely files

- `internal/codediagnostics/lsp_session.go`
- `internal/codediagnostics/lsp_session_test.go`

## Acceptance criteria

- Initialize names the root as the one workspace folder, equal to `rootUri`.
- Scoped servers (rust-analyzer in a nested crate) are unchanged.
- No new client capability: `workspace.workspaceFolders` stays undeclared and
  `workspace/workspaceFolders` requests are still rejected.
- Approval is unchanged: `Directory` stays bound, initialize params are not.

## Tests

- `TestSessionAdvertisesNoLiveCapability` asserts the root workspace folder
  (it asserted no folders before).
- Real-server probe: Pyright definition on project and `.venv` symbols, with
  and without the folder; other managed servers compared the same way.

## Evidence

Verified on 2026-11-17:

- Real Pyright 1.1.414 (the app-managed install, run with the app's
  initialize capabilities and `workspace/configuration` settings, `.venv`
  interpreter) on `learning/supervisor-chat-mvp`, `app/service.py`:
  definition of `Settings` and `ChatRepository` (import and use) was `null`
  without the folder and resolved to `app/config.py:12` and
  `app/db/repository.py:19` with it. `AIMessage` from `.venv` resolved in both.
  The `null` answers stayed `null` after an 8 s wait.
- Pyright diagnostics on five project files: without the folder, 27 false
  `Import "app.…" could not be resolved` errors plus one follow-on error;
  with it, none, and `app/engine/nodes.py` shows 11 more real type errors
  (TypedDict access, `AnyMessage` vs `AIMessage`) that the unresolved
  imports had hidden. Same server requests and opened-files-only
  publishing; peak memory ~350 MB → ~372 MB.
- Ruff 0.15.20, gopls v0.23.0 (this repository), vtsls 0.3.0 (`frontend/`)
  and rust-analyzer 2026-10-26 (a root crate): server requests, published
  diagnostics and error messages identical with and without the folder.
- New test failed before the fix (`workspace folders <nil>`) and passed
  after. `go test ./... -count=1`: all passed. `go vet` on the package:
  clean. `make lint`: fails on 155 existing findings, none in the changed
  files; the totals match the run without this change.

Not checked: ruby-lsp, RuboCop, the PHP and Java servers and Svelte (not
installed here); Go to Definition through the desktop app itself (needs a
build with fresh `remote/` servers, since the fix runs in
`agent-sessions-cli serve`).

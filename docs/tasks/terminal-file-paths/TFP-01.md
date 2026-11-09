# TFP-01 — Detect inner file paths in terminal tool calls

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** —
- **Plan:** [terminal-file-paths.md](../../plan/terminal-file-paths.md)

## Scope

Recognize identifier-style tool-call wrappers, preserving inner path spelling
and exact offsets. Keep existing Git, URL, cwd and modifier behavior.

## Likely files

- `frontend/src/lib/terminalFileLinks.ts`
- `frontend/test/terminal-file-links.test.mjs`

## Acceptance criteria

- `Update(docs/tasks/editor-essentials/README.md)` and `Update(TASKS.md)`
  return only the file path and its exact range.
- Quoted arguments with spaces and line/column suffixes work.
- Parentheses within filenames remain intact; URLs are not split into paths.
- Wrapped-row provider ranges and Ctrl/Cmd activation use only the inner path.

## Tests

Run focused terminal-link tests, full frontend tests, Svelte/TypeScript checks
and a production frontend build. Record native desktop testing separately.

## Evidence

- Reproduced the screenshot failure with a regression test before the fix:
  extraction returned `Update(docs/tasks/editor-essentials/README.md`.
- Added structural tool-call unwrapping and quoted-argument tokenization;
  preserved balanced filename parentheses and exact inner-path offsets.
- Focused terminal-link tests: 15/15 passed, including wrapped-row ranges and
  both Ctrl and Cmd activation, quoted paths, Windows/POSIX paths, suffixes,
  URL exclusion and existing Git/cwd behavior.
- Full frontend suite: 1,177 passed, 1 skipped, 0 failed.
- `make test` (`go test -race ./...`): passed.
- `npm run check`: 0 errors and 0 warnings.
- `npm run build`: passed; Vite reports its large-chunk warning.
- `make lint`: failed with 135 findings in unchanged Go files (including
  unchecked `fmt.Fprintf` results in `cmd/agent-sessions-cli/export.go`).
  No Go files were changed for this task.
- Native desktop/macOS visual Ctrl/Cmd-click testing was not performed.

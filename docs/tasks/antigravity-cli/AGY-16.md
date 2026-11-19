# AGY-16 — Tool calls show no file path or command

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** [AGY-04](AGY-04.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). In a saved Antigravity
transcript, at the Activity and All levels, `read_file`, `write_file` and
the other tool cards show only the tool name. The card has no file path to
open, and no summary line. The path is only in the Parameters JSON, as
`"Path": "/Users/…/cli.md"`.

## Cause

`frontend/src/lib/toolPresentation.ts` knows other agents' argument names:

- `toolFileTarget` reads `file_path`, `filePath` and `notebook_path`, and
  reads `path` only for tools named `read`, `edit`, `write`, `multiedit` or
  `notebookedit`.
- `toolInputSummary` reads `command`, `cmd`, `file_path`, `filePath`,
  `path`, `pattern`, `query` and `description`.

agy uses capitalized names. Argument keys in all local transcripts (agy
1.5.12, Linux, 7 sessions):

| Tool | Keys (count) |
|---|---|
| `read_file` | `Path` (172), `StartLine` / `EndLine` (17) |
| `write_file` | `Path`, `Content`, `Overwrite` (11) |
| `edit_file` | `Path`, `Target`, `Replacement` (4) |
| `run_command` | `CommandLine`, `WaitMsBeforeAsync`, `toolSummary` (21) |
| `search_web` | `query` (6) |
| `invoke_subagent` | `Prompt`, `Role`, `TypeName` (1) |
| `send_message` | `Recipient`, `Message` (1) |

The provider's fixtures (`internal/provider/antigravity/testdata/*.jsonl`)
use other names: `AbsolutePath`, `TargetFile`, `CodeContent`,
`DirectoryPath`, `Task`. None of these appear in real transcripts, so the
fixtures should be brought in line.

Live chat cards are a separate path. agy's ACP `tool_call` gives a title
(`File ../../../.gemini/…`, relative) and `locations` with the absolute
path. Check whether the live card's path opens, and fix it here if the fix
is small.

## Scope

- `toolFileTarget`:
  - `Path` for `read_file`, `write_file` and `edit_file`;
  - the line from `read_file`'s `StartLine`, if agy counts lines from 1;
  - check that from a saved call and its output (`Showing lines …`).
- `toolInputSummary`: `CommandLine`, then `Path`, so `run_command` and the
  file tools get a summary line.
- Fixtures: use the real argument names. Regenerate the goldens and check
  that the diff is only the names.
- `internal/handoff/build.go`: check whether the Antigravity adapter reads
  arguments by name. If it does, use the real ones.

## Likely files

- `frontend/src/lib/toolPresentation.ts`, `frontend/test/` tests for it.
- `internal/provider/antigravity/testdata/*.jsonl` and goldens.
- `frontend/src/lib/mock/mockApi.ts` (the sample Antigravity session).

## Acceptance criteria

- A saved `read_file`, `write_file` and `edit_file` card shows its path,
  and opening it opens the file (at `StartLine` for `read_file`).
- A `run_command` card shows its command line.
- Claude Code, Codex and OpenCode cards are unchanged.

## Tests

- Frontend: `toolFileTarget` and `toolInputSummary` with agy inputs, and
  the existing cases for the other agents.
- Go: goldens after the fixture rename.
- Manual: the "General Greeting" style session on macOS at the Activity
  level.
